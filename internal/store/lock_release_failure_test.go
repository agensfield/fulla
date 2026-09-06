package store

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestDetachedReleaseCleanupDenialCanRetry(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires actual unprivileged removal denial")
	}
	s := fixture(t, false)
	before := archiveTree(t, s, false)
	lock, err := s.Lock("cleanup-denial")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if lock.releaseDir != "" {
			_ = s.Root.Chmod(lock.releaseDir, 0700)
		}
	})
	err = lock.release(func(phase string) error {
		if phase == "cleanup-validated" {
			return s.Root.Chmod(lock.releaseDir, 0500)
		}
		return nil
	})
	var failure *fault.Error
	if !errors.As(err, &failure) || failure.Code != "store.lock_cleanup_failed" || failure.Details["lock_released"] != true || failure.Details["token"] != lock.Token {
		t.Fatal("lost detached cleanup result", err)
	}
	if err := s.Unlocked(); err != nil {
		t.Fatal("cleanup denial stranded store", err)
	}
	if err := s.Root.Chmod(lock.releaseDir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"info", "owner"} {
		if _, err := s.Root.Lstat(lock.releaseDir + "/" + name); err != nil {
			t.Fatal("denial removed ownership", err)
		}
	}
	newer, err := s.Lock("newer-writer")
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal("owned cleanup retry failed", err)
	}
	owner, err := s.InspectLock()
	if err != nil || owner.Token != newer.Token {
		t.Fatal("retry changed newer lock", err)
	}
	if err := newer.Release(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, archiveTree(t, s, false)) {
		t.Fatal("retry changed completed store")
	}
}

func TestDetachedCleanupRefusesUnownedContents(t *testing.T) {
	for _, kind := range []string{"unknown-file", "wrong-owner", "future-version", "remote-owner", "duplicate-token", "nonempty-guard", "unsafe-mode"} {
		t.Run(kind, func(t *testing.T) {
			s := fixture(t, false)
			lock, err := s.Lock("refusal-fixture")
			if err != nil {
				t.Fatal(err)
			}
			_ = lock.release(func(phase string) error {
				if phase == "detached" {
					return errors.New("fixture stop")
				}
				return nil
			})
			child := exec.Command("true")
			if err := child.Run(); err != nil {
				t.Fatal(err)
			}
			// Reconstruct a dead owner for malformed-state refusal cases. Actual killed
			// writers and live-owner refusal have a separate subprocess matrix.
			name := strings.Replace(lock.releaseDir, "-"+strconv.Itoa(os.Getpid())+"-", "-"+strconv.Itoa(child.ProcessState.Pid())+"-", 1)
			if err := securefs.RenameNew(s.Root, lock.releaseDir, name); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "unknown-file":
				err = securefs.WriteNew(s.Root, name+"/private-fixture", []byte("preserve"))
			case "wrong-owner":
				err = securefs.Replace(s.Root, name+"/owner", []byte(securefs.ID()+"\n"))
			case "future-version":
				next := strings.Replace(name, "-v1-", "-v2-", 1)
				err = securefs.RenameNew(s.Root, name, next)
			case "remote-owner":
				binding, _ := releaseBinding(name)
				next := strings.Replace(name, binding.Host, digest([]byte("other-fixture-host")), 1)
				err = securefs.RenameNew(s.Root, name, next)
			case "duplicate-token":
				next := strings.Replace(name, fmt.Sprint("-", child.ProcessState.Pid(), "-"), "-1-", 1)
				err = s.Root.Mkdir(next, 0700)
			case "unsafe-mode":
				err = s.Root.Chmod(name+"/info", 0644)
			case "nonempty-guard":
				err = securefs.WriteNew(s.Root, name+"/recovery", []byte("preserve"))
			}
			if err != nil {
				t.Fatal(err)
			}
			before := archiveTree(t, s, false)
			if _, err := s.Recover(lock.Token); err == nil {
				t.Fatal("accepted invalid cleanup")
			}
			if !reflect.DeepEqual(before, archiveTree(t, s, false)) {
				t.Fatal("refusal modified evidence")
			}
		})
	}
}
