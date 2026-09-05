package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/agensfield/fulla/internal/securefs"
)

func TestFullDisasterRestoreAndCircularProtection(t *testing.T) {
	source := fixture(t, true)
	recovery := fixture(t, false)
	if _, err := source.Write("entry", []byte{0, 255, 10}, false); err != nil {
		t.Fatal(err)
	}
	_, active, err := source.Keys()
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(filepath.Dir(source.Dir), "full.age")
	if _, err := source.ExportFull(active, output); err == nil {
		t.Fatal("accepted circular recovery")
	}
	ids, rs, err := recovery.Keys()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.ExportFull(rs, output); err != nil {
		t.Fatal(err)
	}
	data, err := ReadArtifact(output, MaxBundleBytes)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(filepath.Dir(recovery.Dir), "restored")
	if _, err := RestoreFull(data, ids, target); err != nil {
		t.Fatal(err)
	}
	s, err := Open(target, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Read("entry")
	if err != nil || !bytes.Equal(got, []byte{0, 255, 10}) {
		t.Fatal("recovery lost entry", err)
	}
	before, err := securefs.Read(source.Root, "identities", maxMetadata)
	if err != nil {
		t.Fatal(err)
	}
	after, err := securefs.Read(s.Root, "identities", maxMetadata)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("did not restore original identity", err)
	}
	if _, err := RestoreFull(data, ids, target); err == nil {
		t.Fatal("overwrote restore target")
	}
	if _, err := s.CleanGit(); err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 1
	bad := filepath.Join(filepath.Dir(recovery.Dir), "bad")
	if _, err := RestoreFull(data, ids, bad); err == nil {
		t.Fatal("accepted corrupt full archive")
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatal("published corrupt restored state")
	}
}
