package store

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agensfield/fulla/internal/securefs"
)

func TestLockReleaseCrashHelper(t *testing.T) {
	directory := os.Getenv("FULLA_RELEASE_FIXTURE")
	if directory == "" {
		return
	}
	s, err := Open(directory, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	lock, err := s.Lock("release-fixture")
	if err != nil {
		t.Fatal(err)
	}
	err = lock.release(func(phase string) error {
		if phase == os.Getenv("FULLA_RELEASE_PHASE") {
			fmt.Println("release-ready", lock.Token)
			for {
				time.Sleep(time.Hour)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Fatal("missing release checkpoint")
}

func TestKilledLockReleasePreservesRecoveryAndNewWriter(t *testing.T) {
	for _, git := range []bool{false, true} {
		for _, phase := range []string{"ready", "detached", "synced", "info-removed", "owner-removed", "empty"} {
			t.Run(fmt.Sprintf("git=%v/%s", git, phase), func(t *testing.T) {
				s := fixture(t, git)
				before := archiveTree(t, s, false)
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLockReleaseCrashHelper$")
				child.Env = append(os.Environ(), "FULLA_RELEASE_FIXTURE="+s.Dir, "FULLA_RELEASE_PHASE="+phase)
				out, err := child.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				if err := child.Start(); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if child.ProcessState == nil {
						_ = child.Process.Kill()
						_ = child.Wait()
					}
				}()
				line, err := bufio.NewReader(out).ReadString('\n')
				words := strings.Fields(line)
				if err != nil || len(words) != 2 || words[0] != "release-ready" || !validID(words[1]) {
					t.Fatal("missing release readiness", err, line)
				}
				token := words[1]
				if _, err := s.Recover(token); err == nil {
					t.Fatal("accepted live release owner")
				}
				if err := child.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				_ = child.Wait()
				if phase == "ready" {
					owner, err := s.InspectLock()
					if err != nil || owner == nil || owner.Token != token {
						t.Fatal("active ownership lost before detach", err)
					}
					if _, err := s.Lock("competing-writer"); err == nil {
						t.Fatal("writer entered before detach")
					}
					if _, err := s.Recover(token); err != nil {
						t.Fatal("active recovery failed", err)
					}
				} else {
					if err := s.Unlocked(); err != nil {
						t.Fatal("detached cleanup stranded store", err)
					}
					report, err := s.Doctor(false)
					if err != nil || report.Healthy || len(report.LockCleanup) != 1 || report.LockCleanup[0].Token != token {
						t.Fatal("cleanup inspection lost token", err)
					}
					if err := s.CheckLockCleanup(); err == nil {
						t.Fatal("full archive accepted cleanup residue")
					}
					newer, err := s.Lock("newer-writer")
					if err != nil {
						t.Fatal("new writer could not enter", err)
					}
					newerOwner, err := s.InspectLock()
					if err != nil {
						t.Fatal(err)
					}
					unchanged := archiveTree(t, s, false)
					if _, err := s.Recover(securefs.ID()); err == nil {
						t.Fatal("accepted wrong recovery token")
					}
					if !reflect.DeepEqual(unchanged, archiveTree(t, s, false)) {
						t.Fatal("wrong token mutated store")
					}
					result, err := s.Recover(token)
					if err != nil || result["lock_released"] != true {
						t.Fatal("detached recovery failed", result, err)
					}
					owner, err := s.InspectLock()
					if err != nil || !reflect.DeepEqual(newerOwner, owner) {
						t.Fatal("cleanup changed newer writer", err)
					}
					if err := newer.Release(); err != nil {
						t.Fatal(err)
					}
				}
				if !reflect.DeepEqual(before, archiveTree(t, s, false)) {
					t.Fatal("release recovery changed completed store")
				}
				if err := s.CheckLockCleanup(); err != nil {
					t.Fatal("cleanup residue retained", err)
				}
			})
		}
	}
}
