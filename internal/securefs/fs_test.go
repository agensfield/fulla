package securefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateFilesAndNoReplace(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := WriteNew(r, "entry", []byte("original")); err != nil {
		t.Fatal(err)
	}
	if err := WriteNew(r, "entry", []byte("changed")); err == nil {
		t.Fatal("replaced existing destination")
	}
	if err := r.Link("entry", "alias"); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(r, "entry", 100); err == nil {
		t.Fatal("accepted unexpected hard link")
	}
	if err := r.Remove("alias"); err != nil {
		t.Fatal(err)
	}
	if err := r.Chmod("entry", 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(r, "entry", 100); err == nil {
		t.Fatal("accepted unsafe mode")
	}
	if err := r.Chmod("entry", 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := Read(r, "entry", 100)
	if err != nil || string(b) != "original" {
		t.Fatal("lost original", err)
	}
	if _, err := Read(r, "entry", 1); err == nil {
		t.Fatal("ignored bound")
	}
	if err := os.Symlink(dir, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := Canonical(filepath.Join(dir, "link", "entry"), false); err == nil {
		t.Fatal("accepted symlink")
	}
	if err := ValidateTree(r); err == nil {
		t.Fatal("accepted linked tree")
	}
}
