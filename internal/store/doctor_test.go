package store

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age/plugin"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestDoctorInspectsPluginWithoutDecryptingOrExecuting(t *testing.T) {
	s := fixture(t, false)
	if _, err := s.Write("entry", []byte("private-entry-fixture"), false); err != nil {
		t.Fatal(err)
	}
	private := plugin.EncodeIdentity("fixture", []byte("private-identity-fixture"))
	public := plugin.EncodeRecipient("fixture", []byte("public-fixture"))
	if err := securefs.Replace(s.Root, "identities", []byte(private)); err != nil {
		t.Fatal(err)
	}
	if err := securefs.Replace(s.Root, "recipients", []byte(public)); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("AGEDEBUG", "")
	r, err := s.Doctor(false)
	if err != nil || r.Healthy || len(r.Plugins) != 1 || r.Plugins[0].Issue != "plugin.missing" {
		t.Fatal(r, err)
	}
	executable := filepath.Join(dir, "age-plugin-fixture")
	marker := filepath.Join(dir, "executed")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf called > '"+marker+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	r, err = s.Doctor(false)
	if err != nil || !r.Healthy || !r.IdentityValid || !r.RecipientsValid || !r.Plugins[0].Available {
		t.Fatal(r, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("structural doctor executed plugin", err)
	}
	data, err := json.Marshal(r)
	if err != nil || bytes.Contains(data, []byte(private)) || bytes.Contains(data, []byte("private-entry-fixture")) || bytes.Contains(data, []byte("private-identity-fixture")) {
		t.Fatal("diagnostics exposed private material", err)
	}
}
