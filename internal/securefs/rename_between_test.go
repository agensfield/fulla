package securefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenameNewBetweenConfinedParents(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"source", "destination"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	from, err := os.OpenRoot(filepath.Join(dir, "source"))
	if err != nil {
		t.Fatal(err)
	}
	defer from.Close()
	to, err := os.OpenRoot(filepath.Join(dir, "destination"))
	if err != nil {
		t.Fatal(err)
	}
	defer to.Close()
	if err := WriteNew(from, "staged", []byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := WriteNew(to, "occupied", []byte("old")); err != nil {
		t.Fatal(err)
	}
	for _, names := range [][2]string{{"staged", "occupied"}, {"../source/staged", "new"}, {"staged", "../escape"}, {".", "new"}, {"staged", ".."}} {
		if err := RenameNewBetween(from, names[0], to, names[1]); err == nil {
			t.Fatal("accepted replacement or non-child path", names)
		}
		data, err := os.ReadFile(filepath.Join(dir, "source/staged"))
		if err != nil || string(data) != "new" {
			t.Fatal("refusal changed source", err)
		}
		data, err = os.ReadFile(filepath.Join(dir, "destination/occupied"))
		if err != nil || string(data) != "old" {
			t.Fatal("refusal replaced target", err)
		}
	}
	if err := RenameNewBetween(from, "staged", to, "published"); err != nil {
		t.Fatal(err)
	}
	if _, err := from.Lstat("staged"); !os.IsNotExist(err) {
		t.Fatal("rename retained source", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "destination/published"))
	if err != nil || string(data) != "new" {
		t.Fatal("publication lost bytes", err)
	}
}
