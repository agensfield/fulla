package store

import (
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

const lockReleasePrefix = ".fulla-lock-release-"

type releasedLock struct {
	Token string
	PID   int
	Host  string // SHA-256 of the host name; no raw hostname in a directory name.
}

func releaseBinding(name string) (*releasedLock, error) {
	parts := strings.Split(strings.TrimPrefix(name, lockReleasePrefix), "-")
	if !strings.HasPrefix(name, lockReleasePrefix) || len(parts) != 4 || parts[0] != "v1" || !validID(parts[3]) {
		return nil, fault.New("store.lock_unknown", "unsupported detached lock binding")
	}
	fingerprint, err := hex.DecodeString(parts[1])
	if err != nil || len(fingerprint) != 32 {
		return nil, fault.New("store.lock_unknown", "invalid detached host fingerprint")
	}
	pid, err := strconv.Atoi(parts[2])
	if err != nil || pid <= 0 || strconv.Itoa(pid) != parts[2] {
		return nil, fault.New("store.lock_unknown", "invalid detached owner PID")
	}
	return &releasedLock{Host: parts[1], PID: pid, Token: parts[3]}, nil
}

// Detach the completed lock before removing any owner evidence. The atomic
// directory name binds cleanup to this process and token throughout deletion.
func (l *Lock) release(checkpoint func(string) error) (err error) {
	defer func() {
		if err != nil && l.releaseDir != "" {
			err = l.store.lockCleanupFailure(l.releaseDir, l.Token)
		}
	}()
	if !l.held && l.releaseDir == "" {
		return nil
	}
	if !validID(l.Token) {
		return fault.New("store.lock_unknown", "invalid lock owner token")
	}
	step := func(phase string) error {
		if checkpoint != nil {
			return checkpoint(phase)
		}
		return nil
	}
	if l.releaseDir == "" {
		owner, err := securefs.Read(l.store.Root, "lock/owner", 256)
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(owner)) != l.Token {
			return fault.New("store.lock_changed", "lock ownership changed")
		}
		root, err := l.store.Root.OpenRoot("lock")
		if err != nil {
			return err
		}
		err = validateReleaseFiles(root)
		root.Close()
		if err != nil {
			return err
		}
		host, err := os.Hostname()
		if err != nil {
			return err
		}
		name := lockReleasePrefix + "v1-" + digest([]byte(host)) + "-" + strconv.Itoa(os.Getpid()) + "-" + l.Token
		if err := step("ready"); err != nil {
			return err
		}
		if err := securefs.RenameNew(l.store.Root, "lock", name); err != nil {
			return err
		}
		l.held = false
		l.releaseDir = name
		if err := step("detached"); err != nil {
			return err
		}
	}
	if err := securefs.SyncDir(l.store.Root, "."); err != nil {
		return err
	}
	if err := step("synced"); err != nil {
		return err
	}
	if err := l.store.cleanReleasedLock(l.releaseDir, l.Token, step); err != nil {
		return err
	}
	l.releaseDir = ""
	return nil
}

func validateReleaseFiles(root *os.Root) error {
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(4)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(entries) > 3 {
		return fault.New("store.lock_unknown", "unexpected lock cleanup contents")
	}
	for _, entry := range entries {
		if entry.Name() != "owner" && entry.Name() != "info" && entry.Name() != "recovery" {
			return fault.New("store.lock_unknown", "unexpected file in lock cleanup")
		}
		info, err := root.Lstat(entry.Name())
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || (entry.Name() == "recovery" && info.Size() != 0) || (entry.Name() == "owner" && info.Size() > 256) || (entry.Name() == "info" && info.Size() > 4096) {
			return fault.New("store.lock_unknown", "invalid lock cleanup file")
		}
		if err := securefs.ValidateInfo(entry.Name(), info, true); err != nil {
			return err
		}
	}
	return securefs.ValidateTree(root)
}

func (s *Store) readReleasedLock(name string) (*releasedLock, error) {
	record, err := releaseBinding(name)
	if err != nil {
		return nil, err
	}
	info, err := s.Root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fault.New("store.lock_unknown", "detached lock is not a directory")
	}
	if err := securefs.ValidateInfo(name, info, true); err != nil {
		return nil, err
	}
	root, err := s.Root.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := validateReleaseFiles(root); err != nil {
		return nil, err
	}
	if owner, err := securefs.Read(root, "owner", 256); err == nil {
		if strings.TrimSpace(string(owner)) != record.Token {
			return nil, fault.New("store.lock_changed", "detached lock token differs")
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return record, nil
}

func (s *Store) cleanReleasedLock(name, token string, checkpoint func(string) error) error {
	record, err := s.readReleasedLock(name)
	if errors.Is(err, fs.ErrNotExist) {
		return securefs.SyncDir(s.Root, ".")
	}
	if err != nil {
		return err
	}
	if record.Token != token {
		return fault.New("store.lock_changed", "detached lock token differs")
	}
	root, err := s.Root.OpenRoot(name)
	if err != nil {
		return err
	}
	defer root.Close()
	opened, err := root.Stat(".")
	if err != nil {
		return err
	}
	named, err := s.Root.Lstat(name)
	if err != nil {
		return err
	}
	if !os.SameFile(opened, named) {
		return fault.New("store.lock_changed", "detached cleanup path changed")
	}
	if err := checkpoint("cleanup-validated"); err != nil {
		return err
	}
	for _, file := range []string{"info", "owner"} {
		if err := root.Remove(file); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := checkpoint(file + "-removed"); err != nil {
			return err
		}
	}
	if err := root.Remove("recovery"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := securefs.SyncDir(root, "."); err != nil {
		return err
	}
	if err := checkpoint("empty"); err != nil {
		return err
	}
	if err := s.Root.Remove(name); err != nil {
		return err
	}
	return securefs.SyncDir(s.Root, ".")
}

func (s *Store) lockCleanupFailure(name, token string) *fault.Error {
	err := fault.New("store.lock_cleanup_failed", "lock released but detached cleanup is incomplete")
	err.Details["lock_released"] = true
	err.Details["cleanup_required"] = true
	err.Details["cleanup_path"] = filepath.Join(s.Dir, name)
	err.Details["token"] = token
	return err
}
