package store

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/agensfield/fulla/internal/fault"
)

func TestInitializationCrashHelper(t *testing.T) {
	target := os.Getenv("FULLA_INIT_CRASH_TARGET")
	if target == "" {
		return
	}
	_, err := initialize(target, os.Getenv("FULLA_INIT_CRASH_NO_GIT") == "1", false, func(phase string) error {
		if phase == os.Getenv("FULLA_INIT_CRASH_PHASE") {
			fmt.Println("init-ready")
			time.Sleep(time.Minute)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInitializationCleanupFailureHelper(t *testing.T) {
	directory := os.Getenv("FULLA_INIT_RECOVERY_DENY")
	if directory == "" {
		return
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	s := &Store{Dir: directory, Root: root}
	owner, err := s.InspectLock()
	if err != nil || owner == nil {
		t.Fatal("missing owner", err)
	}
	_, err = s.recover(owner.Token, func() error {
		if err := s.validateInitialization(); err != nil {
			return err
		}
		return os.Chmod(directory, 0500)
	})
	var problem *fault.Error
	if !errors.As(err, &problem) || problem.Code != "store.cleanup_failed" || problem.Details["applied"] != false {
		t.Fatal("missing cleanup failure", err)
	}
}

func TestKilledInitializationRecovery(t *testing.T) {
	for _, noGit := range []bool{false, true} {
		for _, phase := range []string{"bound", "keys", "staged", "published"} {
			t.Run(fmt.Sprintf("no-git=%v/%s", noGit, phase), func(t *testing.T) {
				parent, err := filepath.EvalSymlinks(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(parent, "store with spaces")
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInitializationCrashHelper$")
				mode := "0"
				if noGit {
					mode = "1"
				}
				cmd.Env = append(os.Environ(), "FULLA_INIT_CRASH_TARGET="+target, "FULLA_INIT_CRASH_NO_GIT="+mode, "FULLA_INIT_CRASH_PHASE="+phase)
				pipe, err := cmd.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if cmd.ProcessState == nil {
						_ = cmd.Process.Kill()
						_ = cmd.Wait()
					}
				}()
				line, err := bufio.NewReader(pipe).ReadString('\n')
				if err != nil || line != "init-ready\n" {
					t.Fatal("missing boundary", err)
				}
				location := target
				if phase != "published" {
					stages, err := filepath.Glob(filepath.Join(parent, ".fulla-init-*"))
					if err != nil || len(stages) != 1 {
						t.Fatal("missing stage", err)
					}
					location = stages[0]
				}
				root, err := os.OpenRoot(location)
				if err != nil {
					t.Fatal(err)
				}
				defer root.Close()
				s := &Store{Dir: location, Root: root}
				owner, err := s.InspectLock()
				if err != nil || owner == nil || owner.InitID == "" || owner.InitTarget != target {
					t.Fatal("missing init binding", err)
				}
				before := archiveTree(t, s, false)
				if _, err := RecoverInitialization(location, owner.Token); err == nil {
					t.Fatal("stole live initializer")
				}
				if !reflect.DeepEqual(before, archiveTree(t, s, false)) {
					t.Fatal("live refusal mutated stage")
				}
				if err := cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				_ = cmd.Wait()
				if _, err := RecoverInitialization(location, "wrong-owner"); err == nil {
					t.Fatal("accepted wrong token")
				}
				if !reflect.DeepEqual(before, archiveTree(t, s, false)) {
					t.Fatal("wrong-token refusal mutated stage")
				}
				if phase == "staged" && os.Geteuid() != 0 {
					recovery := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInitializationCleanupFailureHelper$")
					recovery.Env = append(os.Environ(), "FULLA_INIT_RECOVERY_DENY="+location)
					output, err := recovery.CombinedOutput()
					if err != nil {
						t.Fatalf("cleanup failure helper: %v %s", err, output)
					}
					// Restore only the permission deliberately changed after validation.
					if err := os.Chmod(location, 0700); err != nil {
						t.Fatal(err)
					}
					again, err := s.InspectLock()
					if err != nil || again == nil || again.Token == owner.Token || again.InitID != owner.InitID || again.InitTarget != target || again.Alive {
						t.Fatal("lost replacement ownership", err)
					}
					if _, err := RecoverInitialization(location, owner.Token); err == nil {
						t.Fatal("accepted stale token after takeover")
					}
					owner = again
				}
				reused := filepath.Join(parent, ".fulla-init-"+owner.InitID)
				if phase == "published" {
					if err := os.Mkdir(reused, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(reused, "new-occupant"), []byte("preserved"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				result, err := RecoverInitialization(location, owner.Token)
				if err != nil || result["init_applied"] != (phase == "published") || result["lock_released"] != true {
					t.Fatal("recovery failed", result, err)
				}
				if phase == "published" {
					after := archiveTree(t, s, false)
					for name := range before {
						if name == "lock" || len(name) > 5 && name[:5] == "lock/" {
							delete(before, name)
						}
					}
					if !reflect.DeepEqual(before, after) {
						t.Fatal("published recovery changed store material")
					}
					data, err := os.ReadFile(filepath.Join(reused, "new-occupant"))
					if err != nil || string(data) != "preserved" {
						t.Fatal("removed reused stage", err)
					}
				} else {
					if _, err := os.Lstat(location); !os.IsNotExist(err) {
						t.Fatal("stage retained", err)
					}
					if _, err := os.Lstat(target); !os.IsNotExist(err) {
						t.Fatal("recovery published implicitly", err)
					}
					if _, err := Init(target, noGit, false); err != nil {
						t.Fatal("explicit retry failed", err)
					}
				}
				complete, err := Open(target, true, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer complete.Close()
				if err := complete.DeepVerify(); err != nil {
					t.Fatal(err)
				}
				if err := complete.Unlocked(); err != nil {
					t.Fatal(err)
				}
				if _, err := RecoverInitialization(target, owner.Token); err == nil {
					t.Fatal("repeated recovery accepted missing lock")
				}
			})
		}
	}
}
