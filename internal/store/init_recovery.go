package store

import (
	"encoding/base64"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func (s *Store) bindInitialization(lock *Lock, id, target string) error {
	info, err := s.InspectLock()
	if err != nil {
		return err
	}
	if info == nil || info.Token != lock.Token || info.Operation != "init" || !validID(id) || info.InitID != "" || info.StageID != "" || info.AdoptionID != "" || info.PeerReceipt != "" {
		return fault.New("store.lock_changed", "cannot bind initialization to this lock")
	}
	data, err := securefs.Read(s.Root, "lock/info", 4096)
	if err != nil {
		return err
	}
	bound := strings.TrimSpace(string(data)) + " init_id=" + id + " init_target=" + base64.RawURLEncoding.EncodeToString([]byte(target)) + "\n"
	// Leave room within the 4096-byte reader limit for a longer recovery PID
	// and operation name when a new process claims ownership.
	if len(bound) > 4000 {
		return fault.Usage("initialization target exceeds lock binding limit")
	}
	return securefs.Replace(s.Root, "lock/info", []byte(bound))
}

// RecoverInitialization can open a bound partial stage before pa-v1 files exist.
// Ownership and the whole private tree are validated before recovery takeover.
func RecoverInitialization(directory, expected string) (map[string]any, error) {
	directory, err := securefs.Canonical(directory, false)
	if err != nil {
		return nil, err
	}
	root, err := securefs.Open(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	s := &Store{Dir: directory, Root: root}
	if err := s.validateInitialization(); err != nil {
		return nil, err
	}
	return s.recover(expected, s.validateInitialization)
}

func (s *Store) validateInitialization() error {
	if err := securefs.ValidateTree(s.Root); err != nil {
		return err
	}
	info, err := s.InspectLock()
	if err != nil {
		return err
	}
	_, err = s.initializationPublished(info)
	return err
}

func (s *Store) initializationPublished(info *LockInfo) (bool, error) {
	if info == nil || !validID(info.InitID) || !filepath.IsAbs(info.InitTarget) || filepath.Clean(info.InitTarget) != info.InitTarget || strings.ContainsRune(info.InitTarget, 0) {
		return false, fault.New("store.lock_unknown", "invalid initialization ownership")
	}
	directory, err := securefs.Canonical(s.Dir, false)
	if err != nil {
		return false, err
	}
	stage := filepath.Join(filepath.Dir(info.InitTarget), ".fulla-init-"+info.InitID)
	if directory == stage {
		return false, nil
	}
	if directory != info.InitTarget {
		return false, fault.New("transaction.conflict", "initialization binding does not identify this directory")
	}
	data, err := securefs.Read(s.Root, metadata+"/store.json", maxMetadata)
	if err != nil {
		return false, err
	}
	var meta Metadata
	if err := decodeMetadata(data, &meta); err != nil {
		return false, err
	}
	if meta.StoreID != info.InitID {
		return false, fault.New("transaction.conflict", "published initialization identity differs from binding")
	}
	for _, domain := range []string{"peers", "sync", "backup", "identity", "transactions"} {
		if meta.Domains[domain] != 1 {
			return false, fault.New("metadata.unsupported", "initialization recovery requires understood metadata")
		}
	}
	if err := s.Validate(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) recoverInitialization(info *LockInfo, token string) (result map[string]any, err error) {
	published, err := s.initializationPublished(info)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err == nil {
			return
		}
		failure := fault.New("store.cleanup_failed", "initialization recovery incomplete; inspect retained ownership")
		failure.Details["applied"] = published
		failure.Details["cleanup_required"] = true
		failure.Details["target"] = info.InitTarget
		if published {
			failure.Status = 3
		} else {
			failure.Details["staging_path"] = s.Dir
		}
		err = failure
	}()
	parent, err := os.OpenRoot(filepath.Dir(s.Dir))
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	if !published {
		// Compare the opened stage to its parent entry. Never remove a replacement
		// reached through a stale pathname, even when its basename looks plausible.
		opened, err := s.Root.Stat(".")
		if err != nil {
			return nil, err
		}
		named, err := parent.Lstat(filepath.Base(s.Dir))
		if err != nil {
			return nil, err
		}
		if !os.SameFile(opened, named) {
			return nil, fault.New("store.lock_changed", "initialization staging path changed")
		}
		if err := removeInitializationContents(s.Root); err != nil {
			return nil, err
		}
		if err := parent.RemoveAll(filepath.Base(s.Dir)); err != nil {
			return nil, err
		}
	}
	if err := securefs.SyncDir(parent, "."); err != nil {
		return nil, err
	}
	if published {
		if err := s.Root.Remove("lock/recovery"); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		lock := &Lock{store: s, Token: token, held: true}
		if err := lock.Release(); err != nil {
			return nil, err
		}
	}
	return map[string]any{"recovered": true, "init_applied": published, "init_id": info.InitID, "lock_released": true}, nil
}

// Retain ownership until every private child is gone. Removing the whole tree
// first can unlink lock/info before encountering an undeletable identity file.
func removeInitializationContents(root *os.Root) error {
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() != "lock" {
			if err := root.RemoveAll(entry.Name()); err != nil {
				return err
			}
		}
	}
	return securefs.SyncDir(root, ".")
}
