package store

import (
	"bytes"
	"errors"
	"testing"
)

func TestInteractiveWriteLocksCurrentValueAndReleasesOnFailure(t *testing.T) {
	s := fixture(t, true)
	original := []byte{0, 255, 10, 10}
	if _, err := s.Write("entry", original, false); err != nil {
		t.Fatal(err)
	}
	other, err := Open(s.Dir, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	cancelled := errors.New("fixture cancellation")
	_, err = s.WriteInteractive("entry", true, func(value []byte) ([]byte, error) {
		if !bytes.Equal(value, original) {
			t.Fatal("editor received altered original")
		}
		if _, err := other.Write("entry", []byte("concurrent"), true); err == nil {
			t.Fatal("concurrent edit bypassed lock")
		}
		return nil, cancelled
	})
	if !errors.Is(err, cancelled) {
		t.Fatal(err)
	}
	if err := s.Unlocked(); err != nil {
		t.Fatal(err)
	}
	value, err := s.Read("entry")
	if err != nil || !bytes.Equal(value, original) {
		t.Fatal("failed editor changed entry", err)
	}
	replacement := []byte{255, 0, 10}
	if _, err := s.WriteInteractive("entry", true, func([]byte) ([]byte, error) { return replacement, nil }); err != nil {
		t.Fatal(err)
	}
	value, err = s.Read("entry")
	if err != nil || !bytes.Equal(value, replacement) {
		t.Fatal("successful editor lost bytes", err)
	}
	called := false
	if _, err := s.WriteInteractive("entry", false, func([]byte) ([]byte, error) { called = true; return nil, nil }); err == nil || called {
		t.Fatal("duplicate add called editor")
	}
	if _, err := s.WriteInteractive("absent", true, func([]byte) ([]byte, error) { called = true; return nil, nil }); err == nil || called {
		t.Fatal("missing edit called editor")
	}
}
