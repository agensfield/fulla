package store

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func (s *Store) bindAdoption(lock *Lock, id string) error {
	info, err := s.InspectLock()
	if err != nil {
		return err
	}
	if info == nil || info.InitID != "" || info.Token != lock.Token || info.Operation != "adopt" || !validID(id) || info.AdoptionID != "" || info.StageID != "" || info.PeerReceipt != "" {
		return fault.New("store.lock_changed", "cannot bind adoption to this lock")
	}
	data, err := securefs.Read(s.Root, "lock/info", 4096)
	if err != nil {
		return err
	}
	return securefs.Replace(s.Root, "lock/info", []byte(strings.TrimSpace(string(data))+" adoption_id="+id+"\n"))
}

// The published store ID and the bound staging ID are identical. Never infer
// publication from the mere existence of a metadata directory or stage name.
func (s *Store) adoptionPublished(id string) (bool, error) {
	if !validID(id) {
		return false, fault.New("store.lock_unknown", "invalid adoption binding")
	}
	if _, err := s.Root.Lstat(metadata); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	data, err := securefs.Read(s.Root, metadata+"/store.json", maxMetadata)
	if err != nil {
		return false, err
	}
	var meta Metadata
	if err := decodeMetadata(data, &meta); err != nil {
		return false, err
	}
	if meta.StoreID != id {
		return false, fault.New("transaction.conflict", "published metadata does not match adoption binding")
	}
	for _, domain := range []string{"peers", "sync", "backup", "identity", "transactions"} {
		if meta.Domains[domain] != 1 {
			return false, fault.New("metadata.unsupported", "adoption recovery requires understood metadata domains")
		}
	}
	return true, nil
}

func (s *Store) recoverAdoption(id, token string) (result map[string]any, err error) {
	published, err := s.adoptionPublished(id)
	if err != nil {
		return nil, err
	}
	stage := ".fulla-adopt-" + id
	defer func() {
		if err == nil {
			return
		}
		failure := fault.New("store.cleanup_failed", "adoption recovery incomplete; inspect cleanup evidence")
		failure.Details["applied"] = published
		failure.Details["cleanup_required"] = true
		failure.Details["lock_cleanup_required"] = true
		if published {
			failure.Status = 3
		} else {
			failure.Details["staging_path"] = filepath.Join(s.Dir, stage)
		}
		err = failure
	}()
	if !published {
		if info, err := s.Root.Lstat(stage); err == nil {
			if !info.IsDir() {
				return nil, fault.New("transaction.invalid_staging", "bound adoption staging is not a directory")
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if err := s.Root.RemoveAll(stage); err != nil {
			return nil, fault.New("store.cleanup_failed", "adoption staging cleanup failed; recovery lock retained")
		}
	}
	// Confirm either the original metadata publication or unpublished cleanup.
	if err := securefs.SyncDir(s.Root, "."); err != nil {
		return nil, fault.New("store.cleanup_failed", "adoption finalization sync failed; recovery lock retained")
	}
	if err := s.Root.Remove("lock/recovery"); err != nil {
		return nil, err
	}
	lock := &Lock{store: s, Token: token, held: true}
	if err := lock.Release(); err != nil {
		return nil, err
	}
	return map[string]any{"recovered": true, "lock_released": true, "adoption_applied": published, "adoption_id": id}, nil
}
