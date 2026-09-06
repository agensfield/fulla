package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"filippo.io/age/plugin"
	"github.com/agensfield/fulla/internal/securefs"
	"github.com/agensfield/fulla/internal/store"
)

func TestRunRefusesInvalidMappingsBeforeDecryption(t *testing.T) {
	bin := t.TempDir()
	marker := filepath.Join(bin, "attempts")
	if err := os.WriteFile(filepath.Join(bin, "age-plugin-runsentinel"), []byte("#!/bin/sh\nprintf 'invoked\\n' >> \"${0%/*}/attempts\"\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AGEDEBUG", "")
	for _, noGit := range []bool{false, true} {
		t.Run(fmt.Sprintf("no-git=%v", noGit), func(t *testing.T) {
			home, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(home, "store")
			if _, err := store.Init(dir, noGit, false); err != nil {
				t.Fatal(err)
			}
			s, err := store.Open(dir, true, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if _, err := s.Write("present", []byte("synthetic-run-secret"), false); err != nil {
				t.Fatal(err)
			}
			identity := plugin.EncodeIdentity("runsentinel", []byte("synthetic-identity")) + "\n"
			if err := securefs.Replace(s.Root, "identities", []byte(identity)); err != nil {
				t.Fatal(err)
			}
			// A real encrypted entry and sole plugin identity make premature
			// decryption observable. Prove the sentinel works before refusals.
			if _, err := s.Read("present"); err == nil {
				t.Fatal("sentinel unexpectedly decrypted")
			}
			if data, err := os.ReadFile(marker); err != nil || string(data) != "invoked\n" {
				t.Fatal("positive control did not invoke the plugin once", err)
			}
			if err := os.Remove(marker); err != nil {
				t.Fatal(err)
			}
			before := machineFiles(t, dir)
			for _, tc := range []struct {
				name string
				args []string
				code string
			}{
				{"missing-command", nil, "invocation.invalid"},
				{"missing-equals", []string{"--env", "BROKEN"}, "invocation.invalid"},
				{"invalid-env", []string{"--env", "1BAD=present"}, "invocation.invalid"},
				{"empty-env", []string{"--env", "=present"}, "invocation.invalid"},
				{"empty-entry", []string{"--env", "OTHER="}, "invocation.invalid"},
				{"traversal", []string{"--env", "OTHER=../present"}, "invocation.invalid"},
				{"duplicate-env", []string{"--env", "TOKEN=present"}, "invocation.invalid"},
				{"inherit-without-clean", []string{"--inherit", "PATH"}, "invocation.invalid"},
				{"invalid-inherit", []string{"--clean-env", "--inherit", "BAD-NAME"}, "invocation.invalid"},
				{"missing-entry", []string{"--env", "OTHER=missing"}, "entry.not_found"},
				{"missing-executable", nil, "run.executable_missing"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					args := []string{"--store", dir, "--non-interactive", "run", "--env", "TOKEN=present"}
					args = append(args, tc.args...)
					if tc.name != "missing-command" {
						target := os.Args[0]
						if tc.name == "missing-executable" {
							target = filepath.Join(home, "absent-executable")
						}
						args = append(args, "--", target)
					}
					var out, diagnostic bytes.Buffer
					input := &unselectedInput{}
					a := App{In: input, Out: &out, Err: &diagnostic, Getenv: func(key string) string {
						if key == "HOME" {
							return home
						}
						return ""
					}}
					exit := a.Main(args)
					if exit == 0 || !strings.Contains(diagnostic.String(), tc.code) || out.Len() != 0 || input.reads != 0 {
						t.Fatalf("wrong refusal: exit=%d diagnostic=%q", exit, diagnostic.String())
					}
					if _, err := os.Stat(marker); !os.IsNotExist(err) {
						t.Fatal("refusal attempted decryption", err)
					}
					if !reflect.DeepEqual(before, machineFiles(t, dir)) {
						t.Fatal("refusal changed the store")
					}
				})
			}
		})
	}
}
