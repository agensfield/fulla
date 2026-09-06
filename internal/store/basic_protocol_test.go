package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/agensfield/fulla/internal/securefs"
)

func futureBasicFixture(t *testing.T, s *Store, all bool) map[string]archiveTreeEntry {
	t.Helper()
	next := s.Meta
	next.Domains = map[string]int{}
	for name, version := range s.Meta.Domains {
		next.Domains[name] = version
		if all || name == "transactions" {
			next.Domains[name] = 99
		}
	}
	data, err := json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	if err := securefs.Replace(s.Root, metadata+"/store.json", data); err != nil {
		t.Fatal(err)
	}
	// Unknown feature bytes are deliberately not a version-1 journal schema.
	for _, dir := range []string{"transactions", "backups", "receipts"} {
		if err := securefs.WriteNew(s.Root, metadata+"/"+dir+"/future-sentinel", []byte("opaque future feature")); err != nil {
			t.Fatal(err)
		}
	}
	return basicForeignTree(t, s)
}

func basicForeignTree(t *testing.T, s *Store) map[string]archiveTreeEntry {
	t.Helper()
	tree := archiveTree(t, s, false)
	for name := range tree {
		if name == "passwords" || strings.HasPrefix(name, "passwords/") || name == basicBase || strings.HasPrefix(name, basicBase+"/") || name == "lock" || strings.HasPrefix(name, "lock/") {
			delete(tree, name)
		}
	}
	return tree
}

func TestFutureTransactionBasicCRUDPreservesFeatureMetadata(t *testing.T) {
	for _, git := range []bool{false, true} {
		for _, all := range []bool{false, true} {
			t.Run(fmt.Sprintf("git=%v/all=%v", git, all), func(t *testing.T) {
				s := fixture(t, git)
				before := futureBasicFixture(t, s, all)
				for _, step := range []struct {
					name string
					run  func() (MutationResult, error)
				}{
					{"add", func() (MutationResult, error) { return s.Write("a", []byte{0, 255, 10}, false) }},
					{"edit", func() (MutationResult, error) { return s.Write("a", []byte{}, true) }},
					{"move", func() (MutationResult, error) { return s.Move("a", "nested/b") }},
					{"remove", func() (MutationResult, error) { return s.Remove("nested/b", true) }},
				} {
					result, err := step.run()
					if err != nil || !result.Applied {
						t.Fatal(step.name, result, err)
					}
					if !strings.HasPrefix(result.Backup, basicBase+"/backups/") || !strings.HasPrefix(result.Receipt, basicBase+"/receipts/") {
						t.Fatal("used upgradeable feature paths", result)
					}
					switch step.name {
					case "add":
						value, err := s.Read("a")
						if err != nil || !bytes.Equal(value, []byte{0, 255, 10}) {
							t.Fatal("binary add", err)
						}
					case "edit":
						value, err := s.Read("a")
						if err != nil || len(value) != 0 {
							t.Fatal("empty edit", err)
						}
					case "move":
						value, err := s.Read("nested/b")
						if err != nil || len(value) != 0 {
							t.Fatal("empty move", err)
						}
					case "remove":
						if exists, err := s.Exists("nested/b"); err != nil || exists {
							t.Fatal("remove", err)
						}
					}
					if !reflect.DeepEqual(before, basicForeignTree(t, s)) {
						t.Fatal("changed unknown feature bytes or modes")
					}
					if err := s.Unlocked(); err != nil {
						t.Fatal(err)
					}
					entries, err := s.Root.ReadFile(result.Receipt)
					if err != nil {
						t.Fatal(err)
					}
					var j Journal
					if err := StrictJSON(entries, &j); err != nil || j.SnapshotDomain != basicProtocol || j.Command != step.name {
						t.Fatal("lost basic receipt", err)
					}
				}
				unchanged := archiveTree(t, s, false)
				if _, err := s.Rotate(false, "", false); err == nil {
					t.Fatal("rotation interpreted future transaction staging")
				}
				if !reflect.DeepEqual(unchanged, archiveTree(t, s, false)) {
					t.Fatal("refused rotation changed feature metadata")
				}
				for _, operation := range []string{"transfer import", "history restore", "backup restore"} {
					if lock, err := s.lockMutation(operation); err == nil {
						_ = lock.Release()
						t.Fatal("interpreted future recovery domain", operation)
					}
				}
			})
		}
	}
}
