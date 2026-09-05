package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agensfield/fulla/internal/securefs"
	"os"
	"path/filepath"
	"testing"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
)

func TestPeerSSHOptionsRejectExecutableAndMalformedSettings(t *testing.T) {
	_, recipient, err := crypt.Generate()
	if err != nil {
		t.Fatal(err)
	}
	p := Peer{Version: 1, Name: "fixture", Host: "fixture@127.0.0.1", Recipient: recipient, Fingerprint: crypt.Fingerprint(recipient)}
	p.SSHOptions = []string{"Port=40222", "IdentityFile=/private/fixture key", "UserKnownHostsFile=/private/known-hosts", "StrictHostKeyChecking=yes", "IdentitiesOnly=yes"}
	if err := ValidatePeer(p); err != nil {
		t.Fatal(err)
	}
	for _, option := range []string{"ProxyCommand=touch /tmp/unwanted", "LocalCommand=echo unwanted", "PermitLocalCommand=yes", "Include=/tmp/config", "BatchMode=no", "Port", "Port=", "Port=22\nLocalCommand=echo unwanted", "IdentityFile=key\x00file"} {
		t.Run(option, func(t *testing.T) {
			p.SSHOptions = []string{option}
			if err := ValidatePeer(p); err == nil {
				t.Fatal("accepted unsafe or malformed option")
			}
		})
	}
	p.SSHOptions = make([]string, 33)
	for i := range p.SSHOptions {
		p.SSHOptions[i] = "Port=22"
	}
	if err := ValidatePeer(p); err == nil {
		t.Fatal("accepted unbounded options")
	}
}

func TestPeerRemovalConfirmationHoldsCurrentPin(t *testing.T) {
	s := fixture(t, false)
	_, recipient, err := crypt.Generate()
	if err != nil {
		t.Fatal(err)
	}
	peer := Peer{Version: 1, Name: "fixture", Host: "fixture", Recipient: recipient, Fingerprint: crypt.Fingerprint(recipient)}
	if err := s.SavePeer(peer, false, ""); err != nil {
		t.Fatal(err)
	}
	before, err := securefs.Read(s.Root, metadata+"/peers/fixture.json", maxMetadata)
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := os.ReadDir(filepath.Join(s.Dir, metadata, "receipts"))
	if err != nil {
		t.Fatal(err)
	}
	cancelled := errors.New("fixture cancellation")
	err = s.RemovePeerConfirmed("fixture", func(current Peer) error {
		if current.Fingerprint != peer.Fingerprint || current.Host != peer.Host {
			t.Fatal("wrong confirmation metadata")
		}
		if lock, err := s.Lock("competing fixture"); err == nil {
			lock.Release()
			t.Fatal("confirmation did not hold shared lock")
		}
		return cancelled
	})
	if !errors.Is(err, cancelled) {
		t.Fatal("lost cancellation", err)
	}
	after, err := securefs.Read(s.Root, metadata+"/peers/fixture.json", maxMetadata)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("cancel changed peer", err)
	}
	afterReceipts, err := os.ReadDir(filepath.Join(s.Dir, metadata, "receipts"))
	if err != nil || len(receipts) != len(afterReceipts) {
		t.Fatal("cancel wrote removal receipt", err)
	}
	if err := s.RemovePeerConfirmed("fixture", func(Peer) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, metadata, "peers", "fixture.json")); !os.IsNotExist(err) {
		t.Fatal("peer not removed")
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "lock")); !os.IsNotExist(err) {
		t.Fatal("retained peer removal lock")
	}
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
			Command  string `json:"command"`
			Phase    string `json:"phase"`
			Previous Peer   `json:"previous"`
		}
		if err := json.Unmarshal(data, &receipt); err != nil {
			t.Fatal(err)
		}
		if receipt.Command == "peer remove" {
			if receipt.Phase != "applied" || receipt.Previous.Fingerprint != peer.Fingerprint {
				t.Fatal("inaccurate removal receipt")
			}
			found = true
		}
	}
	if !found {
		t.Fatal("missing applied removal receipt")
	}

}

func TestPeerRemovalReportsAppliedOnReleaseFailure(t *testing.T) {
	s := fixture(t, false)
	_, recipient, err := crypt.Generate()
	if err != nil {
		t.Fatal(err)
	}
	peer := Peer{Version: 1, Name: "fixture", Host: "fixture", Recipient: recipient, Fingerprint: crypt.Fingerprint(recipient)}
	if err := s.SavePeer(peer, false, ""); err != nil {
		t.Fatal(err)
	}
	err = s.RemovePeerConfirmed("fixture", func(Peer) error {
		// Simulate ownership changing before release in this disposable store only.
		return securefs.Replace(s.Root, "lock/owner", []byte("different-fixture-owner\n"))
	})
	var problem *fault.Error
	if !errors.As(err, &problem) || problem.Status != 3 || problem.Details["applied"] != true {
		t.Fatal("lost applied state on release failure", err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, metadata, "peers", "fixture.json")); !os.IsNotExist(err) {
		t.Fatal("fixture did not remove peer")
	}
}

func TestPeerSyncMarkReportsAppliedFailures(t *testing.T) {
	for _, failure := range []string{"finalization", "lock-release"} {
		for _, activated := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/activated=%v", failure, activated), func(t *testing.T) {
				s := fixture(t, false)
				identity, err := s.IdentityShow()
				if err != nil {
					t.Fatal(err)
				}
				p := Peer{Version: 1, Name: "fixture", Host: "fixture", Recipient: identity.Recipient, Fingerprint: identity.Fingerprint}
				if err := s.SavePeer(p, false, ""); err != nil {
					t.Fatal(err)
				}
				err = s.markPeer(p.Name, p.Fingerprint, identity.Fingerprint, activated, func() error {
					if failure == "lock-release" {
						return securefs.Replace(s.Root, "lock/owner", []byte("different-fixture-owner\n"))
					}
					return errors.New("fixture finalization failure")
				})
				var problem *fault.Error
				if !errors.As(err, &problem) || problem.Status != 3 || problem.Details["applied"] != true {
					t.Fatal("lost applied sync mark", err)
				}
				current, err := s.Peer(p.Name)
				if err != nil || current.DryRunIdentity != identity.Fingerprint || current.Activated != activated {
					t.Fatal("wrong published mark", err)
				}
				_, err = os.Stat(filepath.Join(s.Dir, "lock"))
				if failure == "lock-release" {
					if err != nil {
						t.Fatal("removed changed-owner lock", err)
					}
				} else if !os.IsNotExist(err) {
					t.Fatal("retained owned lock", err)
				}
			})
		}
	}
}
