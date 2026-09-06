package store

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestAtomicKeyStagingBlocksDestructiveRotation(t *testing.T) {
	for _, git := range []bool{false, true} {
		for _, directory := range []string{".", metadata + "/retired"} {
			t.Run(fmt.Sprintf("git=%t/%s", git, directory), func(t *testing.T) {
				s := fixture(t, git)
				identity, err := s.IdentityShow()
				if err != nil {
					t.Fatal(err)
				}
				// Reconstruct the durable file at the atomic replacement's
				// write-before-rename boundary. No crash-timing claim is made.
				private, err := securefs.Read(s.Root, "identities", maxMetadata)
				if err != nil {
					t.Fatal(err)
				}
				name := ".fulla-stage-" + securefs.ID()
				if directory != "." {
					name = directory + "/" + name
				}
				if err := securefs.WriteNew(s.Root, name, private); err != nil {
					t.Fatal(err)
				}
				before := transactionFiles(t, s)
				report, err := s.Doctor(false)
				if err != nil || report.Healthy || !reflect.DeepEqual(report.Staging, []string{name}) {
					t.Fatal("atomic key copy absent from diagnostics", err, report.Staging)
				}
				_, err = s.Rotate(true, "destroy-retired-key:"+identity.Fingerprint, false)
				var failure *fault.Error
				if !errors.As(err, &failure) || failure.Code != "identity.staging_present" || failure.Details["applied"] != false || !reflect.DeepEqual(failure.Details["staging"], []string{name}) {
					t.Fatal("destructive rotation ignored atomic key copy", err)
				}
				if !reflect.DeepEqual(before, transactionFiles(t, s)) {
					t.Fatal("refusal changed key material or evidence")
				}
			})
		}
	}
}

func TestAtomicKeyStagingBlocksDestructiveRecovery(t *testing.T) {
	for _, git := range []bool{false, true} {
		t.Run(fmt.Sprintf("git=%t", git), func(t *testing.T) {
			s := fixture(t, git)
			identity, err := s.IdentityShow()
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.rotate(true, "destroy-retired-key:"+identity.Fingerprint, false, func(phase string) error {
				if phase == "journaled" {
					return errors.New("fixture interruption")
				}
				return nil
			})
			if err == nil {
				t.Fatal("rotation did not interrupt")
			}
			data, err := securefs.Read(s.Root, metadata+"/rotation.json", maxMetadata)
			if err != nil {
				t.Fatal(err)
			}
			var journal Rotation
			if err := StrictJSON(data, &journal); err != nil {
				t.Fatal(err)
			}
			name := ".fulla-stage-" + securefs.ID()
			if err := securefs.WriteNew(s.Root, name, []byte("retained key-copy fixture")); err != nil {
				t.Fatal(err)
			}
			before := transactionFiles(t, s)
			err = s.finishRotation(&journal, nil)
			var failure *fault.Error
			if !errors.As(err, &failure) || failure.Code != "identity.staging_present" || !reflect.DeepEqual(failure.Details["staging"], []string{name}) {
				t.Fatal("recovery ignored atomic staging or blamed its owned stage", err)
			}
			if !reflect.DeepEqual(before, transactionFiles(t, s)) {
				t.Fatal("refused recovery changed evidence")
			}
		})
	}
}
