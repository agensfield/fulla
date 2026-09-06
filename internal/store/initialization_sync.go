package store

import (
	"errors"
	"io/fs"
	"os"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func syncInitializationAt(root *os.Root, name string) error {
	stage, err := root.OpenRoot(name)
	if err != nil {
		return err
	}
	defer stage.Close()
	return syncInitialization(stage, syncInitializationFile, securefs.SyncDir)
}

func syncInitializationFile(root *os.Root, name string) error {
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}

// Fulla's writes already sync their files, but a freshly initialized Git
// repository contains files written by Git. Sync every staged regular file,
// then every directory bottom-up, before publishing the containing directory.
func syncInitialization(root *os.Root, syncFile, syncDir func(*os.Root, string) error) error {
	if err := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fault.New("store.unsafe", "non-regular entry in initialization staging")
		}
		return syncFile(root, name)
	}); err != nil {
		return err
	}
	return syncStagedDirectories(root, syncDir)
}
