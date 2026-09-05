package securefs

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

func RenameNew(root *os.Root, from, to string) error {
	if filepath.Base(from) != from || filepath.Base(to) != to {
		return fmt.Errorf("publication requires direct child names")
	}
	f, err := root.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	return unix.Renameat2(int(f.Fd()), from, int(f.Fd()), to, unix.RENAME_NOREPLACE)
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
	switch uint64(stat.Type) {
	case 0x6969, 0xff534d42, 0x517b, 0x73757245, 0x564c:
		return fmt.Errorf("network filesystem is unsupported")
	}
	return nil
}
