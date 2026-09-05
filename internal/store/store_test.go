package store

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestMain(m *testing.M) { syscall.Umask(0o077); os.Exit(m.Run()) }

func fixture(t *testing.T, git bool) *Store {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(dir, "store")
	if _, err := Init(name, !git, false); err != nil {
		t.Fatal(err)
	}
	s, err := Open(name, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestInitExplicitAndTransactional(t *testing.T) {
	for _, git := range []bool{false, true} {
		s := fixture(t, git)
		if err := s.DeepVerify(); err != nil {
			t.Fatal(err)
		}
		if _, err := Init(s.Dir, !git, false); err == nil {
			t.Fatal("replaced store")
		}
		if _, err := s.CleanGit(); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(t.TempDir(), "missing")
	if _, err := Open(dir, true, nil); err == nil {
		t.Fatal("opened absent store")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("ordinary open created store")
	}
}

func TestAdoptionPreservesLiveMaterial(t *testing.T) {
	s := fixture(t, true)
	if err := s.Root.RemoveAll(metadata); err != nil {
		t.Fatal(err)
	}
	before, err := securefs.Read(s.Root, "identities", maxMetadata)
	if err != nil {
		t.Fatal(err)
	}
	_, rs, err := s.Keys()
	if err != nil {
		t.Fatal(err)
	}
	c, err := crypt.Encrypt([]byte{0, 255, 10}, rs)
	if err != nil {
		t.Fatal(err)
	}
	if err := securefs.WriteNew(s.Root, "passwords/sample.age", c); err != nil {
		t.Fatal(err)
	}
	if err := s.Commit([]string{"sample"}, "fixture"); err != nil {
		t.Fatal(err)
	}
	if err := privateModes(s.Root, "passwords/.git"); err != nil {
		t.Fatal(err)
	}
	if _, err := Adopt(s.Dir, true, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Root.Stat(metadata); !os.IsNotExist(err) {
		t.Fatal("dry-run mutated metadata")
	}
	if _, err := Adopt(s.Dir, false, nil); err != nil {
		t.Fatal(err)
	}
	after, err := securefs.Read(s.Root, "identities", maxMetadata)
	if err != nil || string(after) != string(before) {
		t.Fatal("changed identity", err)
	}
	after, err = s.Ciphertext("sample")
	if err != nil || string(after) != string(c) {
		t.Fatal("changed ciphertext", err)
	}
	if _, err := Adopt(s.Dir, false, nil); err == nil {
		t.Fatal("replaced adoption")
	}
}

func TestSharedLockAndMetadataFailure(t *testing.T) {
	s := fixture(t, false)
	l, err := s.Lock("test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Lock("other"); err == nil {
		t.Fatal("concurrent lock acquired")
	}
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{`{"version":1,"version":2}`, `{"version":1,"unknown":true}`, `{} {}`} {
		var m Metadata
		if err := StrictJSON([]byte(input), &m); err == nil {
			t.Fatal("accepted malformed state")
		}
	}
}
