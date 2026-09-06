package securefs

import (
	"errors"
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

func TestValidateTreeAfterRootPublication(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(parent, "stage")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := Open(stage)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := WriteNew(root, "entry", []byte("fixture")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(stage, filepath.Join(parent, "published")); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTree(root); err != nil {
		t.Fatal("descriptor-owned tree lost after publication", err)
	}
	if err := root.Chmod("entry", 0644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTree(root); err == nil {
		t.Fatal("accepted unsafe published file")
	}
}

func TestReplacementDistinguishesPublicationFromDurability(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := WriteNew(root, "entry", []byte("old")); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("fixture directory sync failure")
	published, err := replacePublished(root, "entry", []byte("new"), func(*os.Root, string) error { return injected })
	if !published || !errors.Is(err, injected) {
		t.Fatal("lost published state", published, err)
	}
	data, err := Read(root, "entry", 100)
	if err != nil || string(data) != "new" {
		t.Fatal("replacement not visible", err)
	}
	published, err = ReplacePublished(root, "missing", []byte("new"))
	if published || err == nil {
		t.Fatal("reported publication before rename", published, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("left staging files", err)
	}
}

func TestNewPublicationReportsDirectorySyncFailure(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	injected := errors.New("fixture sync failure")
	published, err := publishNewPublished(root, "entry", []byte("new"), func(*os.Root, string) error { return injected })
	if !published || !errors.Is(err, injected) {
		t.Fatal("lost publication", published, err)
	}
	published, err = PublishNewPublished(root, "entry", []byte("overwrite"))
	if published || err == nil {
		t.Fatal("replaced existing file", published, err)
	}
	data, err := Read(root, "entry", 100)
	if err != nil || string(data) != "new" {
		t.Fatal("wrong bytes", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("staging remains", err)
	}
}
