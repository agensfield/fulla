package store

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/agensfield/fulla/internal/securefs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPermissionRepairPreservesBytesAndUsesSharedLock(t *testing.T) {
	s := fixture(t, true)
	if _, err := s.Write("binary", []byte{0, 255, 10}, false); err != nil {
		t.Fatal(err)
	}
	before := map[string][]byte{}
	for _, name := range []string{"identities", "recipients", "passwords/binary.age", "passwords/.git/config"} {
		data, err := s.Root.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		before[name] = data
	}
	lock, err := s.Lock("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Root.Chmod("identities", 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := FixPermissions(s.Dir); err == nil {
		t.Fatal("repaired while shared lock held")
	}
	info, err := s.Root.Stat("identities")
	if err != nil || info.Mode().Perm() != 0644 {
		t.Fatal("changed locked store")
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	if err := s.Root.Chmod("passwords", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(s.Dir, 0755); err != nil {
		t.Fatal(err)
	}
	result, err := FixPermissions(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Applied != 3 || len(result.Changes) != 3 || result.Receipt == "" {
		t.Fatalf("unexpected repair %+v", result)
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, data := range before {
		after, err := s.Root.ReadFile(name)
		if err != nil || !bytes.Equal(data, after) {
			t.Fatalf("bytes changed: %s", name)
		}
	}
	if err := s.DeepVerify(); err != nil {
		t.Fatal(err)
	}
	if err := s.Unlocked(); err != nil {
		t.Fatal(err)
	}
	repeated, err := FixPermissions(s.Dir)
	if err != nil || repeated.Applied != 0 || repeated.Receipt != "" {
		t.Fatal("repair not idempotent", err)
	}
}

func TestPermissionRepairRejectsUnsafeTreeBeforeAnyChmod(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "writable-directory", "pending"} {
		t.Run(kind, func(t *testing.T) {
			s := fixture(t, false)
			if err := s.Root.Chmod("identities", 0644); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "symlink":
				if err := os.Symlink(filepath.Join(s.Dir, "identities"), filepath.Join(s.Dir, "z-unsafe")); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(filepath.Join(s.Dir, "recipients"), filepath.Join(s.Dir, "z-unsafe")); err != nil {
					t.Fatal(err)
				}
			case "writable-directory":
				if err := os.Mkdir(filepath.Join(s.Dir, "z-unsafe"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(filepath.Join(s.Dir, "z-unsafe"), 0777); err != nil {
					t.Fatal(err)
				}
			case "pending":
				if err := s.Root.WriteFile(".fulla/pending.json", []byte("{}"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := FixPermissions(s.Dir); err == nil {
				t.Fatal("unsafe repair accepted")
			}
			info, err := s.Root.Stat("identities")
			if err != nil || info.Mode().Perm() != 0644 {
				t.Fatal("partial repair before preflight failure")
			}
			if _, err := s.Root.Lstat("lock"); !os.IsNotExist(err) {
				t.Fatal("left a lock", err)
			}
		})
	}
}

func TestPermissionRepairKilledOwnerRecovery(t *testing.T) {
	s := fixture(t, false)
	for _, name := range []string{"identities", "recipients"} {
		if err := s.Root.Chmod(name, 0644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestPermissionRepairCrashHelper$", "--", s.Dir)
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	reader := bufio.NewReader(output)
	token, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	token = strings.TrimSpace(token)
	// A live permission owner must not be displaced even with the right token.
	if _, err := RecoverAndFixPermissions(s.Dir, token); err == nil {
		t.Fatal("recovered live repair")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if _, err := RecoverAndFixPermissions(s.Dir, "wrong-token"); err == nil {
		t.Fatal("accepted wrong owner token")
	}
	result, err := RecoverAndFixPermissions(s.Dir, token)
	if err != nil {
		t.Fatal(err)
	}
	if result.Applied != 1 {
		t.Fatalf("expected one remaining repair: %+v", result)
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := s.Unlocked(); err != nil {
		t.Fatal(err)
	}
}

func TestPermissionRepairCrashHelper(t *testing.T) {
	args := os.Args
	index := -1
	for i, arg := range args {
		if arg == "--" {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	root, err := securefs.OpenForModeRepair(args[index+1])
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	s := &Store{Dir: args[index+1], Root: root}
	var changes []securefs.ModeChange
	lock, err := s.lock("fix-permissions", func() error { var err error; changes, err = securefs.PlanModes(root); return err })
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatal("unexpected fixture repair count")
	}
	if _, err := securefs.ApplyModes(root, changes[:1]); err != nil {
		t.Fatal(err)
	}
	fmt.Println(lock.Token)
	time.Sleep(time.Hour)
}
