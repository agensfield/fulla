package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestPublishedJournalFailurePreservesRecovery(t *testing.T) {
	for _, git := range []bool{false, true} {
		for _, rotation := range []bool{false, true} {
			t.Run(fmt.Sprintf("git=%t/rotation=%t", git, rotation), func(t *testing.T) {
				s := fixture(t, git)
				if _, err := s.Write("entry", []byte("original"), false); err != nil {
					t.Fatal(err)
				}
				before, err := s.Ciphertext("entry")
				if err != nil {
					t.Fatal(err)
				}
				identity, err := securefs.Read(s.Root, "identities", maxMetadata)
				if err != nil {
					t.Fatal(err)
				}
				hook := func(phase string) error {
					if phase == "journaled" {
						return errors.New("synthetic-private-publication-error")
					}
					return nil
				}
				if rotation {
					_, err = s.rotate(false, "", false, hook)
				} else {
					_, rs, e := s.Keys()
					if e != nil {
						t.Fatal(e)
					}
					ciphertext, e := crypt.Encrypt([]byte("replacement"), rs)
					if e != nil {
						t.Fatal(e)
					}
					lock, e := s.Lock("fixture edit")
					if e != nil {
						t.Fatal(e)
					}
					_, err = s.mutate(lock, "fixture edit", map[string][]byte{"entry": ciphertext}, hook)
				}
				var failure *fault.Error
				if !errors.As(err, &failure) || failure.Code != "transaction.incomplete" || failure.Status != 3 || failure.Details["applied"] != false || failure.Details["recovery_required"] != true {
					t.Fatal("published journal failure lost its recovery state", err)
				}
				encoded, err := json.Marshal(failure)
				if err != nil || bytes.Contains(encoded, []byte("synthetic-private")) {
					t.Fatal("failure leaked raw publication error", err)
				}
				id, ok := failure.Details["transaction"].(string)
				if !ok || !validID(id) {
					t.Fatal("missing recovery transaction")
				}
				if _, err := s.Root.Lstat(metadata + "/transactions/" + id + "/after"); err != nil {
					t.Fatal("published journal lost staging", err)
				}
				after, err := s.Ciphertext("entry")
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("live entry published prematurely", err)
				}
				after, err = securefs.Read(s.Root, "identities", maxMetadata)
				if err != nil || !bytes.Equal(identity, after) {
					t.Fatal("live identity published prematurely", err)
				}
				owner, err := s.InspectLock()
				if err != nil || owner == nil || !owner.Alive {
					t.Fatal("published journal lost owned lock", err)
				}
				if _, err := s.Read("entry"); err == nil {
					t.Fatal("ordinary read ignored pending recovery")
				}
				journalPath := metadata + "/pending.json"
				if rotation {
					journalPath = metadata + "/rotation.json"
				}
				data, err := securefs.Read(s.Root, journalPath, maxMetadata)
				if err != nil {
					t.Fatal(err)
				}
				if rotation {
					var journal Rotation
					if err := StrictJSON(data, &journal); err != nil || journal.ID != id || journal.Phase != "prepared" {
						t.Fatal("invalid retained rotation", err)
					}
					err = s.finishRotation(&journal, nil)
				} else {
					var journal Journal
					if err := StrictJSON(data, &journal); err != nil || journal.ID != id || journal.Phase != "prepared" {
						t.Fatal("invalid retained transaction", err)
					}
					err = s.finishJournal(&journal, nil)
				}
				if err != nil {
					t.Fatal("retained journal could not finish", err)
				}
				lock := &Lock{store: s, Token: owner.Token, held: true}
				if err := lock.Release(); err != nil {
					t.Fatal(err)
				}
				value, err := s.Read("entry")
				expected := "replacement"
				if rotation {
					expected = "original"
				}
				if err != nil || string(value) != expected {
					t.Fatal("journal recovery changed intended value", err)
				}
				if err := s.DeepVerify(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
