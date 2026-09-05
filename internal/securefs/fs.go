// Package securefs provides rooted private-file operations for local stores.
package securefs

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Canonical rejects symlinks in every existing component. Missing suffixes are
// allowed only for creation paths; callers still publish through a rooted parent.
func Canonical(name string, allowMissing bool) (string, error) {
	abs, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(abs, current), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) && allowMissing {
			return abs, nil
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return "", fmt.Errorf("symlink path component: %s", current)
		}
	}
	return abs, nil
}

func validateObject(name string, info fs.FileInfo) error {
	if !info.IsDir() && !info.Mode().IsRegular() {
		return fmt.Errorf("non-regular private path: %s", name)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot inspect ownership: %s", name)
	}
	if stat.Uid != uint32(os.Getuid()) {
		return fmt.Errorf("foreign owner: %s", name)
	}
	if info.Mode().IsRegular() && stat.Nlink != 1 {
		return fmt.Errorf("hard-linked private file: %s", name)
	}
	if info.Mode()&(fs.ModeSetuid|fs.ModeSetgid|fs.ModeSticky) != 0 {
		return fmt.Errorf("special mode bits: %s", name)
	}
	return nil
}

func ValidateInfo(name string, info fs.FileInfo, private bool) error {
	if err := validateObject(name, info); err != nil {
		return err
	}
	forbidden := fs.FileMode(0o022)
	if private {
		forbidden = 0o077
	}
	if info.Mode().Perm()&forbidden != 0 {
		return fmt.Errorf("unsafe permissions %04o: %s", info.Mode().Perm(), name)
	}
	if info.IsDir() && info.Mode().Perm()&0o700 != 0o700 {
		return fmt.Errorf("owner directory access required: %s", name)
	}
	return nil
}

func Open(name string) (*os.Root, error) { return open(name, false) }

// OpenForModeRepair permits mode issues only; callers must inspect the whole
// tree and acquire the shared store lock before applying any repairs.
func OpenForModeRepair(name string) (*os.Root, error) { return open(name, true) }

func open(name string, repair bool) (*os.Root, error) {
	name, err := Canonical(name, false)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", name)
	}
	if err := validateObject(name, info); err != nil {
		return nil, err
	}
	if !repair {
		if err := ValidateInfo(name, info, true); err != nil {
			return nil, err
		}
	}
	root, err := os.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		root.Close()
		return nil, fmt.Errorf("directory changed while opening: %s", name)
	}
	if err := localFilesystem(root); err != nil {
		root.Close()
		return nil, err
	}
	return root, nil
}

func ValidateTree(root *os.Root) error {
	return fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return ValidateInfo(name, info, true)
	})
}

// Read validates both the directory entry and the opened inode. The private
// owning-user trust model excludes malicious same-user processes.
func Read(root *os.Root, name string, limit int64) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if err := ValidateInfo(name, info, true); err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a file: %s", name)
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("file changed while opening: %s", name)
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file exceeds size limit: %s", name)
	}
	return data, nil
}

func SyncDir(root *os.Root, name string) error {
	f, err := root.Open(name)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}

func WriteNew(root *os.Root, name string, data []byte) (err error) {
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	err = errors.Join(writeErr, f.Sync(), f.Close())
	if err != nil {
		_ = root.Remove(name)
		return err
	}
	return SyncDir(root, filepath.Dir(name))
}

// PublishNew exposes only a complete synced file, with atomic no-replace rename.
func PublishNew(root *os.Root, name string, data []byte) error {
	parent, err := root.OpenRoot(filepath.Dir(name))
	if err != nil {
		return err
	}
	defer parent.Close()
	tmp := ".fulla-stage-" + ID()
	if err := WriteNew(parent, tmp, data); err != nil {
		return err
	}
	defer parent.Remove(tmp)
	if err := RenameNew(parent, tmp, filepath.Base(name)); err != nil {
		return err
	}
	return SyncDir(parent, ".")
}

// Replace atomically updates an already-owned file. No-replace publication is
// a distinct operation; ordinary create paths must not use Replace.
func Replace(root *os.Root, name string, data []byte) error {
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if err := ValidateInfo(name, info, true); err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a file: %s", name)
	}
	tmp := filepath.Join(filepath.Dir(name), ".fulla-stage-"+ID())
	if err := WriteNew(root, tmp, data); err != nil {
		return err
	}
	defer root.Remove(tmp)
	if err := root.Rename(tmp, name); err != nil {
		return err
	}
	return SyncDir(root, filepath.Dir(name))
}

func ID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		panic("random source unavailable")
	}
	return hex.EncodeToString(data[:])
}
