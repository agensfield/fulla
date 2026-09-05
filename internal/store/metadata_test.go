package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestOpenHandleRejectsUpgradedPeerDomain(t *testing.T) {
	s := fixture(t, false)
	_, recipient, err := crypt.Generate()
	if err != nil {
		t.Fatal(err)
	}
	peer := Peer{Version: 1, Name: "future", Host: "fixture", Recipient: recipient[:len(recipient)-1], Fingerprint: crypt.Fingerprint(recipient)}
	newer := s.Meta
	newer.Domains = map[string]int{}
	for domain, v := range s.Meta.Domains {
		newer.Domains[domain] = v
	}
	newer.Domains["peers"] = 2
	data, err := json.Marshal(newer)
	if err != nil {
		t.Fatal(err)
	}
	if err := securefs.Replace(s.Root, metadata+"/store.json", data); err != nil {
		t.Fatal(err)
	}
	err = s.SavePeer(peer, false, "")
	var problem *fault.Error
	if !errors.As(err, &problem) || problem.Code != "metadata.unsupported" {
		t.Fatal("open handle wrote a future peer domain", err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, metadata, "peers", "future.json")); !os.IsNotExist(err) {
		t.Fatal("published unsupported peer metadata")
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "lock")); !os.IsNotExist(err) {
		t.Fatal("retained lock")
	}
}

func TestPersistedDomainVersionMatrix(t *testing.T) {
	for _, domain := range []string{"peers", "sync", "backup", "identity", "transactions"} {
		for _, version := range []int{-1, 0, 1, 2} {
			t.Run(domain+"/"+strconv.Itoa(version), func(t *testing.T) {
				s := fixture(t, false)
				next := s.Meta
				next.Domains = map[string]int{}
				for name, v := range s.Meta.Domains {
					next.Domains[name] = v
				}
				if version == 0 {
					delete(next.Domains, domain)
				} else {
					next.Domains[domain] = version
				}
				data, err := json.Marshal(next)
				if err != nil {
					t.Fatal(err)
				}
				if err := securefs.Replace(s.Root, metadata+"/store.json", data); err != nil {
					t.Fatal(err)
				}
				err = s.RequireDomain(domain)
				if version == 1 {
					if err != nil {
						t.Fatal(err)
					}
				} else {
					var problem *fault.Error
					if !errors.As(err, &problem) || problem.Code != "metadata.unsupported" {
						t.Fatal("accepted unsupported domain", err)
					}
				}
				after, err := securefs.Read(s.Root, metadata+"/store.json", maxMetadata)
				if err != nil || !bytes.Equal(after, data) {
					t.Fatal("domain check rewrote metadata", err)
				}
				// A future feature-domain version does not block independent domains.
				other := "identity"
				if domain == other {
					other = "peers"
				}
				if err := s.RequireDomain(other); err != nil {
					t.Fatal("unrelated domain blocked", err)
				}
			})
		}
	}
}

func TestOpenHandleRejectsMalformedManifest(t *testing.T) {
	for name, change := range map[string]func([]byte) []byte{
		"duplicate":   func(data []byte) []byte { return append([]byte(`{"version":1,`), data[1:]...) },
		"trailing":    func(data []byte) []byte { return append(data, []byte(" {}")...) },
		"future-root": func(data []byte) []byte { return bytes.Replace(data, []byte(`"version":1`), []byte(`"version":2`), 1) },
	} {
		t.Run(name, func(t *testing.T) {
			s := fixture(t, false)
			data, err := securefs.Read(s.Root, metadata+"/store.json", maxMetadata)
			if err != nil {
				t.Fatal(err)
			}
			data = change(data)
			if err := securefs.Replace(s.Root, metadata+"/store.json", data); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Peers(); err == nil {
				t.Fatal("cached handle ignored changed manifest")
			}
			if opened, err := Open(s.Dir, true, nil); err == nil {
				opened.Close()
				t.Fatal("fresh handle accepted malformed manifest")
			}
		})
	}
}

func TestSyncDomainRevalidatedUnderSessionLock(t *testing.T) {
	s := fixture(t, false)
	identity, err := s.IdentityShow()
	if err != nil {
		t.Fatal(err)
	}
	peer := Peer{Version: 1, Name: "fixture", Host: "fixture", Recipient: identity.Recipient, Fingerprint: identity.Fingerprint}
	if err := s.SavePeer(peer, false, ""); err != nil {
		t.Fatal(err)
	}
	before, err := securefs.Read(s.Root, metadata+"/peers/fixture.json", maxMetadata)
	if err != nil {
		t.Fatal(err)
	}
	s.ExpectedPeerName = peer.Name
	s.ExpectedPeerFingerprint = peer.Fingerprint
	newer := s.Meta
	newer.Domains = map[string]int{}
	for domain, value := range s.Meta.Domains {
		newer.Domains[domain] = value
	}
	newer.Domains["sync"] = 2
	data, err := json.Marshal(newer)
	if err != nil {
		t.Fatal(err)
	}
	// Model an upgrade after the session's earlier domain check, before the
	// lock's revalidation. The callback runs while the shared lock is held.
	lock, err := s.lock("fixture sync", func() error {
		return securefs.Replace(s.Root, metadata+"/store.json", data)
	})
	if lock != nil {
		_ = lock.Release()
		t.Fatal("lock accepted upgraded sync domain")
	}
	var problem *fault.Error
	if !errors.As(err, &problem) || problem.Code != "metadata.unsupported" {
		t.Fatal(err)
	}
	// MarkPeer must enforce its own domain even outside a remote session.
	s.ExpectedPeerName = ""
	s.ExpectedPeerFingerprint = ""
	for _, activated := range []bool{false, true} {
		err = s.MarkPeer(peer.Name, peer.Fingerprint, identity.Fingerprint, activated)
		if !errors.As(err, &problem) || problem.Code != "metadata.unsupported" {
			t.Fatal("marked future sync domain", err)
		}
	}
	after, err := securefs.Read(s.Root, metadata+"/peers/fixture.json", maxMetadata)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("changed peer sync state", err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "lock")); !os.IsNotExist(err) {
		t.Fatal("retained lock", err)
	}
}
