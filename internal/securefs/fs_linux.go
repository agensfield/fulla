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
	switch uint64(stat.Type) {
	case 0x6969, 0xff534d42, 0x517b, 0x73757245, 0x564c:
		return fmt.Errorf("network filesystem is unsupported")
	}
	return nil
}
