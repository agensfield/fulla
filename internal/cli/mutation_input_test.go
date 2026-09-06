package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/agensfield/fulla/internal/securefs"
	"github.com/agensfield/fulla/internal/store"
)

func TestUnsupportedMutationLeavesSelectedInputUntouched(t *testing.T) {
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
			if _, err := s.Write("present", []byte("fixture"), false); err != nil {
				t.Fatal(err)
			}
			secret := filepath.Join(home, "input")
			if err := os.WriteFile(secret, []byte("synthetic secret input never consumed"), 0600); err != nil {
				t.Fatal(err)
			}
			for _, domain := range []string{"missing-transactions", "missing-backup"} {
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
				if err := securefs.Replace(s.Root, ".fulla/store.json", data); err != nil {
					t.Fatal(err)
				}
				before := machineFiles(t, home)
				for _, tc := range []struct {
					name string
					args []string
					flag string
				}{
					{"add-stdin", []string{"add", "new"}, "--stdin"},
					{"edit-stdin", []string{"edit", "present"}, "--stdin"},
					{"add-fd", []string{"add", "new"}, "--from-fd"},
					{"edit-fd", []string{"edit", "present"}, "--from-fd"},
					{"import-passphrase", []string{"transfer", "import", filepath.Join(home, "bundle.age")}, "--passphrase-fd"},
				} {
					for _, mode := range []string{"--json", "--non-interactive"} {
						t.Run(domain+"/"+tc.name+"/"+mode, func(t *testing.T) {
							fd, err := os.Open(secret)
							if err != nil {
								t.Fatal(err)
							}
							defer fd.Close()
							input := &unselectedInput{}
							var out, diagnostic bytes.Buffer
							app := App{In: input, Out: &out, Err: &diagnostic, Getenv: func(key string) string {
								if key == "HOME" {
									return home
								}
								return ""
							}}
							args := append([]string{"--store", dir, mode}, tc.args...)
							args = append(args, tc.flag)
							if tc.flag != "--stdin" {
								args = append(args, strconv.Itoa(int(fd.Fd())))
							}
							exit := app.Main(args)
							offset, err := fd.Seek(0, 1)
							if err != nil || offset != 0 || input.reads != 0 {
								t.Fatalf("input consumed or closed: offset=%d reads=%d error=%v", offset, input.reads, err)
							}
							if exit != 1 {
								t.Fatalf("exit=%d", exit)
							}
							if mode == "--json" {
								var envelope struct{ Error struct{ Code string } }
								if err := json.Unmarshal(out.Bytes(), &envelope); err != nil || envelope.Error.Code != "metadata.unsupported" {
									t.Fatal("wrong refusal", err, out.String())
								}
							} else if out.Len() != 0 || !strings.Contains(diagnostic.String(), "metadata.unsupported") {
								t.Fatal("wrong human refusal", diagnostic.String())
							}
							if !reflect.DeepEqual(before, machineFiles(t, home)) {
								t.Fatal("refusal changed files or modes")
							}
						})
					}
				}
			}
		})
	}
}
