package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agensfield/fulla/internal/store"
)

func TestGitPassthroughStreamsStatusAndLock(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "store")
	if _, err := store.Init(dir, false, false); err != nil {
		t.Fatal(err)
	}
	invoke := func(args ...string) (int, string, string) {
		var out, diagnostics bytes.Buffer
		a := App{In: strings.NewReader("fixture input"), Out: &out, Err: &diagnostics, Getenv: func(k string) string {
			if k == "HOME" {
				return home
			}
			return ""
		}}
		code := a.Main(append([]string{"--store", dir, "git"}, args...))
		return code, out.String(), diagnostics.String()
	}
	if code, out, err := invoke("--", "rev-parse", "--show-toplevel"); code != 0 || strings.TrimSpace(out) != filepath.Join(dir, "passwords") || err != "" {
		t.Fatalf("wrong repository/streams: %d %q %q", code, out, err)
	}
	// The explicit expert escape hatch may run a user-supplied Git alias. Use
	// one to observe the lock while the child is alive and verify stream/status
	// forwarding without relying on Git's version-specific error prose.
	alias := "alias.fixture=!test -d ../lock || exit 71; cat; printf diagnostic >&2; exit 23"
	if code, out, err := invoke("--", "-c", alias, "fixture"); code != 23 || out != "fixture input" || err != "diagnostic" {
		t.Fatalf("child contract: %d %q %q", code, out, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "lock")); !os.IsNotExist(err) {
		t.Fatalf("lock retained after child failure: %v", err)
	}
	if code, out, _ := invoke("--json", "--", "-c", alias, "fixture"); code != 2 || !strings.Contains(out, "fulla.cli/v1") || strings.Contains(out, "fixture input") {
		t.Fatalf("JSON did not reject passthrough: %d %s", code, out)
	}
	s, err := store.Open(dir, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	lock, err := s.Lock("test")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	if code, out, _ := invoke("--", "rev-parse", "HEAD"); code != 1 || out != "" {
		t.Fatalf("ran Git while locked: %d %s", code, out)
	}
}
