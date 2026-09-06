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

func TestImpossibleRecoveryPreservesSelectedPassphrase(t *testing.T) {
	for _, noGit := range []bool{false, true} {
		t.Run(fmt.Sprint("no-git=", noGit), func(t *testing.T) {
			home, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(home, "store")
			if _, err := store.Init(dir, noGit, false); err != nil {
				t.Fatal(err)
			}
			passphrase := filepath.Join(home, "passphrase")
			if err := os.WriteFile(passphrase, []byte("synthetic recovery passphrase sentinel"), 0600); err != nil {
				t.Fatal(err)
			}
			missing := filepath.Join(home, "missing.age")
			before := machineFiles(t, home)
			for _, tc := range []struct {
				name, target, code string
				args               []string
			}{
				{"import-missing", dir, "artifact.unsafe", []string{"transfer", "import", missing}},
				{"verify-missing", filepath.Join(home, "absent-store"), "artifact.unsafe", []string{"transfer", "verify", missing}},
				{"restore-missing", filepath.Join(home, "restored"), "artifact.unsafe", []string{"backup", "restore", "--full", "--yes", missing}},
				{"restore-occupied", dir, "recovery.target_exists", []string{"backup", "restore", "--full", "--yes", passphrase}},
				{"restore-file-target", passphrase, "recovery.target_exists", []string{"backup", "restore", "--full", "--yes", passphrase}},
				{"restore-parent-missing", filepath.Join(home, "absent-parent", "restored"), "recovery.invalid_target", []string{"backup", "restore", "--full", "--yes", passphrase}},
			} {
				for _, mode := range []string{"--json", "--non-interactive"} {
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
						args := append([]string{"--store", tc.target, mode}, tc.args...)
						args = append(args, "--passphrase-fd", strconv.Itoa(int(input.Fd())))
						code := app.Main(args)
						position, seekErr := input.Seek(0, 1)
						if seekErr != nil || position != 0 {
							t.Fatalf("consumed or closed recovery input: offset=%d error=%v", position, seekErr)
						}
						if code != 1 {
							t.Fatal("wrong refusal status", code, output.String())
						}
						if mode == "--json" {
							var envelope struct{ Error struct{ Code string } }
							if err := json.Unmarshal(output.Bytes(), &envelope); err != nil || envelope.Error.Code != tc.code {
								t.Fatal("wrong error", err, output.String())
							}
						} else if output.Len() != 0 {
							t.Fatal("unexpected refused recovery output")
						}
						if bytes.Contains(append(output.Bytes(), diagnostic.Bytes()...), []byte("synthetic recovery passphrase")) {
							t.Fatal("secret leaked")
						}
						if !reflect.DeepEqual(before, machineFiles(t, home)) {
							t.Fatal("refusal changed fixture tree")
						}
					})
				}
			}
		})
	}
}
