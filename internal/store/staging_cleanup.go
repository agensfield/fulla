package store

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func (s *Store) validateStagingBinding(id string) error {
	if id == "" {
		return nil
	}
	if err := s.RequireDomain("transactions"); err != nil {
		return err
	}
	for _, name := range []string{"pending.json", "rotation.json", "prune.json"} {
		data, err := securefs.Read(s.Root, metadata+"/"+name, maxMetadata)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		boundID := ""
		switch name {
		case "pending.json":
			var j Journal
			if err := StrictJSON(data, &j); err != nil {
				return err
			}
			boundID = j.ID
		case "rotation.json":
			var j Rotation
			if err := StrictJSON(data, &j); err != nil {
				return err
			}
			boundID = j.ID
		}
		if boundID != id {
			return fault.New("transaction.conflict", "staging binding does not match pending recovery")
		}
	}
	return nil
}

// bindStaging durably associates the exclusively created, still-empty stage
// with its lock before any private recovery material is written there.
func (s *Store) bindStaging(lock *Lock, id string) error {
	info, err := s.InspectLock()
	if err != nil {
		return err
	}
	if info == nil || info.Token != lock.Token || !validID(id) || info.StageID != "" || info.PeerReceipt != "" || info.AdoptionID != "" {
		return fault.New("store.lock_changed", "cannot bind staging to this lock")
	}
	data, err := securefs.Read(s.Root, "lock/info", 4096)
	if err != nil {
		return err
	}
	published, err := securefs.ReplacePublished(s.Root, "lock/info", []byte(strings.TrimSpace(string(data))+" stage_id="+id+"\n"))
	if published {
		lock.StageID = id
	}
	return err
}

// finishUnpublished releases only this operation's owned staging and lock.
// A nonempty dir is supplied only after successful exclusive directory creation.
// Published journals must never take this path: their evidence belongs to recovery.
func (s *Store) finishUnpublished(lock *Lock, dir string, original error) error {
	var cleanupErr error
	if dir != "" {
		owner, err := s.InspectLock()
		if err != nil || owner == nil || owner.Token != lock.Token || owner.StageID != lock.StageID || owner.PeerReceipt != "" {
			failure := unpublishedCleanupFailure(original)
			failure.Details["staging_path"] = filepath.Join(s.Dir, dir)
			failure.Details["staging_cleanup_required"] = true
			failure.Details["lock_cleanup_required"] = true
			failure.Details["ownership_changed"] = true
			return failure
		}
		cleanupErr = s.Root.RemoveAll(dir)
		if cleanupErr == nil {
			cleanupErr = securefs.SyncDir(s.Root, filepath.Dir(dir))
		}
	}
	var releaseErr error
	if cleanupErr == nil || lock.StageID == "" {
		releaseErr = lock.Release()
	}
	if cleanupErr == nil && releaseErr == nil {
		return original
	}
	err := unpublishedCleanupFailure(original)
	if cleanupErr != nil {
		err.Details["staging_path"] = filepath.Join(s.Dir, dir)
		err.Details["staging_cleanup_required"] = true
		if lock.StageID != "" {
			err.Details["lock_retained"] = true
			err.Details["recovery_required"] = true
		}
	}
	if releaseErr != nil {
		err.Details["lock_cleanup_required"] = true
	}
	return err
}

func unpublishedCleanupFailure(original error) *fault.Error {
	err := fault.New("transaction.cleanup_failed", "could not confirm cleanup of an unpublished operation")
	err.Details["applied"] = false
	err.Details["cleanup_required"] = true
	var previous *fault.Error
	if errors.As(original, &previous) {
		err.Details["operation_code"] = previous.Code
		switch previous.Status {
		case 129, 130, 131, 143:
			err.Status = previous.Status
		}
	}
	return err
}
