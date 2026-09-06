package store

import (
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestRotationPublicationCopyMismatchPreservesRecovery(t *testing.T) {
	s := fixture(t, false)
	if _, err := s.Write("entry", []byte("preserved"), false); err != nil {
		t.Fatal(err)
	}
	_, err := s.rotate(false, "", false, func(phase string) error {
		if phase == "publication-staged:identities" {
			return errors.New("fixture interruption")
		}
		return nil
	})
	if err == nil {
		t.Fatal("rotation did not stop at publication copy")
	}
	data, err := securefs.Read(s.Root, metadata+"/rotation.json", maxMetadata)
	if err != nil {
		t.Fatal(err)
	}
	var journal Rotation
	if err := StrictJSON(data, &journal); err != nil {
		t.Fatal(err)
	}
	dir := metadata + "/transactions/" + journal.ID
	entries, err := fs.ReadDir(s.Root.FS(), dir)
	if err != nil {
		t.Fatal(err)
	}
	publication := ""
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "publish-") {
			if publication != "" {
				t.Fatal("unexpected additional publication copy")
			}
			publication = dir + "/" + entry.Name()
		}
	}
	if publication == "" {
		t.Fatal("missing owned publication copy")
	}
	original, err := securefs.Read(s.Root, publication, maxMetadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := securefs.Replace(s.Root, publication, []byte("corrupted fixture")); err != nil {
		t.Fatal(err)
	}
	before := transactionFiles(t, s)
	err = s.finishRotation(&journal, nil)
	var problem *fault.Error
	if !errors.As(err, &problem) || problem.Code != "identity.corrupt_rotation" {
		t.Fatal("accepted changed publication copy", err)
	}
	if !reflect.DeepEqual(before, transactionFiles(t, s)) {
		t.Fatal("refusal changed recovery evidence")
	}
	// Restore only this deliberate fixture corruption, then use normal recovery
	// finalization to prove the journal and original after/ bytes remain useful.
	if err := securefs.Replace(s.Root, publication, original); err != nil {
		t.Fatal(err)
	}
	if err := s.finishRotation(&journal, nil); err != nil {
		t.Fatal(err)
	}
	owner, err := s.InspectLock()
	if err != nil || owner == nil {
		t.Fatal("lost lock evidence", err)
	}
	lock := &Lock{store: s, Token: owner.Token, held: true}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	value, err := s.Read("entry")
	if err != nil || string(value) != "preserved" {
		t.Fatal("retry lost entry", err)
	}
	if paths, err := s.inspectStaging(); err != nil || len(paths) != 0 {
		t.Fatal("retry retained key staging", err)
	}
}
