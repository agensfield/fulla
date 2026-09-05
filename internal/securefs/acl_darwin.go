package securefs

import (
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// hasACL uses the descriptor-based Darwin attribute API. The variable payload
// is a kauth_filesec (sys/attr.h and sys/kauth.h), not a POSIX mode mask.
func hasACL(file *os.File) (bool, error) {
	attrs := unix.Attrlist{Bitmapcount: 5, Commonattr: unix.ATTR_CMN_EXTENDED_SECURITY}
	// Darwin limits ACLs to 128 entries of 24 bytes, plus a 44-byte header.
	var data [4096]byte
	_, _, errno := unix.Syscall6(unix.SYS_FGETATTRLIST, file.Fd(), uintptr(unsafe.Pointer(&attrs)), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0, 0)
	runtime.KeepAlive(file)
	if errno != 0 {
		return false, errno
	}
	return darwinACLPresent(data[:])
}

func darwinACLPresent(data []byte) (bool, error) {
	if len(data) < 12 {
		return false, fmt.Errorf("truncated ACL attributes")
	}
	total := int(binary.LittleEndian.Uint32(data[:4]))
	offset := int(int32(binary.LittleEndian.Uint32(data[4:8]))) + 4
	length := int(binary.LittleEndian.Uint32(data[8:12]))
	if total < 12 || total > len(data) || offset < 12 || length < 0 || offset > total || length > total-offset {
		return false, fmt.Errorf("invalid ACL attribute bounds")
	}
	if length == 0 {
		return false, nil
	}
	if length < 44 {
		return false, fmt.Errorf("truncated file security attributes")
	}
	security := data[offset : offset+length]
	count := binary.LittleEndian.Uint32(security[36:40])
	if count == 0xffffffff {
		return false, nil
	}
	if count > 128 || length != 44+int(count)*24 {
		return false, fmt.Errorf("invalid ACL entry count")
	}
	// Even an explicit empty ACL can carry inheritance policy. Preserve it.
	return true, nil
}
