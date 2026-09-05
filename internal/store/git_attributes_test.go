package store

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/agensfield/fulla/internal/securefs"
)

func TestPaGitAttributesPreservedAcrossEntryRestore(t *testing.T) {
	s := fixture(t, true)
	attributes := []byte("*.age diff=age\n")
	if err := securefs.WriteNew(s.Root, "passwords/.gitattributes", attributes); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Git("add", "--", ".gitattributes"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Git("-c", "user.name=Fixture", "-c", "user.email=fixture@localhost", "commit", "-m", "pa attributes fixture"); err != nil {
		t.Fatal(err)
	}
	value := []byte{0, 255, 10}
	created, err := s.Write("entry", value, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write("entry", []byte("replacement"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BackupRestore(created.Transaction, "after"); err != nil {
		t.Fatal("pa snapshot restore rejected Git metadata", err)
	}
	names, err := s.Names()
	if err != nil || !reflect.DeepEqual(names, []string{"entry"}) {
		t.Fatal("Git metadata exposed as an entry", names, err)
	}
	got, err := s.Read("entry")
	if err != nil || !bytes.Equal(got, value) {
		t.Fatal("restored bytes differ", err)
	}
	got, err = securefs.Read(s.Root, "passwords/.gitattributes", maxMetadata)
	if err != nil || !bytes.Equal(got, attributes) {
		t.Fatal("entry restore changed Git attributes", err)
	}
}

func TestGitAttributesExceptionDoesNotAdmitArbitraryFiles(t *testing.T) {
	for _, name := range []string{"passwords/unexpected", "passwords/nested/.gitattributes"} {
		t.Run(name, func(t *testing.T) {
			s := fixture(t, false)
			if err := s.Root.MkdirAll("passwords/nested", 0700); err != nil {
				t.Fatal(err)
			}
			if err := securefs.WriteNew(s.Root, name, []byte("fixture")); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Names(); err == nil {
				t.Fatal("accepted unrelated metadata file")
			}
		})
	}
}
