package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"

	"github.com/agensfield/fulla/internal/securefs"
)

func TestInitializationSyncsFilesThenDirectories(t *testing.T) {
	for _, git := range []bool{false, true} {
		t.Run(fmt.Sprintf("git=%v", git), func(t *testing.T) {
			s := fixture(t, git)
			if err := s.Root.MkdirAll("passwords/nested/empty", 0700); err != nil {
				t.Fatal(err)
			}
			before := archiveTree(t, s, false)
			expectedFiles, expectedDirs := map[string]bool{}, map[string]bool{}
			if err := fs.WalkDir(s.Root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					expectedDirs[name] = true
				} else {
					expectedFiles[name] = true
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			files, dirs := map[string]bool{}, map[string]int{}
			err := syncInitialization(s.Root, func(root *os.Root, name string) error {
				if len(dirs) != 0 || files[name] {
					t.Fatal("file sync duplicated or followed directory sync")
				}
				files[name] = true
				return syncInitializationFile(root, name)
			}, func(root *os.Root, name string) error {
				if !reflect.DeepEqual(expectedFiles, files) {
					t.Fatal("directory synced before all files")
				}
				if _, exists := dirs[name]; exists {
					t.Fatal("directory synced twice")
				}
				dirs[name] = len(dirs)
				return securefs.SyncDir(root, name)
			})
			if err != nil {
				t.Fatal(err)
			}
			actualDirs := map[string]bool{}
			for name, index := range dirs {
				actualDirs[name] = true
				if name != "." && dirs[path.Dir(name)] <= index {
					t.Fatal("directory parent synced too early", name)
				}
			}
			if !reflect.DeepEqual(expectedFiles, files) || !reflect.DeepEqual(expectedDirs, actualDirs) {
				t.Fatal("incomplete synchronization inventory")
			}
			gitFiles := 0
			for name := range files {
				if strings.HasPrefix(name, "passwords/.git/") {
					gitFiles++
				}
			}
			if git && gitFiles == 0 {
				t.Fatal("fixture did not include Git-created files")
			}
			if !reflect.DeepEqual(before, archiveTree(t, s, false)) {
				t.Fatal("sync changed contents or modes")
			}
		})
	}
}

func TestInitializationSyncStopsOnFailure(t *testing.T) {
	s := fixture(t, false)
	injected := errors.New("fixture sync failure")
	for _, kind := range []string{"file", "directory"} {
		t.Run(kind, func(t *testing.T) {
			fileCalls, dirCalls := 0, 0
			err := syncInitialization(s.Root, func(*os.Root, string) error {
				fileCalls++
				if kind == "file" {
					return injected
				}
				return nil
			}, func(*os.Root, string) error { dirCalls++; return injected })
			if !errors.Is(err, injected) {
				t.Fatal("lost sync failure", err)
			}
			if kind == "file" && (fileCalls != 1 || dirCalls != 0) {
				t.Fatal("continued after file failure")
			}
			if kind == "directory" && (fileCalls == 0 || dirCalls != 1) {
				t.Fatal("continued after directory failure")
			}
		})
	}
}
