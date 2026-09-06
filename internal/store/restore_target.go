package store

import (
	"os"
	"path/filepath"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

// CheckRestoreTarget rejects impossible publication before selecting recovery
// input. Restore rechecks this condition and retains no-replace publication.
func CheckRestoreTarget(target string) error {
	_, _, err := checkRestoreTarget(target)
	return err
}

func checkRestoreTarget(target string) (string, bool, error) {
	target, err := securefs.Canonical(target, true)
	if err != nil {
		return "", false, fault.New("recovery.unsafe_target", err.Error())
	}
	emptyExisting := false
	if info, err := os.Lstat(target); err == nil {
		if !info.IsDir() {
			return "", false, fault.New("recovery.target_exists", "restore target must be empty")
		}
		if err := securefs.ValidateInfo(target, info, true); err != nil {
			return "", false, err
		}
		entries, err := os.ReadDir(target)
		if err != nil || len(entries) != 0 {
			return "", false, fault.New("recovery.target_exists", "restore target must be empty")
		}
		emptyExisting = true
	} else if !os.IsNotExist(err) {
		return "", false, err
	}
	_, err = securefs.Canonical(filepath.Dir(target), false)
	if err != nil {
		return "", false, fault.New("recovery.invalid_target", "restore parent must already exist")
	}
	return target, emptyExisting, nil
}
