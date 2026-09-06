package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"filippo.io/age/plugin"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestMutationDomainsRefuseBeforeSecretAccess(t *testing.T) {
	bin := t.TempDir()
	marker := filepath.Join(bin, "attempts")
	if err := os.WriteFile(filepath.Join(bin, "age-plugin-mutationsentinel"), []byte("#!/bin/sh\nprintf 'invoked\\n' >> \"${0%/*}/attempts\"\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AGEDEBUG", "")
	for _, git := range []bool{false, true} {
		t.Run(fmt.Sprintf("git=%v", git), func(t *testing.T) {
			s := fixture(t, git)
			mutation, err := s.Write("present", []byte("synthetic-value"), false)
			if err != nil {
				t.Fatal(err)
			}
			commit := ""
			if git {
				commit, err = s.Head()
				if err != nil {
					t.Fatal(err)
				}
			}
			_, recipients, err := s.Keys()
			if err != nil {
				t.Fatal(err)
			}
			var bundle bytes.Buffer
			if _, err := s.ExportLogical([]string{"present"}, recipients, "-", &bundle); err != nil {
				t.Fatal(err)
			}
			identity := plugin.EncodeIdentity("mutationsentinel", []byte("synthetic-identity")) + "\n"
			if err := securefs.Replace(s.Root, "identities", []byte(identity)); err != nil {
				t.Fatal(err)
			}
			// A sole plugin identity makes attempted decryption observable. This real
			// read is the positive control, independently of the metadata refusals.
			if _, err := s.Read("present"); err == nil {
				t.Fatal("sentinel decrypted")
			}
			if data, err := os.ReadFile(marker); err != nil || string(data) != "invoked\n" {
				t.Fatal("plugin positive control failed", err)
			}
			if err := os.Remove(marker); err != nil {
				t.Fatal(err)
			}
			for _, domain := range []string{"missing-transactions", "missing-backup"} {
				t.Run(domain, func(t *testing.T) {
					next := s.Meta
					next.Domains = map[string]int{}
					for name, version := range s.Meta.Domains {
						next.Domains[name] = version
					}
					if domain == "missing-transactions" {
						delete(next.Domains, "transactions")
					} else {
						delete(next.Domains, "backup")
					}
					data, err := json.Marshal(next)
					if err != nil {
						t.Fatal(err)
					}
					if err := securefs.Replace(s.Root, metadata+"/store.json", data); err != nil {
						t.Fatal(err)
					}
					before := archiveTree(t, s, false)
					called := false
					input := func([]byte) ([]byte, error) { called = true; return []byte("replacement"), nil }
					cases := []struct {
						name string
						run  func() error
					}{
						{"add", func() error { _, err := s.Write("new", []byte("replacement"), false); return err }},
						{"edit", func() error { _, err := s.Write("present", []byte("replacement"), true); return err }},
						{"interactive-add", func() error { _, err := s.WriteInteractive("new", false, input); return err }},
						{"interactive-edit", func() error { _, err := s.WriteInteractive("present", true, input); return err }},
						{"remove", func() error { _, err := s.Remove("present", true); return err }},
						{"move", func() error { _, err := s.Move("present", "moved"); return err }},
						{"backup-restore", func() error {
							_, err := s.BackupRestoreConfirmed(mutation.Transaction, "after", func(BackupRestorePlan) error { called = true; return nil })
							return err
						}},
						{"import", func() error { _, err := s.ImportLogical(bundle.Bytes(), nil); return err }},
					}
					if git {
						cases = append(cases, struct {
							name string
							run  func() error
						}{"history-restore", func() error {
							_, err := s.HistoryRestoreConfirmed(commit, "present", func(HistoryRestorePlan) error { called = true; return nil })
							return err
						}})
					}
					for _, tc := range cases {
						t.Run(tc.name, func(t *testing.T) {
							err := tc.run()
							var problem *fault.Error
							if !errors.As(err, &problem) || problem.Code != "metadata.unsupported" {
								t.Fatal("wrong refusal", err)
							}
							if called {
								t.Fatal("refusal requested input or confirmation")
							}
							if _, err := os.Stat(marker); !os.IsNotExist(err) {
								t.Fatal("refusal invoked plugin", err)
							}
							if !reflect.DeepEqual(before, archiveTree(t, s, false)) {
								t.Fatal("refusal changed store paths, modes or bytes")
							}
						})
					}
				})
			}
		})
	}
}
