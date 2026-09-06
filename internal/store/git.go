package store

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/agensfield/fulla/internal/fault"
)

func (s *Store) Git(args ...string) ([]byte, error) {
	// A textual commondir can redirect refs/objects even when all filesystem
	// paths are nonsymlinks. Fulla only supports a self-contained repository.
	for _, name := range []string{"passwords/.git/commondir", "passwords/.git/objects/info/alternates"} {
		if _, err := s.Root.Lstat(name); err == nil {
			return nil, fault.New("git.external_storage", "external Git storage is unsupported")
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, fault.New("git.unreadable", "cannot inspect Git storage routing")
		}
	}
	a := []string{"-C", s.PasswordDirectory(), "--git-dir", filepath.Join(s.PasswordDirectory(), ".git"), "--work-tree", s.PasswordDirectory(), "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-c", "core.fsmonitor=false", "-c", "maintenance.auto=false", "-c", "gc.auto=0", "-c", "core.autocrlf=false"}
	// Automatic maintenance can detach, then mutate Git objects after this
	// command returns and outside Fulla's lock/staging lifetime. Internal Git
	// operations must not spawn it; explicit expert Git remains user-controlled.
	a = append(a, args...)
	// Git stderr can contain hooks, filters, filenames, or configured commands.
	// Caller diagnostics never forward arbitrary subprocess output.
	command := exec.Command("git", a...)
	command.Env = internalGitEnvironment(os.Environ())
	out, err := command.Output()
	if err != nil {
		return nil, fault.New("git.failed", "Git operation failed; inspect the encrypted repository with fulla git")
	}
	return out, nil
}

func internalGitEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "GIT_") {
			// Honor standard config-file selection, but not ambient repository,
			// object/index, runtime config, tracing, executable or format routing.
			switch name {
			case "GIT_CONFIG_NOSYSTEM", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM":
			default:
				continue
			}
		}
		result = append(result, entry)
	}
	return append(result, "GIT_OPTIONAL_LOCKS=0")
}

func (s *Store) GitEnabled() (bool, error) {
	info, err := s.Root.Lstat("passwords/.git")
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fault.New("git.unsupported", "external Git directories are unsupported")
	}
	return true, nil
}

func (s *Store) CleanGit() (bool, error) {
	enabled, err := s.GitEnabled()
	if err != nil || !enabled {
		return enabled, err
	}
	names, err := s.Names()
	if err != nil {
		return true, err
	}
	if err := s.checkGitConversions(names); err != nil {
		return true, err
	}
	out, err := s.Git("status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return true, err
	}
	if len(out) != 0 {
		return true, fault.New("git.dirty", "encrypted password repository has uncommitted changes")
	}
	return true, nil
}

func (s *Store) Commit(names []string, message string) error {
	enabled, err := s.GitEnabled()
	if err != nil || !enabled {
		return err
	}
	if len(names) == 0 {
		return nil
	}
	// Recovery can resume after Git configuration changed while interrupted.
	if err := s.checkGitConversions(names); err != nil {
		return err
	}
	paths := []string{}
	for _, name := range names {
		if _, err := EntryPath(name); err != nil {
			return err
		}
		paths = append(paths, name+".age")
	}
	if _, err := s.Git(append([]string{"--literal-pathspecs", "add", "-A", "--"}, paths...)...); err != nil {
		return err
	}
	if _, err := s.Git("-c", "user.name=Fulla", "-c", "user.email=fulla@localhost", "commit", "--allow-empty", "-m", message); err != nil {
		return err
	}
	return nil
}

func (s *Store) Head() (string, error) {
	out, err := s.Git("rev-parse", "--verify", "HEAD")
	return strings.TrimSpace(string(out)), err
}
