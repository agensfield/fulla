package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestMain(m *testing.M) { syscall.Umask(0o077); os.Exit(m.Run()) }

func TestAgentExactByteJourney(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "store")
	invoke := func(input []byte, args ...string) (int, []byte) {
		var out, diagnostics bytes.Buffer
		a := App{In: bytes.NewReader(input), Out: &out, Err: &diagnostics, Getenv: func(k string) string {
			if k == "HOME" {
				return home
			}
			return ""
		}}
		code := a.Main(append([]string{"--store", dir}, args...))
		if bytes.Contains(diagnostics.Bytes(), input) && len(input) > 0 {
			t.Fatal("secret in diagnostic output")
		}
		return code, out.Bytes()
	}
	if code, _ := invoke(nil, "list", "--json"); code != 1 {
		t.Fatal("uninitialized status", code)
	}
	if code, _ := invoke(nil, "init", "--json"); code != 1 {
		t.Fatal("unacknowledged init", code)
	}
	if code, _ := invoke(nil, "init", "--yes", "--json"); code != 0 {
		t.Fatal("init", code)
	}
	value := []byte{'s', 0, 255, 10, 10}
	if code, out := invoke(value, "create", "exact", "--stdin", "--json"); code != 0 {
		t.Fatalf("add %d %s", code, out)
	}
	if code, out := invoke(nil, "cat", "exact"); code != 0 || !bytes.Equal(out, value) {
		t.Fatal("raw bytes", code, out)
	}
	code, out := invoke(nil, "get", "exact", "--json")
	if code != 0 {
		t.Fatal(code)
	}
	var e struct {
		Schema   string
		OK       bool
		Command  string
		Warnings []any
		Data     struct {
			Encoding string
			Value    string
		}
	}
	if err := json.Unmarshal(out, &e); err != nil {
		t.Fatal(err)
	}
	if e.Schema != "fulla.cli/v1" || e.Command != "show" || !e.OK || e.Warnings == nil || e.Data.Encoding != "base64" {
		t.Fatal("wrong envelope", string(out))
	}
	decoded, err := base64.StdEncoding.DecodeString(e.Data.Value)
	if err != nil || !bytes.Equal(decoded, value) {
		t.Fatal("json lost bytes")
	}
	if code, _ := invoke(value, "add", "exact", "--stdin", "--json"); code != 1 {
		t.Fatal("duplicate add", code)
	}
	if code, _ := invoke(nil, "show", "exact", "--force", "--json"); code != 2 {
		t.Fatal("unknown flag", code)
	}
}

func TestParserPreservesPassthroughAndAliases(t *testing.T) {
	p, err := parse([]string{"exec", "--env", "TOKEN=api", "--", "printf", "--json", "a b", "$(not-shell)"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Command != "run" || p.has("json") || len(p.Tail) != 4 || p.Tail[3] != "$(not-shell)" {
		t.Fatal(p)
	}
	for _, args := range [][]string{{"add", "a", "--stdin", "--stdin"}, {"show", "a", "--store"}, {"show", "a", "--json=false"}, {"a"}} {
		p, err := parse(args)
		if args[0] == "a" {
			if err == nil && p.Command == "add" {
				t.Fatal("legacy alias accepted")
			}
			continue
		}
		if err == nil {
			t.Fatal("invalid invocation accepted", args)
		}
	}
}
