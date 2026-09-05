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

func TestDeepDoctorReportsEveryEntryWithoutValues(t *testing.T) {
	s := fixture(t, false)
	secret := []byte("deep-doctor-value-must-not-appear")
	for _, name := range []string{"a-broken", "b-valid", "c-broken"} {
		if _, err := s.Write(name, secret, false); err != nil {
			t.Fatal(err)
		}
	}
	// Preserve valid age headers while corrupting payload authentication, so
	// structural inspection alone cannot discover these failures.
	for _, name := range []string{"a-broken", "c-broken"} {
		data, err := s.Ciphertext(name)
		if err != nil {
			t.Fatal(err)
		}
		data[len(data)-1] ^= 1
		if err := securefs.Replace(s.Root, "passwords/"+name+".age", data); err != nil {
			t.Fatal(err)
		}
	}
	ordinary, err := s.Doctor(false)
	if err != nil || !ordinary.Healthy || len(ordinary.Verification) != 0 {
		t.Fatalf("structural inspection decrypted content: %+v %v", ordinary, err)
	}
	deep, err := s.Doctor(true)
	if err != nil {
		t.Fatal(err)
	}
	if deep.Healthy || len(deep.Verification) != 3 {
		t.Fatalf("incomplete verification: %+v", deep)
	}
	for i, result := range deep.Verification {
		if result.OK != (i == 1) || (i != 1 && result.Error == "") {
			t.Fatalf("wrong result %+v", result)
		}
	}
	encoded, err := json.Marshal(deep)
	if err != nil {
		t.Fatal(err)
	}
	private, err := s.Root.ReadFile("identities")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, secret) || bytes.Contains(encoded, private) {
		t.Fatal("verification leaked private material")
	}
}
