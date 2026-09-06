package store

import (
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"syscall"

	"github.com/agensfield/fulla/internal/fault"
	"golang.org/x/sys/unix"
)

type LockCleanup struct {
	Path  string `json:"path"`
	Token string `json:"token"`
	PID   int    `json:"pid,omitempty"`
	Host  string `json:"host_fingerprint,omitempty"`
}

func (s *Store) lockCleanupNames(token string) ([]string, error) {
	dir, err := s.Root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	names := []string{}
	count := 0
	for {
		entries, err := dir.ReadDir(256)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		count += len(entries)
		if count > 65536 {
			return nil, fault.New("store.lock_cleanup_limit", "lock cleanup inspection exceeds limit")
		}
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(name, lockReleasePrefix) && (token == "" || strings.HasSuffix(name, "-"+token)) {
				names = append(names, name)
				if len(names) > 1024 {
					return nil, fault.New("store.lock_cleanup_limit", "too many detached lock cleanups")
				}
			}
		}
		if errors.Is(err, io.EOF) {
			sort.Strings(names)
			return names, nil
		}
	}
}

func (s *Store) InspectLockCleanup() ([]LockCleanup, error) {
	names, err := s.lockCleanupNames("")
	if err != nil {
		return nil, err
	}
	result := []LockCleanup{}
	for _, name := range names {
		record, err := s.readReleasedLock(name)
		if err != nil {
			return nil, err
		}
		result = append(result, LockCleanup{Path: name, Token: record.Token, PID: record.PID, Host: record.Host})
	}
	return result, nil
}

// Returns handled=false only when this token has no detached cleanup directory.
// A separate active store lock is never changed by detached cleanup recovery.
func (s *Store) RecoverLockCleanup(token string) (result map[string]any, handled bool, err error) {
	if !validID(token) {
		return nil, false, nil
	}
	names, scanErr := s.lockCleanupNames(token)
	if scanErr != nil {
		return nil, true, scanErr
	}
	if len(names) == 0 {
		return nil, false, nil
	}
	if len(names) != 1 {
		return nil, true, fault.New("store.lock_unknown", "ambiguous detached cleanup token")
	}
	name := names[0]
	validate := func() error {
		record, err := s.readReleasedLock(name)
		if err != nil {
			return err
		}
		if record != nil {
			host, err := os.Hostname()
			if err != nil {
				return err
			}
			if record.Host != digest([]byte(host)) || !errors.Is(syscall.Kill(record.PID, 0), syscall.ESRCH) {
				return fault.New("store.lock_active", "refusing live, remote, or unverifiable release owner")
			}
		}
		return nil
	}
	if err := validate(); err != nil {
		return nil, true, err
	}
	root, err := s.Root.OpenRoot(name)
	if err != nil {
		return nil, true, err
	}
	defer root.Close()
	guard, err := root.OpenFile("recovery", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, true, err
	}
	defer guard.Close()
	if err := unix.Flock(int(guard.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return nil, true, fault.New("store.locked", "another cleanup recovery owns this directory")
	}
	if err := validate(); err != nil {
		return nil, true, err
	}
	opened, err := root.Stat(".")
	if err != nil {
		return nil, true, err
	}
	named, err := s.Root.Lstat(name)
	if err != nil {
		return nil, true, err
	}
	if !os.SameFile(opened, named) {
		return nil, true, fault.New("store.lock_changed", "detached cleanup path changed")
	}
	if err := s.cleanReleasedLock(name, token, func(string) error { return nil }); err != nil {
		return nil, true, s.lockCleanupFailure(name, token)
	}
	return map[string]any{"recovered": true, "lock_released": true, "cleanup_removed": name}, true, nil
}

// Full archives must not clone transient lock-cleanup ownership.
func (s *Store) CheckLockCleanup() error {
	entries, err := s.InspectLockCleanup()
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fault.New("store.lock_cleanup_pending", "recover detached lock cleanup before full export")
	}
	return nil
}
