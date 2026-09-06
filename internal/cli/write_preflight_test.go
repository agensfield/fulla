package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agensfield/fulla/internal/store"
)

func TestImpossibleWriteDoesNotConsumeSelectedInput(t *testing.T) {
	for _, git := range []bool{false, true} {
		s := func() *store.Store {
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
			t.Cleanup(func() { s.Close() })
			return s
		}()
		if _, err := s.Write("exists", []byte("fixture"), false); err != nil {
			t.Fatal(err)
		}
		before := machineFiles(t, s.Dir)
		for _, tc := range []struct{ command, name, code string }{
			{"add", "exists", "entry.exists"}, {"edit", "missing", "entry.not_found"},
		} {
			for _, mode := range []string{"--json", "--non-interactive"} {
				input := &unselectedInput{}
				var out, diagnostic bytes.Buffer
				a := App{In: input, Out: &out, Err: &diagnostic, Getenv: func(key string) string {
					if key == "HOME" {
						return filepath.Dir(s.Dir)
					}
					return ""
				}}
				status := a.Main([]string{"--store", s.Dir, mode, tc.command, tc.name, "--stdin"})
				if status != 1 || input.reads != 0 {
					t.Fatalf("%s %s: status=%d input reads=%d", tc.command, mode, status, input.reads)
				}
				if mode == "--json" {
					var envelope struct{ Error struct{ Code string } }
					if err := json.Unmarshal(out.Bytes(), &envelope); err != nil || envelope.Error.Code != tc.code {
						t.Fatal("wrong refusal", err, out.String())
					}
				}
				if !reflect.DeepEqual(before, machineFiles(t, s.Dir)) {
					t.Fatal("refusal changed store")
				}
			}
		}
	}
}
