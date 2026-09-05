package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func setBackupVersion(t *testing.T, s *Store, version int) {
	t.Helper()
	next := s.Meta
	next.Domains = map[string]int{}
	for domain, value := range s.Meta.Domains {
		next.Domains[domain] = value
	}
	next.Domains["backup"] = version
	data, err := json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	if err := securefs.Replace(s.Root, metadata+"/store.json", data); err != nil {
		t.Fatal(err)
	}
}

func TestFutureBackupDomainPreservesCRUDAndTransactionSnapshots(t *testing.T) {
	for _, git := range []bool{false, true} {
		t.Run(fmt.Sprintf("git=%v", git), func(t *testing.T) {
			s := fixture(t, git)
			original, replacement := []byte{0, 255, 10}, []byte{128, 0, 10, 10}
			if _, err := s.Write("original", original, false); err != nil {
				t.Fatal(err)
			}
			// This opaque file belongs to the future domain; CRUD must not interpret it.
			if err := securefs.WriteNew(s.Root, metadata+"/backups/.fulla-stage-future", []byte("opaque future format")); err != nil {
				t.Fatal(err)
			}
			setBackupVersion(t, s, 2)
			before := transactionFiles(t, s)
			edit, err := s.Write("original", replacement, true)
			if err != nil {
				t.Fatal(err)
			}
			if edit.Backup != snapshotBase("transactions")+"/"+edit.Transaction {
				t.Fatal("wrong snapshot receipt", edit)
			}
			if _, err := s.Move("original", "renamed"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Remove("renamed", true); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Write("new", original, false); err != nil {
				t.Fatal(err)
			}
			got, err := s.Read("new")
			if err != nil || !bytes.Equal(got, original) {
				t.Fatal("future-domain CRUD lost bytes", err)
			}
			after := transactionFiles(t, s)
			for name, hash := range before {
				if strings.HasPrefix(name, metadata+"/backups/") || name == metadata+"/store.json" {
					if after[name] != hash {
						t.Fatal("CRUD changed newer metadata", name)
					}
				}
			}
			for name := range after {
				if strings.HasPrefix(name, metadata+"/backups/") {
					if _, ok := before[name]; !ok {
						t.Fatal("CRUD added future backup metadata", name)
					}
				}
			}
			_, err = s.Backups()
			var problem *fault.Error
			if !errors.As(err, &problem) || problem.Code != "metadata.unsupported" {
				t.Fatal("interpreted future backup domain", err)
			}
			ids, _, err := s.Keys()
			if err != nil {
				t.Fatal(err)
			}
			for phase, want := range map[string][]byte{"before": original, "after": replacement} {
				data, err := securefs.Read(s.Root, edit.Backup+"/"+phase+"/passwords/original.age", crypt.MaxEntryBytes+4096)
				if err != nil {
					t.Fatal(err)
				}
				plain, err := crypt.Decrypt(data, ids)
				if err != nil || !bytes.Equal(plain, want) {
					t.Fatal("transaction snapshot lost exact bytes", phase, err)
				}
			}
			// Undo only the injected fixture version, not a real future-format downgrade.
			setBackupVersion(t, s, 1)
			backups, err := s.Backups()
			if err != nil || len(backups) != 5 {
				t.Fatal("compatible reader lost transaction snapshots", len(backups), err)
			}
			if _, err := s.BackupRestore(edit.Transaction, "before"); err != nil {
				t.Fatal(err)
			}
			got, err = s.Read("original")
			if err != nil || !bytes.Equal(got, original) {
				t.Fatal("snapshot restore lost original", err)
			}
			zero := 0
			result, err := s.Prune(Retention{Keep: &zero}, false)
			if err != nil || len(result.Deleted) != 6 {
				t.Fatal("prune lost snapshot locations", result, err)
			}
			entries, err := fs.ReadDir(s.Root.FS(), snapshotBase("transactions"))
			if err != nil || len(entries) != 0 {
				t.Fatal("prune retained transaction snapshots", err)
			}
		})
	}
}

func TestTransactionSnapshotPruneRecoveryAndUnknownLocation(t *testing.T) {
	s := fixture(t, false)
	setBackupVersion(t, s, 2)
	for _, name := range []string{"a", "b"} {
		if _, err := s.Write(name, []byte(name), false); err != nil {
			t.Fatal(err)
		}
	}
	setBackupVersion(t, s, 1)
	_, err := s.prune(Retention{Keep: retain(0)}, false, func(phase string) error {
		if phase == "removed" {
			return errors.New("fixture interruption")
		}
		return nil
	})
	if err == nil {
		t.Fatal("prune did not interrupt")
	}
	data, err := securefs.Read(s.Root, metadata+"/prune.json", maxMetadata)
	if err != nil {
		t.Fatal(err)
	}
	var j pruneJournal
	if err := StrictJSON(data, &j); err != nil {
		t.Fatal(err)
	}
	if j.Selected[0].SnapshotDomain != "transactions" {
		t.Fatal("prune lost snapshot location")
	}
	original := j.Selected[0].SnapshotDomain
	j.Selected[0].SnapshotDomain = "../../passwords"
	before := transactionFiles(t, s)
	if err := s.finishPrune(&j, nil); err == nil {
		t.Fatal("accepted unknown snapshot namespace")
	}
	after := transactionFiles(t, s)
	for name, hash := range before {
		if after[name] != hash {
			t.Fatal("invalid namespace changed files")
		}
	}
	j.Selected[0].SnapshotDomain = original
	if err := s.finishPrune(&j, nil); err != nil {
		t.Fatal("prune retry lost bound locations", err)
	}
	entries, err := fs.ReadDir(s.Root.FS(), snapshotBase("transactions"))
	if err != nil || len(entries) != 0 {
		t.Fatal("prune retry retained snapshots", err)
	}
}

func TestDuplicateSnapshotLocationFailsClosed(t *testing.T) {
	s := fixture(t, false)
	r, err := s.Write("entry", []byte("fixture"), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Root.Mkdir(snapshotBase("transactions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(s.Root, r.Backup, snapshotBase("transactions")+"/"+r.Transaction); err != nil {
		t.Fatal(err)
	}
	before := transactionFiles(t, s)
	for _, operation := range []func() error{
		func() error { _, err := s.Backups(); return err },
		func() error { _, err := s.BackupRestore(r.Transaction, "before"); return err },
		func() error { _, err := s.Prune(Retention{Keep: retain(0)}, false); return err },
	} {
		if err := operation(); err == nil {
			t.Fatal("accepted ambiguous snapshot identity")
		}
		after := transactionFiles(t, s)
		if len(before) != len(after) {
			t.Fatal("refusal changed files")
		}
		for name, hash := range before {
			if after[name] != hash {
				t.Fatal("refusal changed file", name)
			}
		}
	}
}

func TestTransactionSnapshotsSurviveFullArchive(t *testing.T) {
	source := fixture(t, false)
	if _, err := source.Write("entry", []byte{0, 255, 10}, false); err != nil {
		t.Fatal(err)
	}
	setBackupVersion(t, source, 2)
	edit, err := source.Write("entry", []byte("new"), true)
	if err != nil {
		t.Fatal(err)
	}
	setBackupVersion(t, source, 1) // Undo only the synthetic fixture version.
	recovery := fixture(t, false)
	ids, rs, err := recovery.Keys()
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(filepath.Dir(source.Dir), "archive.age")
	before := transactionFiles(t, source)
	if _, err := source.ExportFull(rs, output); err != nil {
		t.Fatal(err)
	}
	data, err := ReadArtifact(output, MaxBundleBytes)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(filepath.Dir(recovery.Dir), "restored")
	if _, err := RestoreFull(data, ids, target); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(target, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if !reflect.DeepEqual(before, transactionFiles(t, restored)) {
		t.Fatal("archive changed complete store files")
	}
	if _, err := restored.BackupRestore(edit.Transaction, "before"); err != nil {
		t.Fatal(err)
	}
	got, err := restored.Read("entry")
	if err != nil || !bytes.Equal(got, []byte{0, 255, 10}) {
		t.Fatal("archive lost transaction snapshot recovery", err)
	}
}
