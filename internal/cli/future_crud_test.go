package cli

import (
	"bytes"
	"encoding/base64"
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

func TestFutureMetadataMachineCRUD(t *testing.T) {
	for _, noGit := range []bool{false, true} {
		for _, mode := range []string{"--json", "--non-interactive"} {
			t.Run(fmt.Sprintf("no-git=%v/%s", noGit, mode), func(t *testing.T) {
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
				next := s.Meta
				for name := range next.Domains {
					next.Domains[name] = 99
				}
				manifest, err := json.Marshal(next)
				if err != nil {
					t.Fatal(err)
				}
				if err := securefs.Replace(s.Root, ".fulla/store.json", manifest); err != nil {
					t.Fatal(err)
				}
				if err := securefs.WriteNew(s.Root, ".fulla/transactions/opaque", []byte("future transaction data")); err != nil {
					t.Fatal(err)
				}
				s.Close()
				foreign := func() map[string][32]byte {
					files := machineFiles(t, dir)
					for name := range files {
						rel, err := filepath.Rel(dir, name)
						if err != nil {
							t.Fatal(err)
						}
						if rel == "passwords" || strings.HasPrefix(rel, "passwords/") || rel == ".fulla/basic-v1" || strings.HasPrefix(rel, ".fulla/basic-v1/") {
							delete(files, name)
						}
					}
					return files
				}
				before := foreign()
				invoke := func(input []byte, args ...string) []byte {
					var out, diagnostic bytes.Buffer
					app := App{In: bytes.NewReader(input), Out: &out, Err: &diagnostic, Getenv: func(key string) string {
						if key == "HOME" {
							return home
						}
						return ""
					}}
					if code := app.Main(append([]string{"--store", dir, mode}, args...)); code != 0 {
						t.Fatal("machine CRUD failed", code, out.String(), diagnostic.String())
					}
					return out.Bytes()
				}
				initial := []byte{0, 255, 10}
				invoke(initial, "add", "a", "--stdin")
				output := invoke(nil, "show", "a")
				if mode == "--json" {
					var envelope struct {
						Data struct {
							Value    string
							Encoding string
						}
					}
					if err := json.Unmarshal(output, &envelope); err != nil || envelope.Data.Encoding != "base64" || envelope.Data.Value != base64.StdEncoding.EncodeToString(initial) {
						t.Fatal("base64 value mismatch", err)
					}
				} else if !bytes.Equal(output, initial) {
					t.Fatal("raw value mismatch")
				}
				inputPath := filepath.Join(home, "input")
				if err := os.WriteFile(inputPath, []byte{}, 0600); err != nil {
					t.Fatal(err)
				}
				fd, err := os.Open(inputPath)
				if err != nil {
					t.Fatal(err)
				}
				invoke(nil, "edit", "a", "--from-fd", strconv.Itoa(int(fd.Fd())))
				_ = fd.Close()
				invoke(nil, "move", "a", "b")
				invoke(nil, "remove", "b", "--permanent-delete")
				if !reflect.DeepEqual(before, foreign()) {
					t.Fatal("machine CRUD changed opaque feature bytes or modes")
				}
			})
		}
	}
}
