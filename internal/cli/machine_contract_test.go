package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agensfield/fulla/internal/store"
)

type unselectedInput struct{ reads int }

func (r *unselectedInput) Read([]byte) (int, error) {
	r.reads++
	return 0, errors.New("fixture input channel was not selected")
}

func machineFiles(t *testing.T, home string) map[string][32]byte {
	t.Helper()
	files := map[string][32]byte{}
	if err := filepath.WalkDir(home, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data := []byte(info.Mode().String() + "\x00")
		if !entry.IsDir() {
			contents, err := os.ReadFile(name)
			if err != nil {
				return err
			}
			data = append(data, contents...)
		}
		files[name] = sha256.Sum256(data)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

func TestCanonicalMachineCommandsDoNotReadImplicitStdin(t *testing.T) {
	for _, git := range []bool{false, true} {
		t.Run(fmt.Sprintf("git=%v", git), func(t *testing.T) {
			home, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(home, "store")
			if _, err := store.Init(dir, !git, false); err != nil {
				t.Fatal(err)
			}
			s, err := store.Open(dir, true, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			value := []byte("fixture secret\x00\xff\n")
			added, err := s.Write("entry", value, false)
			if err != nil {
				t.Fatal(err)
			}
			identity, err := s.IdentityShow()
			if err != nil {
				t.Fatal(err)
			}
			if err := s.SavePeer(store.Peer{Version: 1, Name: "fixture", Host: "fixture", Recipient: identity.Recipient, Fingerprint: identity.Fingerprint}, false, ""); err != nil {
				t.Fatal(err)
			}
			commit := strings.Repeat("a", 40)
			historyStatus, historyCode := 1, "history.unavailable"
			removeCode := "interaction.required"
			if git {
				commit, err = s.Head()
				if err != nil {
					t.Fatal(err)
				}
				historyStatus, historyCode = 0, ""
				removeCode = "entry.not_found"
			}
			cases := []struct {
				command string
				args    []string
				status  int
				code    string
			}{
				{"init", nil, 1, "interaction.required"},
				{"add", []string{"new"}, 1, "interaction.required"},
				{"show", []string{"entry"}, 0, ""},
				{"copy", nil, 2, "invocation.invalid"},
				{"edit", []string{"entry"}, 1, "interaction.required"},
				{"list", nil, 0, ""},
				{"remove", []string{"missing"}, 1, removeCode},
				{"move", []string{"entry", "entry"}, 1, "entry.exists"},
				{"run", nil, 2, "invocation.invalid"},
				{"sync", []string{"missing"}, 1, "peer.not_found"},
				{"peer add", nil, 2, "invocation.invalid"},
				{"peer list", nil, 0, ""},
				{"peer show", []string{"fixture"}, 0, ""},
				{"peer rotate", nil, 2, "invocation.invalid"},
				{"peer remove", []string{"fixture"}, 1, "interaction.required"},
				{"identity show", nil, 0, ""},
				{"identity rotate", nil, 1, "interaction.required"},
				{"transfer export", []string{"--output", filepath.Join(home, "export.age")}, 1, "interaction.required"},
				{"transfer verify", []string{filepath.Join(home, "missing.age")}, 1, "interaction.required"},
				{"transfer import", nil, 2, "invocation.invalid"},
				{"history list", nil, historyStatus, historyCode},
				{"history show", []string{commit}, historyStatus, historyCode},
				{"history restore", []string{commit, "entry"}, 1, "interaction.required"},
				{"backup list", nil, 0, ""},
				{"backup show", []string{added.Transaction}, 0, ""},
				{"backup restore", []string{added.Transaction}, 1, "interaction.required"},
				{"backup prune", []string{"--keep", "0"}, 0, ""},
				{"backup export", []string{"--full", "--output", filepath.Join(home, "full.age")}, 1, "interaction.required"},
				{"status", nil, 0, ""},
				{"doctor", nil, 0, ""},
				{"git", nil, 2, "invocation.invalid"},
				{"completion", []string{"bash"}, 2, "invocation.invalid"},
				{"version", nil, 0, ""},
				{"remote serve", nil, 2, "invocation.invalid"},
			}
			for _, mode := range []string{"--json", "--non-interactive"} {
				for _, tc := range cases {
					if mode == "--non-interactive" && (tc.command == "run" || tc.command == "git" || tc.command == "completion" || tc.command == "remote serve") {
						continue
					}
					t.Run(mode+"/"+tc.command, func(t *testing.T) {
						before := machineFiles(t, home)
						input := &unselectedInput{}
						var stdout, stderr bytes.Buffer
						app := App{In: input, Out: &stdout, Err: &stderr, Getenv: func(key string) string {
							if key == "HOME" {
								return home
							}
							return ""
						}}
						args := append([]string{"--store", dir, mode}, strings.Fields(tc.command)...)
						args = append(args, tc.args...)
						code := app.Main(args)
						if code != tc.status {
							t.Fatalf("status %d, wanted %d: %s %s", code, tc.status, stdout.Bytes(), stderr.Bytes())
						}
						if input.reads != 0 {
							t.Fatal("read an unselected stdin channel")
						}
						if bytes.Contains(stderr.Bytes(), []byte("fixture secret")) || (tc.command != "show" && (bytes.Contains(stdout.Bytes(), []byte("fixture secret")) || bytes.Contains(stdout.Bytes(), []byte(base64.StdEncoding.EncodeToString(value))))) {
							t.Fatal("secret value appeared outside explicit show output")
						}
						if !reflect.DeepEqual(before, machineFiles(t, home)) {
							t.Fatal("read/refusal changed the fixture tree or modes")
						}
						if mode == "--non-interactive" {
							if tc.command == "show" && !bytes.Equal(stdout.Bytes(), value) {
								t.Fatal("raw show lost exact bytes")
							}
							return
						}
						var envelope struct {
							Schema   string          `json:"schema"`
							OK       bool            `json:"ok"`
							Command  string          `json:"command"`
							Data     json.RawMessage `json:"data"`
							Warnings []any           `json:"warnings"`
							Error    *struct {
								Code    string         `json:"code"`
								Message string         `json:"message"`
								Details map[string]any `json:"details"`
							} `json:"error"`
						}
						decoder := json.NewDecoder(&stdout)
						if err := decoder.Decode(&envelope); err != nil {
							t.Fatal(err)
						}
						if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
							t.Fatal("not exactly one JSON document", err)
						}
						if envelope.Schema != "fulla.cli/v1" || envelope.Command != tc.command || envelope.OK != (code == 0) {
							t.Fatal("wrong envelope", envelope)
						}
						if code == 0 {
							if len(envelope.Data) == 0 || envelope.Warnings == nil || envelope.Error != nil {
								t.Fatal("incomplete success envelope")
							}
							if tc.command == "backup prune" {
								var result struct {
									DryRun bool `json:"dry_run"`
								}
								if err := json.Unmarshal(envelope.Data, &result); err != nil || !result.DryRun {
									t.Fatal("prune did not remain a preview", err)
								}
							}
						} else if envelope.Error == nil || envelope.Error.Code != tc.code || envelope.Error.Message == "" || envelope.Error.Details == nil {
							t.Fatal("wrong typed failure", envelope)
						}
					})
				}
			}
		})
	}
}
