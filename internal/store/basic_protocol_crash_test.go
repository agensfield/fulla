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

	"github.com/agensfield/fulla/internal/crypt"
)

func TestBasicProtocolCrashHelper(t *testing.T) {
	dir := os.Getenv("FULLA_BASIC_CRASH_FIXTURE")
	if dir == "" {
		return
	}
	s, err := Open(dir, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if token := os.Getenv("FULLA_BASIC_DENIAL_TOKEN"); token != "" {
		owner, err := s.InspectLock()
		if err != nil || owner == nil {
			t.Fatal(err)
		}
		if _, err := s.recover(token, func() error {
			if err := s.Validate(); err != nil {
				return err
			}
			return s.Root.Chmod(transactionBase(basicProtocol)+"/"+owner.StageID, 0500)
		}); err == nil {
			t.Fatal("expected cleanup denial")
		}
		owner, err = s.InspectLock()
		if err != nil || owner == nil || owner.Token == token || owner.StageProtocol != basicProtocol {
			t.Fatal("lost replacement ownership", err)
		}
		fmt.Println("recovery-denied", owner.Token)
		return
	}
	lock, err := s.lockMutation("move")
	if err != nil {
		t.Fatal(err)
	}
	_, rs, err := s.Keys()
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := crypt.Encrypt([]byte{0, 255, 10}, rs)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.mutate(lock, "move", map[string][]byte{"a": nil, "b": ciphertext}, func(phase string) error {
		if phase == os.Getenv("FULLA_BASIC_CRASH_PHASE") {
			fmt.Println("basic-ready", lock.Token)
			for {
				time.Sleep(time.Hour)
			}
		}
		return nil
	})
	t.Fatal("missing crash checkpoint", err)
}

func TestBasicProtocolKilledWriterRecovery(t *testing.T) {
	for _, git := range []bool{false, true} {
		for _, phase := range []string{"staged", "journaled", "published:a", "committed", "receipted", "staged-cleanup-denial"} {
			t.Run(fmt.Sprintf("git=%v/%s", git, phase), func(t *testing.T) {
				if phase == "staged-cleanup-denial" && os.Geteuid() == 0 {
					t.Skip("requires real unprivileged removal denial")
				}
				s := fixture(t, git)
				if _, err := s.Write("a", []byte{0, 255, 10}, false); err != nil {
					t.Fatal(err)
				}
				before := futureBasicFixture(t, s, true)
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBasicProtocolCrashHelper$")
				stop := phase
				if phase == "staged-cleanup-denial" {
					stop = "staged"
				}
				child.Env = append(os.Environ(), "FULLA_BASIC_CRASH_FIXTURE="+s.Dir, "FULLA_BASIC_CRASH_PHASE="+stop)
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
				if err != nil || len(words) != 2 || words[0] != "basic-ready" {
					t.Fatal("child readiness", line, err)
				}
				token := words[1]
				if _, err := s.Recover(token); err == nil {
					t.Fatal("recovered live owner")
				}
				if err := child.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				_ = child.Wait()
				owner, err := s.InspectLock()
				if err != nil || owner == nil || owner.StageProtocol != basicProtocol || owner.StageID == "" {
					t.Fatal("lost basic binding", err)
				}
				if phase == "staged-cleanup-denial" {
					stage := transactionBase(basicProtocol) + "/" + owner.StageID
					t.Cleanup(func() { _ = s.Root.Chmod(stage, 0700) })
					denied := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBasicProtocolCrashHelper$")
					denied.Env = append(os.Environ(), "FULLA_BASIC_CRASH_FIXTURE="+s.Dir, "FULLA_BASIC_DENIAL_TOKEN="+token)
					if output, err := denied.CombinedOutput(); err != nil || !strings.Contains(string(output), "recovery-denied") {
						t.Fatal("cleanup denial helper failed", err)
					}
					replacement, err := s.InspectLock()
					if err != nil || replacement == nil || replacement.Token == token || replacement.StageProtocol != basicProtocol || replacement.StageID != owner.StageID {
						t.Fatal("lost retry binding", err)
					}
					if err := s.Root.Chmod(stage, 0700); err != nil {
						t.Fatal(err)
					}
					if _, err := s.Recover(token); err == nil {
						t.Fatal("accepted superseded token")
					}
					token = replacement.Token

				}
				result, err := s.Recover(token)
				if err != nil || result["protocol"] != basicProtocol || result["lock_released"] != true {
					t.Fatal("basic recovery", result, err)
				}
				retained, removed := "b", "a"
				if stop == "staged" {
					retained, removed = "a", "b"
				}
				value, err := s.Read(retained)
				if err != nil || string(value) != string([]byte{0, 255, 10}) {
					t.Fatal("lost exact value", err)
				}
				if exists, err := s.Exists(removed); err != nil || exists {
					t.Fatal("incorrect recovered names", err)
				}
				if !reflect.DeepEqual(before, basicForeignTree(t, s)) {
					t.Fatal("recovery changed unknown feature metadata")
				}
				if err := s.Unlocked(); err != nil {
					t.Fatal(err)
				}
				stages, err := s.Root.Open(transactionBase(basicProtocol))
				if err != nil {
					t.Fatal(err)
				}
				entries, err := stages.ReadDir(-1)
				stages.Close()
				if err != nil || len(entries) != 0 {
					t.Fatal("retained basic stage", err)
				}
			})
		}
	}
}
