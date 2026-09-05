package store

import (
	"errors"
	"testing"

	"github.com/agensfield/fulla/internal/securefs"
)

func TestRotationContinuityAndExplicitDestruction(t *testing.T) {
	s := fixture(t, true)
	if _, err := s.Write("entry", []byte("before-rotation"), false); err != nil {
		t.Fatal(err)
	}
	history, err := s.History("entry")
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.IdentityShow()
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Rotate(false, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.NewFingerprint == old.Fingerprint || result.OldFingerprint != old.Fingerprint {
		t.Fatal(result)
	}
	if _, err := s.Write("entry", []byte("after-rotation"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.HistoryRestore(history[0].Commit, "entry"); err != nil {
		t.Fatal("old history inaccessible", err)
	}
	if _, err := s.Rotate(false, "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.HistoryRestore(history[0].Commit, "entry"); err != nil {
		t.Fatal("retired chain inaccessible", err)
	}
	if _, err := s.Rotate(true, "yes", false); err == nil {
		t.Fatal("accepted routine yes as destruction authority")
	}
	current, err := s.IdentityShow()
	if err != nil {
		t.Fatal(err)
	}
	result, err = s.Rotate(true, "destroy-retired-key:"+current.Fingerprint, true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Destroyed || len(result.AffectedEntries) != 1 {
		t.Fatal(result)
	}
	if _, err := s.HistoryRestore(history[0].Commit, "entry"); err == nil {
		t.Fatal("destroyed key remained available")
	}
	if _, err := s.Read("entry"); err != nil {
		t.Fatal("rotation stranded live entry", err)
	}
}

func TestRotationPublicationRecovery(t *testing.T) {
	for _, phase := range []string{"staged", "published:passwords/entry.age", "published:recipients", "published:identities", "committed"} {
		t.Run(phase, func(t *testing.T) {
			s := fixture(t, true)
			if _, err := s.Write("entry", []byte("value"), false); err != nil {
				t.Fatal(err)
			}
			_, err := s.rotate(false, "", false, func(at string) error {
				if at == phase {
					return errors.New("injected")
				}
				return nil
			})
			if err == nil {
				t.Fatal("did not inject")
			}
			if phase == "staged" {
				if _, err := s.Read("entry"); err != nil {
					t.Fatal("prepublication lost old state", err)
				}
				return
			}
			if _, err := s.Read("entry"); err == nil {
				t.Fatal("interrupted rotation was readable")
			}
			data, err := securefs.Read(s.Root, metadata+"/rotation.json", maxMetadata)
			if err != nil {
				t.Fatal(err)
			}
			var j Rotation
			if err := StrictJSON(data, &j); err != nil {
				t.Fatal(err)
			}
			if err := s.finishRotation(&j, nil); err != nil {
				t.Fatal(err)
			}
			owner, err := securefs.Read(s.Root, "lock/owner", 256)
			if err != nil {
				t.Fatal(err)
			}
			lock := &Lock{store: s, Token: string(owner[:len(owner)-1]), held: true}
			if err := lock.Release(); err != nil {
				t.Fatal(err)
			}
			got, err := s.Read("entry")
			if err != nil || string(got) != "value" {
				t.Fatal("rotation recovery failed", err)
			}
		})
	}
}
