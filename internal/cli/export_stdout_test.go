package cli

import (
	"bytes"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"filippo.io/age"
	"github.com/agensfield/fulla/internal/store"
)

type shortExportWriter struct{ calls int }

func (w *shortExportWriter) Write(data []byte) (int, error) { w.calls++; return len(data) / 2, nil }

func TestPartialExportStdoutReturnsAppliedFailure(t *testing.T) {
	for _, noGit := range []bool{false, true} {
		t.Run(fmt.Sprintf("no-git=%t", noGit), func(t *testing.T) {
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
			if _, err := s.Write("entry", []byte("synthetic secret sentinel"), false); err != nil {
				t.Fatal(err)
			}
			s.Close()
			before := machineFiles(t, home)
			identity, err := age.GenerateX25519Identity()
			if err != nil {
				t.Fatal(err)
			}
			var diagnostics bytes.Buffer
			output := &shortExportWriter{}
			app := App{In: bytes.NewReader(nil), Out: output, Err: &diagnostics, Getenv: func(k string) string {
				if k == "HOME" {
					return home
				}
				return ""
			}}
			code := app.Main([]string{"--store", dir, "--non-interactive", "transfer", "export", "--output", "-", "--recipient", identity.Recipient().String()})
			if code != 3 || output.calls != 1 || !bytes.Contains(diagnostics.Bytes(), []byte("transaction.incomplete")) {
				t.Fatal("lost stream failure status", code, output.calls, diagnostics.String())
			}
			if bytes.Contains(diagnostics.Bytes(), []byte("sentinel")) {
				t.Fatal("secret diagnostic")
			}
			if !reflect.DeepEqual(before, machineFiles(t, home)) {
				t.Fatal("partial stream wrote receipt or changed store")
			}
		})
	}
}
