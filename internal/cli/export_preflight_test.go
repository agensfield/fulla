package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/agensfield/fulla/internal/store"
)

func TestImpossibleExportDoesNotConsumePassphrase(t *testing.T) {
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
			occupied := filepath.Join(home, "occupied")
			manifest := filepath.Join(home, "manifest")
			passphrase := filepath.Join(home, "passphrase")
			for name, content := range map[string][]byte{occupied: []byte("retained"), manifest: []byte("{malformed"), passphrase: []byte("synthetic recovery passphrase for refusal fixture")} {
				if err := os.WriteFile(name, content, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			before := machineFiles(t, home)
			for _, tc := range []struct {
				name   string
				args   []string
				status int
				code   string
			}{
				{"logical-occupied", []string{"transfer", "export", "--output", occupied}, 1, "export.exists"},
				{"full-occupied", []string{"backup", "export", "--full", "--output", occupied}, 1, "export.exists"},
				{"malformed-manifest", []string{"transfer", "export", "--output", filepath.Join(home, "new"), "--manifest", manifest}, 2, "invocation.invalid"},
				{"stdout-manifest", []string{"transfer", "export", "--output", "-", "--manifest", manifest}, 2, "invocation.invalid"},
			} {
				for _, mode := range []string{"--json", "--non-interactive"} {
					if tc.name == "stdout-manifest" && mode == "--json" {
						continue
					}
					t.Run(tc.name+"/"+mode, func(t *testing.T) {
						input, err := os.Open(passphrase)
						if err != nil {
							t.Fatal(err)
						}
						defer input.Close()
						var output, diagnostic bytes.Buffer
						app := App{In: &unselectedInput{}, Out: &output, Err: &diagnostic, Getenv: func(key string) string {
							if key == "HOME" {
								return home
							}
							return ""
						}}
						args := append([]string{"--store", dir, mode}, tc.args...)
						args = append(args, "--passphrase-fd", strconv.Itoa(int(input.Fd())))
						status := app.Main(args)
						// An attempted passphrase read also closes its selected descriptor.
						// Inspect it immediately, before opening any additional files.
						position, seekErr := input.Seek(0, 1)
						if seekErr != nil || position != 0 {
							t.Fatalf("refusal consumed or closed passphrase input: offset=%d error=%v", position, seekErr)
						}
						if status != tc.status {
							t.Fatalf("status=%d, want %d", status, tc.status)
						}
						if mode == "--json" {
							var envelope struct{ Error struct{ Code string } }
							if err := json.Unmarshal(output.Bytes(), &envelope); err != nil || envelope.Error.Code != tc.code {
								t.Fatal("incorrect refusal", err, output.String())
							}
						} else if output.Len() != 0 {
							t.Fatal("refused export emitted output")
						}
						if bytes.Contains(append(output.Bytes(), diagnostic.Bytes()...), []byte("synthetic recovery passphrase")) {
							t.Fatal("passphrase leaked")
						}
						if !reflect.DeepEqual(before, machineFiles(t, home)) {
							t.Fatal("refusal changed fixture files or modes")
						}
					})
				}
			}
		})
	}
}
