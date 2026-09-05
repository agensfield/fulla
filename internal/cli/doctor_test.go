package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateDiagnosticReport(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "store")
	invoke := func(input string, args ...string) int {
		var out, stderr bytes.Buffer
		app := App{In: strings.NewReader(input), Out: &out, Err: &stderr, Getenv: func(key string) string {
			if key == "HOME" {
				return home
			}
			return ""
		}}
		return app.Main(append([]string{"--store", dir, "--json"}, args...))
	}
	if code := invoke("", "init", "--yes", "--no-git"); code != 0 {
		t.Fatal(code)
	}
	const secret = "diagnostic-secret-must-not-appear"
	const name = "private-entry-name"
	if code := invoke(secret, "add", name, "--stdin"); code != 0 {
		t.Fatal(code)
	}
	healthyOutput := filepath.Join(home, "healthy.json")
	if code := invoke("", "doctor", "--deep", "--report", healthyOutput); code != 0 {
		t.Fatal("healthy deep report", code)
	}
	healthyBytes, err := os.ReadFile(healthyOutput)
	if err != nil {
		t.Fatal(err)
	}
	var healthy diagnosticReport
	if err := json.Unmarshal(healthyBytes, &healthy); err != nil || !healthy.Healthy || !healthy.Complete || !healthy.Deep {
		t.Fatal("invalid healthy report", err)
	}
	identity, err := os.ReadFile(filepath.Join(dir, "identities"))
	if err != nil {
		t.Fatal(err)
	}
	// Corrupt only the fixture ciphertext. An unhealthy structural inspection
	// must still emit a useful report and preserve its nonzero exit status.
	if err := os.WriteFile(filepath.Join(dir, "passwords", name+".age"), []byte("bad header"), 0600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(home, "report.json")
	if code := invoke("", "doctor", "--report", output); code != 1 {
		t.Fatal(code)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, excluded := range [][]byte{[]byte(secret), []byte(name), identity, []byte(home)} {
		if bytes.Contains(data, excluded) {
			t.Fatal("report contains excluded data")
		}
	}
	var report diagnosticReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Healthy || !report.Complete || report.Schema != "fulla.diagnostic/v1" || len(report.Issues) != 1 || report.Issues[0] != "ciphertext.invalid_header" {
		t.Fatalf("unexpected report: %+v", report)
	}
	info, err := os.Stat(output)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("report must be private", err)
	}
	if code := invoke("", "doctor", "--report", output); code != 1 {
		t.Fatal("existing destination accepted", code)
	}
	after, err := os.ReadFile(output)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatal("existing report changed")
	}
	for _, dest := range []string{filepath.Join(dir, "report.json"), filepath.Join(home, "missing", "report.json"), "-"} {
		if code := invoke("", "doctor", "--report", dest); code == 0 {
			t.Fatal("unsafe report destination accepted")
		}
	}
	link := filepath.Join(home, "linked.json")
	if err := os.Symlink(output, link); err != nil {
		t.Fatal(err)
	}
	if code := invoke("", "doctor", "--report", link); code != 1 {
		t.Fatal("symlink destination accepted", code)
	}
	if code := invoke("", "doctor", "--recover-lock", "token", "--report", filepath.Join(home, "recovery.json")); code != 2 {
		t.Fatal("mixed recovery and inspection accepted", code)
	}
}

func TestDiagnosticReportBeforeStoreOpen(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "store")
	invoke := func(args ...string) (int, []byte) {
		var out, stderr bytes.Buffer
		app := App{Out: &out, Err: &stderr, Getenv: func(key string) string {
			if key == "HOME" {
				return home
			}
			return ""
		}}
		code := app.Main(append([]string{"--store", dir, "--json"}, args...))
		return code, out.Bytes()
	}
	check := func(filename, expected string) {
		t.Helper()
		output := filepath.Join(home, filename)
		code, envelope := invoke("doctor", "--deep", "--report", output)
		if code != 1 {
			t.Fatalf("status %d: %s", code, envelope)
		}
		data, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		var report diagnosticReport
		if err := json.Unmarshal(data, &report); err != nil {
			t.Fatal(err)
		}
		if report.Complete || report.Healthy || report.Error != expected {
			t.Fatalf("unexpected early report %+v", report)
		}
		if bytes.Contains(data, []byte(home)) {
			t.Fatal("path leaked")
		}
	}
	check("missing.json", "store.uninitialized")
	if code, out := invoke("init", "--yes", "--no-git"); code != 0 {
		t.Fatalf("init %d: %s", code, out)
	}
	identities := filepath.Join(dir, "identities")
	before, err := os.ReadFile(identities)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(identities, 0644); err != nil {
		t.Fatal(err)
	}
	check("unsafe-file.json", "store.unsafe")
	after, err := os.ReadFile(identities)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("identity changed")
	}
	info, err := os.Stat(identities)
	if err != nil || info.Mode().Perm() != 0644 {
		t.Fatal("inspection changed permissions")
	}
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	check("unsafe-root.json", "store.unsafe")
	// Restore only test fixture modes for cleanup.
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(identities, 0600); err != nil {
		t.Fatal(err)
	}
}
