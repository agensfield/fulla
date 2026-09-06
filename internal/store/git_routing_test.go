package store

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestInternalGitIgnoresAmbientRepositoryRouting(t *testing.T) {
	for _, route := range []string{"repository", "index", "objects", "common", "runtime-config", "trace"} {
		t.Run(route, func(t *testing.T) {
			unrelated := fixture(t, true)
			if _, err := unrelated.Write("sentinel", []byte("unrelated fixture"), false); err != nil {
				t.Fatal(err)
			}
			before := archiveTree(t, unrelated, false)
			gitdir := filepath.Join(unrelated.PasswordDirectory(), ".git")
			switch route {
			case "repository":
				t.Setenv("GIT_DIR", gitdir)
				t.Setenv("GIT_WORK_TREE", unrelated.PasswordDirectory())
			case "index":
				t.Setenv("GIT_INDEX_FILE", filepath.Join(gitdir, "index"))
			case "objects":
				t.Setenv("GIT_OBJECT_DIRECTORY", filepath.Join(gitdir, "objects"))
			case "common":
				t.Setenv("GIT_COMMON_DIR", gitdir)
			case "runtime-config":
				t.Setenv("GIT_CONFIG_COUNT", "1")
				t.Setenv("GIT_CONFIG_KEY_0", "core.worktree")
				t.Setenv("GIT_CONFIG_VALUE_0", unrelated.PasswordDirectory())
			case "trace":
				t.Setenv("GIT_TRACE", filepath.Join(unrelated.Dir, "redirected-trace"))
			}
			// Initialization itself must stay in the selected private directory.
			s := fixture(t, true)
			value := []byte{0, 255, 10}
			if _, err := s.Write("selected", value, false); err != nil {
				t.Fatal(err)
			}
			got, err := s.Read("selected")
			if err != nil || !bytes.Equal(got, value) {
				t.Fatal("lost value", err)
			}
			if _, err := s.CleanGit(); err != nil {
				t.Fatal(err)
			}
			top, err := s.Git("rev-parse", "--show-toplevel")
			if err != nil || strings.TrimSpace(string(top)) != s.PasswordDirectory() {
				t.Fatal("Git escaped selected store", err)
			}
			if !reflect.DeepEqual(before, archiveTree(t, unrelated, false)) {
				t.Fatal("internal Git mutated unrelated repository")
			}
		})
	}
}

func TestInternalGitOverridesConfiguredWorktree(t *testing.T) {
	s, unrelated := fixture(t, true), fixture(t, true)
	if _, err := s.Git("config", "core.worktree", unrelated.PasswordDirectory()); err != nil {
		t.Fatal(err)
	}
	before := archiveTree(t, unrelated, false)
	if _, err := s.Write("entry", []byte("fixture"), false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, archiveTree(t, unrelated, false)) {
		t.Fatal("configured worktree escaped store")
	}
}

func TestInternalGitRejectsExternalStorageFiles(t *testing.T) {
	for _, name := range []string{"commondir", "objects/info/alternates"} {
		t.Run(name, func(t *testing.T) {
			s, unrelated := fixture(t, true), fixture(t, true)
			file := filepath.Join(s.PasswordDirectory(), ".git", name)
			if err := os.WriteFile(file, []byte(filepath.Join(unrelated.PasswordDirectory(), ".git")+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			before, other := archiveTree(t, s, false), archiveTree(t, unrelated, false)
			if _, err := s.Write("entry", []byte("fixture"), false); err == nil {
				t.Fatal("accepted external Git storage")
			}
			if !reflect.DeepEqual(before, archiveTree(t, s, false)) || !reflect.DeepEqual(other, archiveTree(t, unrelated, false)) {
				t.Fatal("external storage refusal mutated files")
			}
		})
	}
}
