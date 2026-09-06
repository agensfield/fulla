package securefs

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

func RenameNew(root *os.Root, from, to string) error {
	return RenameNewBetween(root, from, root, to)
}

// RenameNewBetween atomically publishes between two already-confined parents.
// Neither argument is interpreted as a path beneath its directory descriptor.
func RenameNewBetween(fromRoot *os.Root, from string, toRoot *os.Root, to string) error {
	if filepath.Base(from) != from || filepath.Base(to) != to || from == "." || from == ".." || to == "." || to == ".." {
		return fmt.Errorf("publication requires direct child names")
	}
	source, err := fromRoot.Open(".")
	if err != nil {
		return err
	}
	defer source.Close()
	destination, err := toRoot.Open(".")
	if err != nil {
		return err
	}
	defer destination.Close()
	return unix.RenameatxNp(int(source.Fd()), from, int(destination.Fd()), to, unix.RENAME_EXCL)
}

func localFilesystem(root *os.Root) error {
	f, err := root.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	var stat unix.Statfs_t
	if err := unix.Fstatfs(int(f.Fd()), &stat); err != nil {
		return err
	}
	if stat.Flags&unix.MNT_LOCAL == 0 {
		return fmt.Errorf("network filesystem is unsupported")
	}
	return nil
}
