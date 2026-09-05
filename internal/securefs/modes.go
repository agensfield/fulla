package securefs

import (
	"fmt"
	"io/fs"
	"os"
)

type ModeChange struct {
	Path   string      `json:"path"`
	Before fs.FileMode `json:"before"`
	After  fs.FileMode `json:"after"`
	info   fs.FileInfo
}

// PlanModes reads metadata only. Every object must be safe apart from ordinary
// mode bits, and all ACLs are preserved by refusing repair on ACL-bearing trees.
func PlanModes(root *os.Root) ([]ModeChange, error) {
	changes := []ModeChange{}
	err := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if err := validateObject(name, info); err != nil {
			return err
		}
		if info.IsDir() && info.Mode().Perm()&0022 != 0 {
			return fmt.Errorf("writable shared directory requires manual review: %s", name)
		}
		f, err := inspectModeFile(root, name, info)
		if err != nil {
			return err
		}
		defer f.Close()
		mode := info.Mode().Perm() & 0700
		if info.IsDir() {
			mode = 0700
		}
		if mode != info.Mode().Perm() {
			changes = append(changes, ModeChange{Path: name, Before: info.Mode().Perm(), After: mode, info: info})
		}
		return nil
	})
	return changes, err
}

func inspectModeFile(root *os.Root, name string, info fs.FileInfo) (*os.File, error) {
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) || opened.Mode() != info.Mode() {
		f.Close()
		return nil, fmt.Errorf("path changed during mode inspection: %s", name)
	}
	acl, err := hasACL(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("cannot inspect ACL: %s: %w", name, err)
	}
	if acl {
		f.Close()
		return nil, fmt.Errorf("intentional ACL requires manual review: %s", name)
	}
	return f, nil
}

// ApplyModes must run under the shared store lock. Partial progress never widens
// access for other users, is returned explicitly, and can be safely replanned after a crash.
func ApplyModes(root *os.Root, changes []ModeChange) (int, error) {
	for i, change := range changes {
		info, err := root.Lstat(change.Path)
		if err != nil {
			return i, err
		}
		if change.info == nil || !os.SameFile(info, change.info) || info.Mode().Perm() != change.Before {
			return i, fmt.Errorf("mode repair target changed: %s", change.Path)
		}
		f, err := inspectModeFile(root, change.Path, info)
		if err != nil {
			return i, err
		}
		err = f.Chmod(change.After)
		if err != nil {
			f.Close()
			return i, err
		}
		err = f.Sync()
		closeErr := f.Close()
		if err != nil {
			return i + 1, err
		}
		if closeErr != nil {
			return i + 1, closeErr
		}
	}
	return len(changes), nil
}
