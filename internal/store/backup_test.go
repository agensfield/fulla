package store

import "testing"

func TestHistoryAndWholeSnapshotRestore(t *testing.T) {
	s := fixture(t, true)
	if _, err := s.Write("persistent", []byte("first"), false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write("removed", []byte("second"), false); err != nil {
		t.Fatal(err)
	}
	history, err := s.History("removed")
	if err != nil || len(history) == 0 {
		t.Fatal(history, err)
	}
	deletion, err := s.Remove("removed", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.HistoryRestore(history[0].Commit, "removed"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read("removed")
	if err != nil || string(got) != "second" {
		t.Fatal("history restore failed", err)
	}
	if _, err := s.Write("extra", []byte("not-in-snapshot"), false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BackupRestore(deletion.Transaction, "before"); err != nil {
		t.Fatal(err)
	}
	names, err := s.Names()
	if err != nil || len(names) != 2 {
		t.Fatal("snapshot not restored", names, err)
	}
	got, err = s.Read("persistent")
	if err != nil || string(got) != "first" {
		t.Fatal("unmodified snapshot entry lost", err)
	}
	if _, err := s.BackupRestore(deletion.Transaction, "after"); err != nil {
		t.Fatal(err)
	}
	names, err = s.Names()
	if err != nil || len(names) != 1 || names[0] != "persistent" {
		t.Fatal("after snapshot not restored", names, err)
	}
	backups, err := s.Backups()
	if err != nil || len(backups) < 1 {
		t.Fatal(backups, err)
	}
}
