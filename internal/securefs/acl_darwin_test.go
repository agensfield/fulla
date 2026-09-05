package securefs

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNativeACLInspection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	present, err := hasACL(f)
	if err != nil || present {
		t.Fatalf("plain file: acl=%t err=%v", present, err)
	}
	// Only this disposable fixture is changed. A deny ACL is still intentional
	// policy and must not be silently removed or changed by mode repair.
	if out, err := exec.Command("/bin/chmod", "+a", "everyone deny delete", path).CombinedOutput(); err != nil {
		t.Fatalf("fixture ACL: %v %s", err, out)
	}
	t.Cleanup(func() {
		if out, err := exec.Command("/bin/chmod", "-a", "everyone deny delete", path).CombinedOutput(); err != nil {
			t.Errorf("remove fixture ACL: %v %s", err, out)
		}
	})
	present, err = hasACL(f)
	if err != nil || !present {
		t.Fatalf("ACL file: acl=%t err=%v", present, err)
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err := PlanModes(root); err == nil {
		t.Fatal("ACL-bearing repair accepted")
	}

	// POSIX mode bits alone look private; the ACL must still be detected.
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTree(root); err == nil {
		t.Fatal("ordinary validation accepted ACL")
	}
	if _, err := Read(root, filepath.Base(path), 1024); err == nil {
		t.Fatal("private read accepted ACL")
	}
	present, err = hasACL(f)
	if err != nil || !present {
		t.Fatal("inspection changed ACL", err)
	}

}
