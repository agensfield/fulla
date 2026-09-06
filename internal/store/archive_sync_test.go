package store

import (
	"errors"
	"os"
	"path"
	"reflect"
	"testing"

	"github.com/agensfield/fulla/internal/securefs"
)

func TestRestoreSyncsAllDirectoriesBeforeTheirParents(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, name := range []string{"a/b/empty", "c/empty"} {
		if err := root.MkdirAll(name, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := securefs.WriteNew(root, "a/b/value", []byte("fixture")); err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	err = syncStagedDirectories(root, func(root *os.Root, name string) error {
		if _, ok := seen[name]; ok {
			t.Fatalf("duplicate sync %s", name)
		}
		seen[name] = len(seen)
		return securefs.SyncDir(root, name)
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]bool{".": true, "a": true, "a/b": true, "a/b/empty": true, "c": true, "c/empty": true}
	actual := map[string]bool{}
	for name, index := range seen {
		actual[name] = true
		if name != "." && seen[path.Dir(name)] <= index {
			t.Fatalf("parent synced before child %s", name)
		}
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("missing directory syncs: %v", seen)
	}
	injected := errors.New("fixture sync failure")
	calls := 0
	err = syncStagedDirectories(root, func(*os.Root, string) error { calls++; return injected })
	if !errors.Is(err, injected) || calls != 1 {
		t.Fatalf("sync error lost: %v (%d calls)", err, calls)
	}
}
