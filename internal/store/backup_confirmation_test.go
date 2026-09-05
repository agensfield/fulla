package store

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestBackupRestoreConfirmationPlanAndCancellation(t *testing.T) {
	s := fixture(t, false)
	old := []byte{0, 255, 10}
	if _, err := s.Write("replace", old, false); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.Write("add", old, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Remove("add", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write("replace", []byte("current"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write("remove", []byte("current"), false); err != nil {
		t.Fatal(err)
	}
	backups, err := s.Backups()
	if err != nil {
		t.Fatal(err)
	}
	other, err := Open(s.Dir, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	cancelled := errors.New("fixture cancelled")
	_, err = s.BackupRestoreConfirmed(snapshot.Transaction, "after", func(plan BackupRestorePlan) error {
		if plan.ID != snapshot.Transaction || plan.Phase != "after" || !reflect.DeepEqual(plan.Added, []string{"add"}) || !reflect.DeepEqual(plan.Replaced, []string{"replace"}) || !reflect.DeepEqual(plan.Removed, []string{"remove"}) {
			t.Fatalf("incorrect plan %+v", plan)
		}
		if _, err := other.Write("competing", []byte("fixture"), false); err == nil {
			t.Fatal("confirmation lacked shared lock")
		}
		return cancelled
	})
	if !errors.Is(err, cancelled) {
		t.Fatal(err)
	}
	names, err := s.Names()
	if err != nil || !reflect.DeepEqual(names, []string{"remove", "replace"}) {
		t.Fatal("cancel changed names", names, err)
	}
	for _, name := range names {
		value, err := s.Read(name)
		if err != nil || string(value) != "current" {
			t.Fatal("cancel changed value", err)
		}
	}
	after, err := s.Backups()
	if err != nil || len(after) != len(backups) {
		t.Fatal("cancel created backup")
	}
	if err := s.Unlocked(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BackupRestoreConfirmed(snapshot.Transaction, "after", func(BackupRestorePlan) error { return nil }); err != nil {
		t.Fatal(err)
	}
	names, err = s.Names()
	if err != nil || !reflect.DeepEqual(names, []string{"add", "replace"}) {
		t.Fatal("restore has wrong names", names, err)
	}
	for _, name := range names {
		value, err := s.Read(name)
		if err != nil || !bytes.Equal(value, old) {
			t.Fatal("restore lost exact bytes", err)
		}
	}
}

func TestEmptyBackupRestoreRejectsMismatchedActiveIdentity(t *testing.T) {
	s := fixture(t, false)
	snapshot, err := s.Write("entry", []byte("fixture"), false)
	if err != nil {
		t.Fatal(err)
	}
	_, recipient, err := crypt.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if err := securefs.Replace(s.Root, "recipients", []byte(recipient)); err != nil {
		t.Fatal(err)
	}
	called := false
	if _, err := s.BackupRestoreConfirmed(snapshot.Transaction, "before", func(BackupRestorePlan) error { called = true; return nil }); err == nil || called {
		t.Fatal("empty restore bypassed key consistency")
	}
	exists, err := s.Exists("entry")
	if err != nil || !exists {
		t.Fatal("preflight failure removed entry")
	}
	if err := s.Unlocked(); err != nil {
		t.Fatal(err)
	}
}
