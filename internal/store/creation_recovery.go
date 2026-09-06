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

func (s *Store) bindCreation(lock *Lock, kind, id, target string) error {
	info, err := s.InspectLock()
	if err != nil {
		return err
	}
	if info == nil || info.Token != lock.Token || info.Operation != kind || (kind != "init" && kind != "restore") || !validID(id) || info.InitID != "" || info.RestoreID != "" || info.StageID != "" || info.AdoptionID != "" || info.PeerReceipt != "" {
		return fault.New("store.lock_changed", "cannot bind store creation to this lock")
	}
	data, err := securefs.Read(s.Root, "lock/info", 4096)
	if err != nil {
		return err
	}
	bound := strings.TrimSpace(string(data)) + " " + kind + "_id=" + id + " " + kind + "_target=" + base64.RawURLEncoding.EncodeToString([]byte(target)) + "\n"
	// Leave room within the 4096-byte reader limit for a longer recovery PID
	// and operation name when a new process claims ownership.
	if len(bound) > 3900 {
		return fault.Usage("store creation target exceeds lock binding limit")
	}
	return securefs.Replace(s.Root, "lock/info", []byte(bound))
}

// RecoverCreation can open a bound init/restore stage before pa-v1 files exist.
// Ownership and the whole private tree are validated before recovery takeover.
func RecoverCreation(directory, expected string) (map[string]any, error) {
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
	if err := s.validateCreation(); err != nil {
		return nil, err
	}
	return s.recover(expected, s.validateCreation)
}

func (s *Store) validateCreation() error {
	if err := securefs.ValidateTree(s.Root); err != nil {
		return err
	}
	info, err := s.InspectLock()
	if err != nil {
		return err
	}
	_, err = s.creationPublished(info)
	return err
}

type creationBinding struct {
	kind, id, target, storeID string
}

func bindingForCreation(info *LockInfo) (creationBinding, error) {
	if info == nil {
		return creationBinding{}, fault.New("store.lock_unknown", "missing creation ownership")
	}
	b := creationBinding{kind: "init", id: info.InitID, target: info.InitTarget, storeID: info.InitID}
	if info.RestoreID != "" {
		b = creationBinding{kind: "restore", id: info.RestoreID, target: info.RestoreTarget, storeID: info.RestoreStoreID}
	}
	if !validID(b.id) || !filepath.IsAbs(b.target) || filepath.Clean(b.target) != b.target || strings.ContainsRune(b.target, 0) {
		return b, fault.New("store.lock_unknown", "invalid creation ownership")
	}
	return b, nil
}

func (s *Store) creationPublished(info *LockInfo) (bool, error) {
	b, err := bindingForCreation(info)
	if err != nil {
		return false, err
	}
	directory, err := securefs.Canonical(s.Dir, false)
	if err != nil {
		return false, err
	}
	stage := filepath.Join(filepath.Dir(b.target), ".fulla-"+b.kind+"-"+b.id)
	if directory == stage {
		return false, nil
	}
	if directory != b.target {
		return false, fault.New("transaction.conflict", "store creation binding does not identify this directory")
	}
	data, err := securefs.Read(s.Root, metadata+"/store.json", maxMetadata)
	if err != nil {
		return false, err
	}
	var meta Metadata
	if err := decodeMetadata(data, &meta); err != nil {
		return false, err
	}
	if !validID(b.storeID) || meta.StoreID != b.storeID {
		return false, fault.New("transaction.conflict", "published store creation identity differs from binding")
	}
	for _, domain := range []string{"peers", "sync", "backup", "identity", "transactions"} {
		if meta.Domains[domain] != 1 {
			return false, fault.New("metadata.unsupported", "store creation recovery requires understood metadata")
		}
	}
	if err := s.Validate(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) recoverCreation(info *LockInfo, token string) (result map[string]any, err error) {
	b, err := bindingForCreation(info)
	if err != nil {
		return nil, err
	}
	published, err := s.creationPublished(info)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err == nil {
			return
		}
		failure := fault.New("store.cleanup_failed", "store creation recovery incomplete; inspect retained ownership")
		failure.Details["applied"] = published
		failure.Details["cleanup_required"] = true
		failure.Details["target"] = b.target
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
			return nil, fault.New("store.lock_changed", "store creation staging path changed")
		}
		if err := removeCreationContents(s.Root); err != nil {
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
	return map[string]any{"recovered": true, b.kind + "_applied": published, b.kind + "_id": b.id, "lock_released": true}, nil
}

// Retain ownership until every private child is gone. Removing the whole tree
// first can unlink lock/info before encountering an undeletable identity file.
func removeCreationContents(root *os.Root) error {
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
