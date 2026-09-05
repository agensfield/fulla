package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestPeerReceiptReconciliation(t *testing.T) {
	for _, state := range []string{"old", "new", "unexpected", "contradictory"} {
		t.Run(state, func(t *testing.T) {
			s := fixture(t, false)
			identity, err := s.IdentityShow()
			if err != nil {
				t.Fatal(err)
			}
			p := Peer{Version: 1, Name: "fixture", Host: "old", Recipient: identity.Recipient, Fingerprint: identity.Fingerprint}
			if err := s.SavePeer(p, false, ""); err != nil {
				t.Fatal(err)
			}
			previous, err := s.Peer(p.Name)
			if err != nil {
				t.Fatal(err)
			}
			next := previous
			next.Host = "new"
			current := previous
			if state == "new" {
				current = next
			}
			if state == "unexpected" {
				current.Host = "unexplained"
			}
			live, err := json.Marshal(current)
			if err != nil {
				t.Fatal(err)
			}
			if err := securefs.Replace(s.Root, metadata+"/peers/fixture.json", live); err != nil {
				t.Fatal(err)
			}
			id := securefs.ID()
			name := metadata + "/receipts/" + id + ".json"
			phase := "prepared"
			if state == "contradictory" {
				phase = "applied"
			}
			data, err := json.Marshal(map[string]any{"version": 1, "command": "peer rotate", "previous": previous, "replacement": next, "phase": phase})
			if err != nil {
				t.Fatal(err)
			}
			if err := securefs.PublishNew(s.Root, name, data); err != nil {
				t.Fatal(err)
			}
			lock, err := s.Lock("fixture receipt reconciliation")
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Release()
			err = s.reconcilePeerReceipt(id)
			after, readErr := securefs.Read(s.Root, name, maxMetadata)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if state == "unexpected" || state == "contradictory" {
				var problem *fault.Error
				if !errors.As(err, &problem) || problem.Code != "peer.recovery_mismatch" || !bytes.Equal(data, after) {
					t.Fatal("guessed ambiguous recovery", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				var receipt struct{ Phase string }
				if err := json.Unmarshal(after, &receipt); err != nil {
					t.Fatal(err)
				}
				expected := "aborted"
				if state == "new" {
					expected = "applied"
				}
				if receipt.Phase != expected {
					t.Fatal("wrong reconciled phase", receipt.Phase)
				}
				if err := s.reconcilePeerReceipt(id); err != nil {
					t.Fatal("retry failed", err)
				}
				repeated, err := securefs.Read(s.Root, name, maxMetadata)
				if err != nil || !bytes.Equal(after, repeated) {
					t.Fatal("retry rewrote finalized receipt", err)
				}
			}
			afterLive, err := securefs.Read(s.Root, metadata+"/peers/fixture.json", maxMetadata)
			if err != nil || !bytes.Equal(live, afterLive) {
				t.Fatal("receipt recovery changed live authorization", err)
			}
		})
	}
}

func TestPeerReceiptBindingSurvivesRecoveryTakeover(t *testing.T) {
	s := fixture(t, false)
	identity, err := s.IdentityShow()
	if err != nil {
		t.Fatal(err)
	}
	p := Peer{Version: 1, Name: "fixture", Host: "old", Recipient: identity.Recipient, Fingerprint: identity.Fingerprint}
	if err := s.SavePeer(p, false, ""); err != nil {
		t.Fatal(err)
	}
	next := p
	next.Host = "new"
	start := func(operation string, token string) *exec.Cmd {
		public, err := json.Marshal(next)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestPeerCrashHelper$")
		cmd.Env = append(os.Environ(), "FULLA_PEER_CRASH_FIXTURE="+s.Dir, "FULLA_PEER_CRASH_OPERATION="+operation, "FULLA_PEER_CRASH_PUBLIC_RECORD="+string(public), "FULLA_PEER_CRASH_OLD_PIN="+token)
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
				t.Fatal("fixture did not pause")
			}
		case <-time.After(20 * time.Second):
			t.Fatal("fixture timed out")
		}
		return cmd
	}
	writer := start("rotate", p.Fingerprint)
	owner, err := s.InspectLock()
	if err != nil || owner == nil || owner.PeerReceipt == "" {
		t.Fatal("missing receipt binding", err)
	}
	if err := writer.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = writer.Wait()
	current, err := securefs.Read(s.Root, metadata+"/peers/fixture.json", maxMetadata)
	if err != nil {
		t.Fatal(err)
	}
	var unexpected Peer
	if err := json.Unmarshal(current, &unexpected); err != nil {
		t.Fatal(err)
	}
	unexpected.Host = "unexplained"
	data, err := json.Marshal(unexpected)
	if err != nil {
		t.Fatal(err)
	}
	if err := securefs.Replace(s.Root, metadata+"/peers/fixture.json", data); err != nil {
		t.Fatal(err)
	}
	recovering := start("recover", owner.Token)
	takeover, err := s.InspectLock()
	if err != nil || takeover == nil || !takeover.Alive || takeover.Token == owner.Token || takeover.PeerReceipt != owner.PeerReceipt {
		t.Fatal("lost binding during takeover", err)
	}
	if err := recovering.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = recovering.Wait()
	// Repair only the fixture's deliberately injected unexplained state, then
	// retry with the newly inspected dead owner's token.
	if err := securefs.Replace(s.Root, metadata+"/peers/fixture.json", current); err != nil {
		t.Fatal(err)
	}
	result, err := s.Recover(takeover.Token)
	if err != nil || result["recovered"] != true || result["lock_released"] != true {
		t.Fatal("takeover retry failed", result, err)
	}
	receipt, err := securefs.Read(s.Root, metadata+"/receipts/"+owner.PeerReceipt+".json", maxMetadata)
	if err != nil {
		t.Fatal(err)
	}
	var phase struct{ Phase string }
	if err := json.Unmarshal(receipt, &phase); err != nil || phase.Phase != "applied" {
		t.Fatal("receipt not reconciled", err)
	}
}

func TestPeerRemovalReceiptReconciliation(t *testing.T) {
	for _, state := range []string{"old", "removed", "unexpected", "missing-registry"} {
		t.Run(state, func(t *testing.T) {
			s := fixture(t, false)
			identity, err := s.IdentityShow()
			if err != nil {
				t.Fatal(err)
			}
			p := Peer{Version: 1, Name: "fixture", Host: "old", Recipient: identity.Recipient, Fingerprint: identity.Fingerprint}
			if err := s.SavePeer(p, false, ""); err != nil {
				t.Fatal(err)
			}
			previous, err := s.Peer(p.Name)
			if err != nil {
				t.Fatal(err)
			}
			name := metadata + "/peers/fixture.json"
			before, err := securefs.Read(s.Root, name, maxMetadata)
			if err != nil {
				t.Fatal(err)
			}
			if state == "removed" {
				if err := s.Root.Remove(name); err != nil {
					t.Fatal(err)
				}
			}
			if state == "unexpected" {
				current := previous
				current.Host = "unexpected"
				before, err = json.Marshal(current)
				if err != nil {
					t.Fatal(err)
				}
				if err := securefs.Replace(s.Root, name, before); err != nil {
					t.Fatal(err)
				}
			}
			if state == "missing-registry" {
				if err := s.Root.Rename(metadata+"/peers", metadata+"/peers-fixture-away"); err != nil {
					t.Fatal(err)
				}
			}
			id := securefs.ID()
			receiptPath := metadata + "/receipts/" + id + ".json"
			data, err := json.Marshal(map[string]any{"version": 1, "command": "peer remove", "previous": previous, "phase": "prepared"})
			if err != nil {
				t.Fatal(err)
			}
			if err := securefs.PublishNew(s.Root, receiptPath, data); err != nil {
				t.Fatal(err)
			}
			lock, err := s.Lock("fixture removal receipt")
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Release()
			err = s.reconcilePeerReceipt(id)
			after, readErr := securefs.Read(s.Root, receiptPath, maxMetadata)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if state == "unexpected" || state == "missing-registry" {
				var problem *fault.Error
				expected := "peer.recovery_mismatch"
				if state == "missing-registry" {
					expected = "peer.recovery_invalid"
				}
				if !errors.As(err, &problem) || problem.Code != expected || !bytes.Equal(data, after) {
					t.Fatal("guessed removal", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				var receipt struct {
					Phase       string
					Replacement *Peer
				}
				if err := json.Unmarshal(after, &receipt); err != nil {
					t.Fatal(err)
				}
				phase := "aborted"
				if state == "removed" {
					phase = "applied"
				}
				if receipt.Phase != phase || receipt.Replacement != nil {
					t.Fatal("wrong removal receipt")
				}
				if err := s.reconcilePeerReceipt(id); err != nil {
					t.Fatal("retry failed", err)
				}
			}
			if state == "removed" {
				if _, err := s.Root.Lstat(name); !os.IsNotExist(err) {
					t.Fatal("recreated removed pin", err)
				}
			} else {
				if state == "missing-registry" {
					name = metadata + "/peers-fixture-away/fixture.json"
				}
				current, err := securefs.Read(s.Root, name, maxMetadata)
				if err != nil || !bytes.Equal(before, current) {
					t.Fatal("changed authorization", err)
				}
			}
		})
	}
}
