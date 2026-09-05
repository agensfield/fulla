package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/agensfield/fulla/internal/crypt"
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
	case "enroll", "rotate":
		err = s.savePeer(p, operation == "rotate", os.Getenv("FULLA_PEER_CRASH_OLD_PIN"), pause)
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
		for _, operation := range []string{"enroll", "rotate", "dry-run", "activate"} {
			t.Run(fmt.Sprintf("git=%t/%s", git, operation), func(t *testing.T) {
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
				if operation == "rotate" {
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
				cmd.Env = append(os.Environ(), "FULLA_PEER_CRASH_FIXTURE="+s.Dir, "FULLA_PEER_CRASH_OPERATION="+operation, "FULLA_PEER_CRASH_PUBLIC_RECORD="+string(public), "FULLA_PEER_CRASH_OLD_PIN="+original.Fingerprint)
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
				if err != nil || result["lock_released"] != true || result["recovered"] != false {
					t.Fatal("unexpected no-journal lock recovery", result, err)
				}
				current, err := s.Peer(next.Name)
				if err != nil || current.Host != next.Host || current.Fingerprint != next.Fingerprint {
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
				if operation == "rotate" {
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
						if receipt.Command != "peer rotate" {
							continue
						}
						found = true
						if receipt.Phase != "prepared" || receipt.Previous.Host != original.Host || receipt.Replacement.Host != next.Host || receipt.Previous.Fingerprint != original.Fingerprint || receipt.Replacement.Fingerprint != next.Fingerprint {
							t.Fatal("lost unresolved rotation evidence")
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
