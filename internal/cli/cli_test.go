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

func TestBackupPruneCLIPreviewsBeforeExplicitApplication(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "store")
	invoke := func(args ...string) (int, map[string]any) {
		var out bytes.Buffer
		a := App{In: bytes.NewReader([]byte("fixture")), Out: &out, Getenv: func(k string) string {
			if k == "HOME" {
				return home
			}
			return ""
		}}
		code := a.Main(append([]string{"--store", dir, "--json"}, args...))
		var result map[string]any
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return code, result
	}
	if code, _ := invoke("init", "--yes"); code != 0 {
		t.Fatal(code)
	}
	for _, name := range []string{"a", "b", "c"} {
		if code, _ := invoke("add", name, "--stdin"); code != 0 {
			t.Fatal(code)
		}
	}
	if code, _ := invoke("backup", "prune", "--yes"); code != 2 {
		t.Fatal("accepted implicit retention", code)
	}
	code, preview := invoke("backup", "prune", "--keep", "1")
	if code != 0 || preview["data"].(map[string]any)["dry_run"] != true {
		t.Fatal("missing preview", code, preview)
	}
	code, backups := invoke("backup", "list")
	if code != 0 || len(backups["data"].(map[string]any)["backups"].([]any)) != 3 {
		t.Fatal("preview removed backups", code)
	}
	code, result := invoke("backup", "prune", "--keep", "1", "--yes")
	if code != 0 || len(result["data"].(map[string]any)["deleted"].([]any)) != 2 {
		t.Fatal("prune not applied", code, result)
	}
	code, backups = invoke("backup", "list")
	if code != 0 || len(backups["data"].(map[string]any)["backups"].([]any)) != 1 {
		t.Fatal("wrong retained set", code)
	}
}

func TestDoctorUnhealthyIsNonzeroWithReport(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "store")
	invoke := func(args ...string) (int, map[string]any) {
		var out bytes.Buffer
		a := App{In: bytes.NewReader([]byte("fixture")), Out: &out, Getenv: func(k string) string {
			if k == "HOME" {
				return home
			}
			return ""
		}}
		code := a.Main(append([]string{"--store", dir, "--json"}, args...))
		var result map[string]any
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return code, result
	}
	if code, _ := invoke("init", "--yes", "--no-git"); code != 0 {
		t.Fatal(code)
	}
	if code, _ := invoke("add", "entry", "--stdin"); code != 0 {
		t.Fatal(code)
	}
	code, result := invoke("status")
	if code != 0 {
		t.Fatal(code, result)
	}
	summary := result["data"].(map[string]any)["backups"].(map[string]any)
	if summary["count"] != float64(1) || summary["bytes"].(float64) <= 0 || summary["oldest"] == "" {
		t.Fatal("missing backup statistics", summary)
	}
	if code, _ := invoke("doctor"); code != 0 {
		t.Fatal("healthy doctor", code)
	}
	if err := os.WriteFile(filepath.Join(dir, "passwords", "entry.age"), []byte("invalid fixture ciphertext"), 0600); err != nil {
		t.Fatal(err)
	}
	code, result = invoke("doctor")
	if code != 1 {
		t.Fatal("unhealthy doctor succeeded", code, result)
	}
	problem := result["error"].(map[string]any)
	report := problem["details"].(map[string]any)["report"].(map[string]any)
	if problem["code"] != "doctor.unhealthy" || report["healthy"] != false || len(report["issues"].([]any)) == 0 {
		t.Fatal("missing typed report", problem)
	}
}
