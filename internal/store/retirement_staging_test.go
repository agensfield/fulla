package store

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestDestructiveRotationRecoveryPreservesUnrelatedStaging(t *testing.T) {
	for _, git := range []bool{false, true} {
		t.Run(fmt.Sprintf("git=%t", git), func(t *testing.T) {
			s := fixture(t, git)
			if _, err := s.Write("entry", []byte("unchanged"), false); err != nil {
				t.Fatal(err)
			}
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
				t.Fatal("destructive rotation did not interrupt")
			}
			data, err := securefs.Read(s.Root, metadata+"/rotation.json", maxMetadata)
			if err != nil {
				t.Fatal(err)
			}
			var journal Rotation
			if err := StrictJSON(data, &journal); err != nil {
				t.Fatal(err)
			}
			// Model an extra stage encountered when recovering an older writer.
			// It is intentionally not interpreted as safe-to-delete private data.
			other := metadata + "/transactions/" + securefs.ID()
			if err := s.Root.Mkdir(other, 0700); err != nil {
				t.Fatal(err)
			}
			if err := securefs.WriteNew(s.Root, other+"/private-fixture", []byte("retained evidence")); err != nil {
				t.Fatal(err)
			}
			before := transactionFiles(t, s)
			err = s.finishRotation(&journal, nil)
			var failure *fault.Error
			if !errors.As(err, &failure) || failure.Code != "identity.staging_present" || !reflect.DeepEqual(failure.Details["staging"], []string{other}) {
				t.Fatal("recovery ignored unrelated staging or blamed its own stage", err)
			}
			if !reflect.DeepEqual(before, transactionFiles(t, s)) {
				t.Fatal("refused recovery changed live state, journals, or staging")
			}
		})
	}
}
