package store

import (
	"errors"
	"path/filepath"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

// finishUnpublished releases only this operation's owned staging and lock.
// A nonempty dir is supplied only after successful exclusive directory creation.
// Published journals must never take this path: their evidence belongs to recovery.
func (s *Store) finishUnpublished(lock *Lock, dir string, original error) error {
	var cleanupErr error
	if dir != "" {
		cleanupErr = s.Root.RemoveAll(dir)
		if cleanupErr == nil {
			cleanupErr = securefs.SyncDir(s.Root, filepath.Dir(dir))
		}
	}
	releaseErr := lock.Release()
	if cleanupErr == nil && releaseErr == nil {
		return original
	}
	err := fault.New("transaction.cleanup_failed", "could not confirm cleanup of an unpublished operation")
	err.Details["applied"] = false
	err.Details["cleanup_required"] = true
	if cleanupErr != nil {
		err.Details["staging_path"] = filepath.Join(s.Dir, dir)
		err.Details["staging_cleanup_required"] = true
	}
	if releaseErr != nil {
		err.Details["lock_cleanup_required"] = true
	}
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
