package store

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/agensfield/fulla/internal/fault"
)

func TestHistoryRestoreConfirmationOwnsLockAndPreservesCancelledState(t *testing.T) {
	s := fixture(t, true)
	old := []byte{0, 255, 10}
	current := []byte("current-fixture")
	if _, err := s.Write("entry", old, false); err != nil {
		t.Fatal(err)
	}
	history, err := s.History("entry")
	if err != nil {
		t.Fatal(err)
	}
	ref := history[0].Commit
	if _, err := s.Write("entry", current, true); err != nil {
		t.Fatal(err)
	}
	head, err := s.Head()
	if err != nil {
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
	_, err = s.HistoryRestoreConfirmed(ref, "entry", func(plan HistoryRestorePlan) error {
		if plan.Commit != ref || plan.Name != "entry" || !plan.Replaces {
			t.Fatalf("incorrect plan %+v", plan)
		}
		if _, err := other.Write("entry", []byte("competing"), true); err == nil {
			t.Fatal("confirmation did not hold shared lock")
		}
		return cancelled
	})
	if !errors.Is(err, cancelled) {
		t.Fatal(err)
	}
	value, err := s.Read("entry")
	if err != nil || !bytes.Equal(value, current) {
		t.Fatal("cancel changed entry")
	}
	afterHead, err := s.Head()
	if err != nil || head != afterHead {
		t.Fatal("cancel changed Git history")
	}
	afterBackups, err := s.Backups()
	if err != nil || len(backups) != len(afterBackups) {
		t.Fatal("cancel created backup")
	}
	if err := s.Unlocked(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.HistoryRestoreConfirmed(ref, "entry", func(HistoryRestorePlan) error { return nil }); err != nil {
		t.Fatal(err)
	}
	value, err = s.Read("entry")
	if err != nil || !bytes.Equal(value, old) {
		t.Fatal("restore lost exact bytes")
	}
	// Recovery must not require decrypting the damaged current value. Commit
	// this synthetic damage so the ordinary dirty-Git refusal is not bypassed.
	if err := s.Root.WriteFile("passwords/entry.age", []byte("damaged fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Commit([]string{"entry"}, "Fixture corruption"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.HistoryRestore(ref, "entry"); err != nil {
		t.Fatal("could not recover damaged current entry", err)
	}
	value, err = s.Read("entry")
	if err != nil || !bytes.Equal(value, old) {
		t.Fatal("damaged-entry recovery lost bytes")
	}
	if _, err := s.Remove("entry", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.HistoryRestoreConfirmed(ref, "entry", func(plan HistoryRestorePlan) error {
		if plan.Replaces {
			t.Fatal("missing entry reported as replacement")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	value, err = s.Read("entry")
	if err != nil || !bytes.Equal(value, old) {
		t.Fatal("missing-entry recovery lost bytes")
	}
}

func TestHistoryUnavailableWithoutOwnedGit(t *testing.T) {
	s := fixture(t, false)
	ref := strings.Repeat("0", 40)
	_, err := s.HistoryShow(ref)
	var problem *fault.Error
	if !errors.As(err, &problem) || problem.Code != "history.unavailable" {
		t.Fatal("untracked history show did not fail closed", err)
	}
	called := false
	_, err = s.HistoryRestoreConfirmed(ref, "entry", func(HistoryRestorePlan) error { called = true; return nil })
	if !errors.As(err, &problem) || problem.Code != "history.unavailable" || called {
		t.Fatal("untracked history restore reached confirmation", err)
	}
	if err := s.Unlocked(); err != nil {
		t.Fatal(err)
	}
}
