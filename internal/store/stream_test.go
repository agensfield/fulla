package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age/plugin"
	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
)

func TestRecoveryStreamsRejectPluginDebugBeforeExecution(t *testing.T) {
	s := fixture(t, false)
	if _, err := s.Write("entry", []byte("fixture value"), false); err != nil {
		t.Fatal(err)
	}
	ciphertext, err := s.Ciphertext("entry")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "age-plugin-streamfixture"), []byte("#!/bin/sh\nprintf called > \"${0%/*}/executed\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ids, err := crypt.Identities([]byte(plugin.EncodeIdentity("streamfixture", []byte("fixture"))), nil)
	if err != nil {
		t.Fatal(err)
	}
	recipients, err := crypt.Recipients([]byte(plugin.EncodeRecipient("streamfixture", []byte("fixture"))), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("AGEDEBUG", "plugin")
	output := filepath.Join(dir, "export.age")
	target := filepath.Join(dir, "restore")
	for name, operation := range map[string]func() error{
		"logical export": func() error { _, err := s.ExportLogical([]string{"entry"}, recipients, output, nil); return err },
		"full export":    func() error { _, err := s.ExportFull(recipients, output); return err },
		"logical verify": func() error { _, err := VerifyLogical(ciphertext, ids); return err },
		"logical import": func() error { _, err := s.ImportLogical(ciphertext, ids); return err },
		"full restore":   func() error { _, err := RestoreFull(ciphertext, ids, target); return err },
	} {
		t.Run(name, func(t *testing.T) {
			var problem *fault.Error
			if err := operation(); !errors.As(err, &problem) || problem.Code != "plugin.debug_forbidden" {
				t.Fatal("stream bypassed plugin policy", err)
			}
			for _, name := range []string{"executed", "export.age", "restore"} {
				if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
					t.Fatal("forbidden stream executed or published", name, err)
				}
			}
			if err := s.Unlocked(); err != nil {
				t.Fatal("stream retained lock after preflight refusal", err)
			}
		})
	}
}
