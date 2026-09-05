package securefs

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestNativeACLInspection(t *testing.T) {
	name := filepath.Join(t.TempDir(), "fixture")
	if err := os.WriteFile(name, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	present, err := hasACL(f)
	if err != nil || present {
		t.Fatalf("plain: %t %v", present, err)
	}
	// Linux POSIX ACL xattr v2: user_obj, named user, group_obj, mask, other.
	data := make([]byte, 4+5*8)
	binary.LittleEndian.PutUint32(data, 2)
	entries := []struct {
		tag, perm uint16
		id        uint32
	}{{1, 6, 0xffffffff}, {2, 4, uint32(os.Getuid() + 1)}, {4, 0, 0xffffffff}, {16, 4, 0xffffffff}, {32, 0, 0xffffffff}}
	for i, e := range entries {
		offset := 4 + i*8
		binary.LittleEndian.PutUint16(data[offset:], e.tag)
		binary.LittleEndian.PutUint16(data[offset+2:], e.perm)
		binary.LittleEndian.PutUint32(data[offset+4:], e.id)
	}
	if err := unix.Fsetxattr(int(f.Fd()), "system.posix_acl_access", data, 0); err != nil {
		t.Fatal(err)
	}
	present, err = hasACL(f)
	if err != nil || !present {
		t.Fatalf("ACL: %t %v", present, err)
	}
	root, err := os.OpenRoot(filepath.Dir(name))
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err := PlanModes(root); err == nil {
		t.Fatal("ACL-bearing repair accepted")
	}
}
