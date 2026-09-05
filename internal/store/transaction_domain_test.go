package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"testing"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func transactionFiles(t *testing.T, s *Store) map[string]string {
	t.Helper()
	files := map[string]string{}
	if err := fs.WalkDir(s.Root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(s.Root.FS(), name)
		if err == nil {
			files[name] = digest(data)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

func TestTransactionRejectsUnsupportedSnapshotPublication(t *testing.T) {
	for _, git := range []bool{false, true} {
		for _, recovery := range []bool{false, true} {
			t.Run(fmt.Sprintf("git=%v/recovery=%v", git, recovery), func(t *testing.T) {
				s := fixture(t, git)
				if _, err := s.Write("existing", []byte("original"), false); err != nil {
					t.Fatal(err)
				}
				var pending Journal
				if recovery {
					_, rs, err := s.Keys()
					if err != nil {
						t.Fatal(err)
					}
					ciphertext, err := crypt.Encrypt([]byte("replacement"), rs)
					if err != nil {
						t.Fatal(err)
					}
					lock, err := s.Lock("fixture edit")
					if err != nil {
						t.Fatal(err)
					}
					_, err = s.mutate(lock, "fixture edit", map[string][]byte{"existing": ciphertext}, func(phase string) error {
						if phase == "published:existing" {
							return errors.New("fixture interruption")
						}
						return nil
					})
					if err == nil {
						t.Fatal("fixture failed to interrupt")
					}
					data, err := securefs.Read(s.Root, metadata+"/pending.json", maxMetadata)
					if err != nil {
						t.Fatal(err)
					}
					if err := StrictJSON(data, &pending); err != nil {
						t.Fatal(err)
					}
				}
				next := s.Meta
				next.Domains = map[string]int{}
				for domain, version := range s.Meta.Domains {
					next.Domains[domain] = version
				}
				next.Domains["backup"] = 2
				if !recovery {
					delete(next.Domains, "backup")
				}
				data, err := json.Marshal(next)
				if err != nil {
					t.Fatal(err)
				}
				if err := securefs.Replace(s.Root, metadata+"/store.json", data); err != nil {
					t.Fatal(err)
				}
				before := transactionFiles(t, s)
				if recovery {
					err = s.finishJournal(&pending, nil)
				} else {
					_, err = s.Write("new", []byte("value"), false)
				}
				var problem *fault.Error
				if !errors.As(err, &problem) || problem.Code != "metadata.unsupported" {
					t.Fatal("transaction wrote unsupported backup domain", err)
				}
				if !reflect.DeepEqual(before, transactionFiles(t, s)) {
					t.Fatal("unsupported transaction changed store files")
				}
				if !recovery {
					value, err := s.Read("existing")
					if err != nil || string(value) != "original" {
						t.Fatal("future backup domain blocked independent read", err)
					}
				}
				// Restore only the deliberately injected fixture version. This is
				// not a production downgrade or a migration of future metadata.
				data, err = json.Marshal(s.Meta)
				if err != nil {
					t.Fatal(err)
				}
				if err := securefs.Replace(s.Root, metadata+"/store.json", data); err != nil {
					t.Fatal(err)
				}
				if recovery {
					err = s.finishJournal(&pending, nil)
				} else {
					_, err = s.Write("new", []byte("value"), false)
				}
				if err != nil {
					t.Fatal("supported transaction could not retry", err)
				}
			})
		}
	}
}
