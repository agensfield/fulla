package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"time"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

type PermissionResult struct {
	Changes []securefs.ModeChange `json:"changes"`
	Applied int                   `json:"applied"`
	Receipt string                `json:"receipt,omitempty"`
}

func FixPermissions(directory string) (result PermissionResult, err error) {
	root, err := securefs.OpenForModeRepair(directory)
	if err != nil {
		return result, fault.New("permissions.unsafe", err.Error())
	}
	defer root.Close()
	s := &Store{Dir: directory, Root: root}
	// Refuse unrelated directories before even creating a lock. Identity and
	// ciphertext contents are never read by permission repair.
	for _, name := range []string{"identities", "recipients", "passwords", metadata + "/store.json"} {
		if _, err := root.Lstat(name); err != nil {
			return result, fault.New("store.invalid", "permission repair requires an existing Fulla store")
		}
	}
	if _, err := securefs.PlanModes(root); err != nil {
		return result, fault.New("permissions.unsafe", err.Error())
	}
	lock, err := s.lock("fix-permissions", func() error {
		var planErr error
		result.Changes, planErr = securefs.PlanModes(root)
		if planErr != nil {
			return fault.New("permissions.unsafe", planErr.Error())
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	defer func() {
		if releaseErr := lock.Release(); releaseErr != nil && err == nil {
			err = fault.Applied("permissions repaired but lock release failed", lock.Token)
		}
		var problem *fault.Error
		if errors.As(err, &problem) && problem.Status == 3 {
			problem.Details["repair"] = result
		}
	}()
	result.Applied, err = securefs.ApplyModes(root, result.Changes)
	if err != nil {
		if result.Applied > 0 {
			return result, fault.Applied("permission repair partially applied; inspect and retry explicitly", lock.Token)
		}
		return result, fault.New("permissions.failed", err.Error())
	}
	if result.Applied == 0 {
		return result, nil
	}
	result.Receipt = metadata + "/receipts/" + lock.Token + ".json"
	data, _ := json.Marshal(map[string]any{"version": 1, "command": "doctor fix-permissions", "at": time.Now().UTC().Format(time.RFC3339Nano), "changes": result.Changes, "applied": result.Applied})
	if err := securefs.PublishNew(root, result.Receipt, data); err != nil {
		return result, fault.Applied("permissions repaired but receipt publication failed", lock.Token)
	}
	return result, nil
}

// RecoverAndFixPermissions is explicitly selected with the inspected lock token.
// Only interrupted permission repair may bypass ordinary private-mode validation.
func RecoverAndFixPermissions(directory, expected string) (PermissionResult, error) {
	root, err := securefs.OpenForModeRepair(directory)
	if err != nil {
		return PermissionResult{}, fault.New("permissions.unsafe", err.Error())
	}
	defer root.Close()
	s := &Store{Dir: directory, Root: root}
	validate := func() error {
		info, err := s.InspectLock()
		if err != nil {
			return err
		}
		if info == nil || info.Operation != "fix-permissions" {
			return fault.New("permissions.wrong_lock", "selected lock does not belong to permission repair")
		}
		for _, name := range []string{"pending.json", "rotation.json", "prune.json"} {
			if _, err := root.Lstat(metadata + "/" + name); !errors.Is(err, fs.ErrNotExist) {
				return fault.New("transaction.pending", "permission recovery refuses other pending operations")
			}
		}
		_, err = securefs.PlanModes(root)
		if err != nil {
			return fault.New("permissions.unsafe", err.Error())
		}
		return nil
	}
	if err := validate(); err != nil {
		return PermissionResult{}, err
	}
	if _, err := s.recover(expected, validate); err != nil {
		return PermissionResult{}, err
	}
	// Recovery has released the stale lock. Reacquire through the normal repair
	// entry point, which repeats both preflight and validation under the new lock.
	return FixPermissions(directory)
}

// PermissionLock exposes ownership evidence without requiring safe mode bits.
// It still refuses unsafe object types, owners, links, and ACLs before reading it.
func PermissionLock(directory string) (*LockInfo, error) {
	root, err := securefs.OpenForModeRepair(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if _, err := securefs.PlanModes(root); err != nil {
		return nil, err
	}
	return (&Store{Dir: directory, Root: root}).InspectLock()
}
