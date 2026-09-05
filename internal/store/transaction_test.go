package store

import (
	"bytes"
	"errors"
	"testing"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestCRUDExactBytesAndStrictSemantics(t *testing.T) {
	for _, git := range []bool{false, true} {
		s := fixture(t, git)
		for _, value := range [][]byte{nil, []byte("hello\n\n"), {0, 255, 254, 10}} {
			if _, err := s.Write("nested/entry", value, false); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Write("nested/entry", []byte("overwrite"), false); err == nil {
				t.Fatal("upsert accepted")
			}
			got, err := s.Read("nested/entry")
			if err != nil || !bytes.Equal(got, value) {
				t.Fatal("byte mismatch", err)
			}
			if _, err := s.Write("absent", value, true); err == nil {
				t.Fatal("edit created entry")
			}
			if _, err := s.Write("nested/entry", []byte("edited"), true); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Move("nested/entry", "moved"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Move("moved", "moved"); err == nil {
				t.Fatal("move overwrote destination")
			}
			if !git {
				if _, err := s.Remove("moved", false); err == nil {
					t.Fatal("unacknowledged permanent deletion")
				}
			}
			if _, err := s.Remove("moved", true); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Read("moved"); err == nil {
				t.Fatal("deleted entry still live")
			}
			if _, err := s.CleanGit(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestJournalRecoveryAtPublishBoundaries(t *testing.T) {
	for _, phase := range []string{"staged", "published:a", "published:b", "committed", "receipted"} {
		t.Run(phase, func(t *testing.T) {
			s := fixture(t, true)
			if _, err := s.Write("a", []byte("old"), false); err != nil {
				t.Fatal(err)
			}
			_, rs, err := s.Keys()
			if err != nil {
				t.Fatal(err)
			}
			ciphertext, err := crypt.Encrypt([]byte("new"), rs)
			if err != nil {
				t.Fatal(err)
			}
			lock, err := s.Lock("test")
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.mutate(lock, "test", map[string][]byte{"a": nil, "b": ciphertext}, func(at string) error {
				if at == phase {
					return errors.New("injected failure")
				}
				return nil
			})
			if err == nil {
				t.Fatal("injection did not fail")
			}
			if phase == "staged" {
				got, err := s.Read("a")
				if err != nil || string(got) != "old" {
					t.Fatal("prepublication changed state", err)
				}
				return
			}
			if _, err := s.Read("a"); err == nil {
				t.Fatal("allowed read of interrupted transaction")
			}
			data, err := securefs.Read(s.Root, metadata+"/pending.json", maxMetadata)
			if err != nil {
				t.Fatal(err)
			}
			var j Journal
			if err := StrictJSON(data, &j); err != nil {
				t.Fatal(err)
			}
			if err := s.finishJournal(&j, nil); err != nil {
				t.Fatal(err)
			}
			if err := lock.Release(); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Read("a"); err == nil {
				t.Fatal("old path remains")
			}
			got, err := s.Read("b")
			if err != nil || string(got) != "new" {
				t.Fatal("recovery failed", err)
			}
			if _, err := s.CleanGit(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRecoveryRejectsUnrelatedCiphertext(t *testing.T) {
	s := fixture(t, false)
	if _, err := s.Write("a", []byte("old"), false); err != nil {
		t.Fatal(err)
	}
	_, rs, err := s.Keys()
	if err != nil {
		t.Fatal(err)
	}
	c, err := crypt.Encrypt([]byte("new"), rs)
	if err != nil {
		t.Fatal(err)
	}
	l, err := s.Lock("test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.mutate(l, "test", map[string][]byte{"a": c}, func(at string) error {
		if at == "published:a" {
			return errors.New("fault")
		}
		return nil
	})
	if err == nil {
		t.Fatal("expected fault")
	}
	if err := securefs.Replace(s.Root, "passwords/a.age", []byte("unrelated")); err != nil {
		t.Fatal(err)
	}
	data, err := securefs.Read(s.Root, metadata+"/pending.json", maxMetadata)
	if err != nil {
		t.Fatal(err)
	}
	var j Journal
	if err := StrictJSON(data, &j); err != nil {
		t.Fatal(err)
	}
	if err := s.finishJournal(&j, nil); err == nil {
		t.Fatal("overwrote external change")
	}
}
