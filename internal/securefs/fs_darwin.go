package securefs

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

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
