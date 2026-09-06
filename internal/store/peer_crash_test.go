package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestPeerCrashHelper(t *testing.T) {
	dir := os.Getenv("FULLA_PEER_CRASH_FIXTURE")
	if dir == "" {
		return
	}
	s, err := Open(dir, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var p Peer
	if err := json.Unmarshal([]byte(os.Getenv("FULLA_PEER_CRASH_PUBLIC_RECORD")), &p); err != nil {
		t.Fatal(err)
	}
	pause := func() error { fmt.Println("peer-published"); time.Sleep(time.Minute); return nil }
	switch operation := os.Getenv("FULLA_PEER_CRASH_OPERATION"); operation {
	case "rotate-error", "remove-error":
		failed := func(phase string) error {
			if phase == "published" {
				return errors.New("fixture handled publication error")
			}
			return nil
		}
		if operation == "rotate-error" {
			err = s.savePeer(p, true, os.Getenv("FULLA_PEER_CRASH_OLD_PIN"), failed)
		} else {
			err = s.removePeerConfirmed(p.Name, nil, failed)
		}
		var problem *fault.Error
		if !errors.As(err, &problem) || problem.Status != 3 || problem.Details["applied"] != true || problem.Details["recovery_required"] != true {
			t.Fatal("lost recoverable failure evidence", err)
		}
		fmt.Println("peer-error-retained")
		return
	case "recover":
		_, err = s.Recover(os.Getenv("FULLA_PEER_CRASH_OLD_PIN"))
		if err == nil {
			t.Fatal("fixture expected recovery refusal")
		}
		_ = pause()
		return
	case "enroll", "rotate", "legacy-rotate":
		hook := func(phase string) error {
			wanted := os.Getenv("FULLA_PEER_CRASH_PHASE")
			if wanted == "" {
				wanted = "published"
			}
			if phase == wanted {
				return pause()
			}
			return nil
		}
		if operation == "legacy-rotate" {
			hook = func(phase string) error {
				if phase != "published" {
					return nil
				}
				info, err := securefs.Read(s.Root, "lock/info", 4096)
				if err != nil {
					return err
				}
				fields := []string{}
				for _, field := range strings.Fields(string(info)) {
					if !strings.HasPrefix(field, "peer_receipt=") {
						fields = append(fields, field)
					}
				}
				if err := securefs.Replace(s.Root, "lock/info", []byte(strings.Join(fields, " ")+"\n")); err != nil {
					return err
				}
				return pause()
			}
		}
		err = s.savePeer(p, operation != "enroll", os.Getenv("FULLA_PEER_CRASH_OLD_PIN"), hook)
	case "remove":
		err = s.removePeerConfirmed(p.Name, nil, func(phase string) error {
			wanted := os.Getenv("FULLA_PEER_CRASH_PHASE")
			if wanted == "" {
				wanted = "published"
			}
			if phase == wanted {
				return pause()
			}
			return nil
		})
	case "dry-run", "activate":
		identity, e := s.IdentityShow()
		if e != nil {
			t.Fatal(e)
		}
		err = s.markPeer(p.Name, p.Fingerprint, identity.Fingerprint, operation == "activate", pause)
	default:
		t.Fatal("unknown fixture operation")
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestPeerPublicationKilledOwner(t *testing.T) {
	for _, git := range []bool{false, true} {
		for _, operation := range []string{"enroll", "rotate", "legacy-rotate", "remove", "dry-run", "activate"} {
			phases := []string{"published"}
			if operation == "rotate" || operation == "remove" {
				phases = []string{"prepared", "published", "receipted"}
			}
			for _, phase := range phases {
				t.Run(fmt.Sprintf("git=%t/%s/%s", git, operation, phase), func(t *testing.T) {
					s := fixture(t, git)
					value := []byte{0, 255, 10, 65}
					if _, err := s.Write("entry", value, false); err != nil {
						t.Fatal(err)
					}
					ciphertext, err := securefs.Read(s.Root, "passwords/entry.age", maxMetadata)
					if err != nil {
						t.Fatal(err)
					}
					identity, err := s.IdentityShow()
					if err != nil {
						t.Fatal(err)
					}
					keys, err := securefs.Read(s.Root, "identities", maxMetadata)
					if err != nil {
						t.Fatal(err)
					}
					original := Peer{Version: 1, Name: "fixture", Host: "fixture", Recipient: identity.Recipient, Fingerprint: identity.Fingerprint}
					if operation != "enroll" {
						if err := s.SavePeer(original, false, ""); err != nil {
							t.Fatal(err)
						}
					}
					next := original
					if operation == "rotate" || operation == "legacy-rotate" {
						_, recipient, err := crypt.Generate()
						if err != nil {
							t.Fatal(err)
						}
						next.Recipient = recipient
						next.Fingerprint = crypt.Fingerprint(recipient)
						next.Host = "rotated-fixture"
					}
					public, err := json.Marshal(next)
					if err != nil {
						t.Fatal(err)
					}
					cmd := exec.Command(os.Args[0], "-test.run=^TestPeerCrashHelper$")
					cmd.Env = append(os.Environ(), "FULLA_PEER_CRASH_FIXTURE="+s.Dir, "FULLA_PEER_CRASH_OPERATION="+operation, "FULLA_PEER_CRASH_PHASE="+phase, "FULLA_PEER_CRASH_PUBLIC_RECORD="+string(public), "FULLA_PEER_CRASH_OLD_PIN="+original.Fingerprint)
					output, err := cmd.StdoutPipe()
					if err != nil {
						t.Fatal(err)
					}
					if err := cmd.Start(); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
					ready := make(chan bool, 1)
					go func() {
						scanner := bufio.NewScanner(output)
						ready <- scanner.Scan() && scanner.Text() == "peer-published"
					}()
					select {
					case ok := <-ready:
						if !ok {
							t.Fatal("peer child exited before publication")
						}
					case <-time.After(20 * time.Second):
						t.Fatal("peer publication not reached")
					}
					lock, err := s.InspectLock()
					if err != nil || lock == nil || !lock.Alive {
						t.Fatal("missing live owner", err)
					}
					if _, err := s.Recover(lock.Token); err == nil {
						t.Fatal("stole live peer writer")
					}
					if err := cmd.Process.Kill(); err != nil {
						t.Fatal(err)
					}
					_ = cmd.Wait()
					if _, err := s.Read("entry"); err == nil {
						t.Fatal("read through dead owner's lock")
					}
					if _, err := s.Recover("wrong-owner"); err == nil {
						t.Fatal("accepted wrong owner")
					}
					result, err := s.Recover(lock.Token)
					if err != nil || result["lock_released"] != true || result["recovered"] != (operation == "rotate" || operation == "remove") {
						t.Fatal("unexpected no-journal lock recovery", result, err)
					}
					current, err := s.Peer(next.Name)
					if phase == "prepared" {
						if err != nil || current.Host != original.Host || current.Fingerprint != original.Fingerprint {
							t.Fatal("changed unpublished peer", err)
						}
					} else if operation == "remove" {
						var problem *fault.Error
						if !errors.As(err, &problem) || problem.Code != "peer.not_found" {
							t.Fatal("removal was not preserved", err)
						}
					} else if err != nil || current.Host != next.Host || current.Fingerprint != next.Fingerprint {
						t.Fatal("lost published peer", err)
					}
					if current.Activated != (operation == "activate") {
						t.Fatal("wrong activation state")
					}
					if operation == "dry-run" || operation == "activate" {
						if current.DryRunIdentity != identity.Fingerprint {
							t.Fatal("lost dry-run identity")
						}
					}
					got, err := s.Read("entry")
					if err != nil || !bytes.Equal(got, value) {
						t.Fatal("lost entry bytes", err)
					}
					afterCiphertext, err := securefs.Read(s.Root, "passwords/entry.age", maxMetadata)
					if err != nil || !bytes.Equal(ciphertext, afterCiphertext) {
						t.Fatal("rewrote entry ciphertext", err)
					}
					afterKeys, err := securefs.Read(s.Root, "identities", maxMetadata)
					if err != nil || !bytes.Equal(keys, afterKeys) {
						t.Fatal("changed identity", err)
					}
					if operation == "rotate" || operation == "legacy-rotate" || operation == "remove" {
						entries, err := os.ReadDir(filepath.Join(s.Dir, metadata, "receipts"))
						if err != nil {
							t.Fatal(err)
						}
						found := false
						for _, entry := range entries {
							data, err := os.ReadFile(filepath.Join(s.Dir, metadata, "receipts", entry.Name()))
							if err != nil {
								t.Fatal(err)
							}
							var receipt struct {
								Command     string
								Phase       string
								Previous    Peer
								Replacement Peer
							}
							if err := json.Unmarshal(data, &receipt); err != nil {
								t.Fatal(err)
							}
							command := "peer rotate"
							if operation == "remove" {
								command = "peer remove"
							}
							if receipt.Command != command {
								continue
							}
							found = true
							expectedPhase := "applied"
							if phase == "prepared" {
								expectedPhase = "aborted"
							}
							if operation == "legacy-rotate" {
								expectedPhase = "prepared"
							}
							if receipt.Phase != expectedPhase || receipt.Previous.Host != original.Host || receipt.Previous.Fingerprint != original.Fingerprint {
								t.Fatal("lost reconciled peer evidence")
							}
							if operation == "remove" {
								if receipt.Replacement.Name != "" {
									t.Fatal("invented removal replacement")
								}
							} else if receipt.Replacement.Host != next.Host || receipt.Replacement.Fingerprint != next.Fingerprint {
								t.Fatal("lost replacement evidence")
							}
						}
						if !found {
							t.Fatal("missing prepared rotation receipt")
						}
					}
				})
			}
		}
	}
}
