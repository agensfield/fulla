package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestHandledPeerFailureRetainsRecoverableEvidence(t *testing.T) {
	for _, git := range []bool{false, true} {
		for _, operation := range []string{"rotate-error", "remove-error"} {
			t.Run(fmt.Sprintf("git=%t/%s", git, operation), func(t *testing.T) {
				s := fixture(t, git)
				value := []byte{0, 255, 10, 42}
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
				original := Peer{Version: 1, Name: "fixture", Host: "old", Recipient: identity.Recipient, Fingerprint: identity.Fingerprint}
				if err := s.SavePeer(original, false, ""); err != nil {
					t.Fatal(err)
				}
				next := original
				if operation == "rotate-error" {
					_, recipient, err := crypt.Generate()
					if err != nil {
						t.Fatal(err)
					}
					next.Recipient = recipient
					next.Fingerprint = crypt.Fingerprint(recipient)
				}
				public, err := json.Marshal(next)
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPeerCrashHelper$")
				cmd.Env = append(os.Environ(), "FULLA_PEER_CRASH_FIXTURE="+s.Dir, "FULLA_PEER_CRASH_OPERATION="+operation, "FULLA_PEER_CRASH_PUBLIC_RECORD="+string(public), "FULLA_PEER_CRASH_OLD_PIN="+original.Fingerprint)
				output, err := cmd.CombinedOutput()
				if err != nil || !bytes.Contains(output, []byte("peer-error-retained")) {
					t.Fatalf("handled-error child failed: %v %s", err, output)
				}
				owner, err := s.InspectLock()
				if err != nil || owner == nil || owner.Alive || owner.PeerReceipt == "" {
					t.Fatal("lost recovery binding after normal exit", err)
				}
				if _, err := s.Read("entry"); err == nil {
					t.Fatal("read bypassed incomplete operation")
				}
				receiptName := metadata + "/receipts/" + owner.PeerReceipt + ".json"
				data, err := securefs.Read(s.Root, receiptName, maxMetadata)
				if err != nil {
					t.Fatal(err)
				}
				var receipt struct{ Phase string }
				if err := json.Unmarshal(data, &receipt); err != nil || receipt.Phase != "prepared" {
					t.Fatal("missing prepared evidence", err)
				}
				result, err := s.Recover(owner.Token)
				if err != nil || result["recovered"] != true || result["lock_released"] != true {
					t.Fatal("normal-exit recovery failed", result, err)
				}
				data, err = securefs.Read(s.Root, receiptName, maxMetadata)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(data, &receipt); err != nil || receipt.Phase != "applied" {
					t.Fatal("receipt not finalized", err)
				}
				current, err := s.Peer(next.Name)
				if operation == "remove-error" {
					var problem *fault.Error
					if !errors.As(err, &problem) || problem.Code != "peer.not_found" {
						t.Fatal("recreated removed pin", err)
					}
				} else if err != nil || current.Fingerprint != next.Fingerprint {
					t.Fatal("changed published pin", err)
				}
				after, err := securefs.Read(s.Root, "passwords/entry.age", maxMetadata)
				if err != nil || !bytes.Equal(after, ciphertext) {
					t.Fatal("changed ciphertext", err)
				}
				plaintext, err := s.Read("entry")
				if err != nil || !bytes.Equal(plaintext, value) {
					t.Fatal("lost secret bytes", err)
				}
			})
		}
	}
}
