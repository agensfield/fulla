package crypt

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age/plugin"
	"github.com/agensfield/fulla/internal/fault"
)

func TestMissingPluginIsPreciseAndDebugModeFailsBeforeExecution(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("AGEDEBUG", "")
	ids, err := Identities([]byte(plugin.EncodeIdentity("fixture", []byte("private-fixture-payload"))), nil)
	if err != nil {
		t.Fatal(err)
	}
	rs, err := Recipients([]byte(plugin.EncodeRecipient("fixture", []byte("public-fixture-payload"))), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Encrypt([]byte("synthetic-value"), rs)
	var problem *fault.Error
	if !errors.As(err, &problem) || problem.Code != "plugin.missing" || problem.Details["executable"] != "age-plugin-fixture" {
		t.Fatal("missing-plugin classification", err)
	}
	_, native, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	nativeRecipients, err := Recipients([]byte(native), nil)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := Encrypt([]byte("synthetic-value"), nativeRecipients)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Decrypt(ciphertext, ids)
	if !errors.As(err, &problem) || problem.Code != "plugin.missing" {
		t.Fatal("decrypt missing-plugin classification", err)
	}
	executable := filepath.Join(dir, "age-plugin-fixture")
	marker := filepath.Join(dir, "executed")
	script := "#!/bin/sh\nprintf called > \"${0%/*}/executed\"\n"
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGEDEBUG", "plugin")
	for _, operation := range []func() error{func() error { _, e := Encrypt([]byte("synthetic-value"), rs); return e }, func() error { _, e := Decrypt(ciphertext, ids); return e }} {
		if err := operation(); !errors.As(err, &problem) || problem.Code != "plugin.debug_forbidden" {
			t.Fatal("unsafe debug mode accepted", err)
		}
	}
	statuses := Plugins(ids, rs)
	if len(statuses) != 1 || !statuses[0].Available || statuses[0].Issue != "plugin.debug_forbidden" {
		t.Fatal(statuses)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("plugin executed during inspection or forbidden debug", err)
	}
}
