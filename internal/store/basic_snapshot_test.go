package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agensfield/fulla/internal/securefs"
)

func setTransactionVersion(t *testing.T, s *Store, version int) {
	t.Helper()
	next := s.Meta
	next.Domains = map[string]int{}
	for name, value := range s.Meta.Domains {
		next.Domains[name] = value
	}
	next.Domains["transactions"] = version
	data, err := json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	if err := securefs.Replace(s.Root, metadata+"/store.json", data); err != nil {
		t.Fatal(err)
	}
}

func TestBasicSnapshotsRemainRecoverableAndPrunable(t *testing.T) {
	for _, git := range []bool{false, true} {
		t.Run(fmt.Sprint("git=", git), func(t *testing.T) {
			s := fixture(t, git)
			setTransactionVersion(t, s, 99)
			original, replacement := []byte{0, 255, 10}, []byte{128, 0, 10, 10}
			if _, err := s.Write("a", original, false); err != nil {
				t.Fatal(err)
			}
			edit, err := s.Write("a", replacement, true)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.Write("b", []byte{}, false); err != nil {
				t.Fatal(err)
			}
			backups, err := s.Backups()
			if err != nil || len(backups) != 3 {
				t.Fatal("basic snapshots missing", backups, err)
			}
			for _, backup := range backups {
				journal, err := s.BackupShow(backup.ID)
				if err != nil || backup.SnapshotDomain != basicProtocol || journal.SnapshotDomain != basicProtocol {
					t.Fatal("lost snapshot origin", err)
				}
			}
			before := archiveTree(t, s, false)
			confirmed := false
			if _, err := s.BackupRestoreConfirmed(edit.Transaction, "before", func(BackupRestorePlan) error { confirmed = true; return nil }); err == nil || confirmed {
				t.Fatal("restoration bypassed future transaction domain")
			}
			preview, err := s.Prune(Retention{Keep: retain(1)}, true)
			if err != nil || len(preview.Candidates) != 2 {
				t.Fatal("basic retention preview", preview, err)
			}
			if !reflect.DeepEqual(before, archiveTree(t, s, false)) {
				t.Fatal("refusal or preview changed store")
			}
			// Undo only this fixture's injected counter. There is no production schema
			// transformation here; never lower real domain versions to enable recovery.
			setTransactionVersion(t, s, 1)
			for _, phase := range []string{"before", "after"} {
				if _, err := s.BackupRestore(edit.Transaction, phase); err != nil {
					t.Fatal("basic snapshot restore", phase, err)
				}
				want := original
				if phase == "after" {
					want = replacement
				}
				value, err := s.Read("a")
				if err != nil || !bytes.Equal(value, want) {
					t.Fatal("snapshot lost exact bytes", phase, err)
				}
				if exists, err := s.Exists("b"); err != nil || exists {
					t.Fatal("snapshot lost entry-set semantics", err)
				}
			}
			recovery := fixture(t, false)
			ids, recipients, err := recovery.Keys()
			if err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(filepath.Dir(recovery.Dir), "archive.age")
			if _, err := s.ExportFull(recipients, output); err != nil {
				t.Fatal(err)
			}
			archive, err := ReadArtifact(output, MaxBundleBytes)
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(filepath.Dir(recovery.Dir), "restored")
			if _, err := RestoreFull(archive, ids, target); err != nil {
				t.Fatal(err)
			}
			clone, err := Open(target, true, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer clone.Close()
			journal, err := clone.BackupShow(edit.Transaction)
			if err != nil || journal.SnapshotDomain != basicProtocol {
				t.Fatal("full archive lost basic snapshot", err)
			}
			if _, err := clone.BackupRestore(edit.Transaction, "before"); err != nil {
				t.Fatal(err)
			}
			value, err := clone.Read("a")
			if err != nil || !bytes.Equal(value, original) {
				t.Fatal("cloned snapshot lost bytes", err)
			}
			live, err := s.Ciphertext("a")
			if err != nil {
				t.Fatal(err)
			}
			pruned, err := s.Prune(Retention{Keep: retain(0)}, false)
			if err != nil || len(pruned.Deleted) == 0 {
				t.Fatal("basic prune", pruned, err)
			}
			remaining, err := s.Backups()
			if err != nil || len(remaining) != 0 {
				t.Fatal("retained snapshots", err)
			}
			after, err := s.Ciphertext("a")
			if err != nil || !bytes.Equal(live, after) {
				t.Fatal("prune modified live ciphertext", err)
			}
		})
	}
}
