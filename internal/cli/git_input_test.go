package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/agensfield/fulla/internal/securefs"
	"github.com/agensfield/fulla/internal/store"
)

func TestGitWriteRefusalPreservesInput(t *testing.T) {
	for _, reason := range []string{"conversion", "dirty"} {
		t.Run(reason, func(t *testing.T) {
			home, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(home, "store")
			if _, err := store.Init(dir, false, false); err != nil {
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
			code := "git.conversion_unsupported"
			if reason == "conversion" {
				// Only the prospective new name is affected: existing-inventory validation
				// cannot detect this rule. check-attr must inspect the destination itself.
				if err := securefs.WriteNew(s.Root, "passwords/.git/info/attributes", []byte("new.age text\n")); err != nil {
					t.Fatal(err)
				}
			} else {
				code = "git.dirty"
				c, err := s.Ciphertext("present")
				if err != nil {
					t.Fatal(err)
				}
				if err := securefs.WriteNew(s.Root, "passwords/untracked.age", c); err != nil {
					t.Fatal(err)
				}
			}
			inputPath := filepath.Join(home, "input")
			if err := os.WriteFile(inputPath, []byte("synthetic secret input"), 0600); err != nil {
				t.Fatal(err)
			}
			before := machineFiles(t, home)
			for _, mode := range []string{"--json", "--non-interactive"} {
				for _, channel := range []string{"--stdin", "--from-fd"} {
					t.Run(mode+"/"+channel, func(t *testing.T) {
						fd, err := os.Open(inputPath)
						if err != nil {
							t.Fatal(err)
						}
						defer fd.Close()
						input := &unselectedInput{}
						var out, diagnostic bytes.Buffer
						a := App{In: input, Out: &out, Err: &diagnostic, Getenv: func(key string) string {
							if key == "HOME" {
								return home
							}
							return ""
						}}
						args := []string{"--store", dir, mode, "add", "new", channel}
						if channel == "--from-fd" {
							args = append(args, strconv.Itoa(int(fd.Fd())))
						}
						exit := a.Main(args)
						offset, err := fd.Seek(0, 1)
						if err != nil || offset != 0 || input.reads != 0 {
							t.Fatalf("refusal consumed or closed input: offset=%d reads=%d error=%v", offset, input.reads, err)
						}
						if exit != 1 {
							t.Fatalf("exit=%d", exit)
						}
						if mode == "--json" {
							var envelope struct{ Error struct{ Code string } }
							if err := json.Unmarshal(out.Bytes(), &envelope); err != nil || envelope.Error.Code != code {
								t.Fatal("wrong refusal", err, out.String())
							}
						} else if out.Len() != 0 || !strings.Contains(diagnostic.String(), code) {
							t.Fatal("wrong refusal", diagnostic.String())
						}
						if !reflect.DeepEqual(before, machineFiles(t, home)) {
							t.Fatal("refusal changed fixture")
						}
					})
				}
			}
		})
	}
}
