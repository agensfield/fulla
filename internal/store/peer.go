package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"time"
	"unicode"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

type Peer struct {
	Version        int      `json:"version"`
	Name           string   `json:"name"`
	Host           string   `json:"host"`
	Store          string   `json:"store"`
	Binary         string   `json:"binary"`
	SSHOptions     []string `json:"ssh_options,omitempty"`
	Recipient      string   `json:"recipient"`
	Fingerprint    string   `json:"fingerprint"`
	Enrolled       string   `json:"enrolled"`
	DryRunIdentity string   `json:"dry_run_identity"`
	Activated      bool     `json:"activated"`
}

func PeerName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, c := range name {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' {
			continue
		}
		return false
	}
	return true
}

func ValidatePeer(p Peer) error {
	if p.Version != 1 {
		return fault.New("peer.unsupported", "unsupported peer metadata version")
	}
	if len(p.SSHOptions) > 32 {
		return fault.Usage("too many SSH options")
	}
	allowed := map[string]bool{"Port": true, "User": true, "HostName": true, "IdentityFile": true, "IdentitiesOnly": true, "UserKnownHostsFile": true, "GlobalKnownHostsFile": true, "StrictHostKeyChecking": true, "ProxyJump": true, "ConnectTimeout": true, "ServerAliveInterval": true, "ServerAliveCountMax": true, "Compression": true}
	for _, option := range p.SSHOptions {
		key, value, ok := strings.Cut(option, "=")
		if !ok || !allowed[key] || value == "" || len(value) > 4096 {
			return fault.Usage("invalid SSH transport option; use a supported NAME=VALUE setting")
		}
		for _, c := range value {
			if unicode.IsControl(c) {
				return fault.Usage("SSH options cannot contain control characters")
			}
		}
	}
	if !PeerName(p.Name) {
		return fault.Usage("peer name must use letters, digits, underscores, or hyphens")
	}
	if p.Host == "" || strings.HasPrefix(p.Host, "-") {
		return fault.Usage("invalid SSH host")
	}
	for _, v := range []string{p.Host, p.Store, p.Binary} {
		for _, c := range v {
			if unicode.IsControl(c) {
				return fault.Usage("peer transport fields cannot contain control characters")
			}
		}
	}
	for _, c := range p.Host {
		if unicode.IsSpace(c) {
			return fault.Usage("SSH host cannot contain whitespace")
		}
	}
	if p.Store != "" && !strings.HasPrefix(p.Store, "/") {
		return fault.Usage("remote store path must be absolute")
	}
	if p.Binary != "" && !strings.HasPrefix(p.Binary, "/") && p.Binary != "fulla" {
		return fault.Usage("remote binary must be fulla or an absolute path")
	}
	if p.Fingerprint != crypt.Fingerprint(p.Recipient) {
		return fault.New("peer.trust_mismatch", "peer fingerprint does not match its recipient")
	}
	if _, err := crypt.Recipients([]byte(p.Recipient), nil); err != nil {
		return err
	}
	return nil
}

func (s *Store) Peer(name string) (Peer, error) {
	var p Peer
	if err := s.RequireDomain("peers"); err != nil {
		return p, err
	}
	if !PeerName(name) {
		return p, fault.Usage("invalid peer name")
	}
	data, err := securefs.Read(s.Root, metadata+"/peers/"+name+".json", maxMetadata)
	if errors.Is(err, fs.ErrNotExist) {
		return p, fault.New("peer.not_found", "peer does not exist")
	}
	if err != nil {
		return p, err
	}
	if err := StrictJSON(data, &p); err != nil {
		return p, err
	}
	if p.Name != name {
		return p, fault.New("peer.invalid", "peer record name mismatch")
	}
	return p, ValidatePeer(p)
}

func (s *Store) Peers() ([]Peer, error) {
	if err := s.RequireDomain("peers"); err != nil {
		return nil, err
	}
	r, err := s.Root.OpenRoot(metadata + "/peers")
	if err != nil {
		return nil, err
	}
	defer r.Close()
	entries, err := fs.ReadDir(r.FS(), ".")
	if err != nil {
		return nil, err
	}
	peers := []Peer{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			return nil, fault.New("peer.invalid", "unexpected peer registry file")
		}
		p, err := s.Peer(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		peers = append(peers, p)
	}
	return peers, nil
}

func (s *Store) SavePeer(p Peer, replace bool, expected string) (err error) {
	if err := s.RequireDomain("peers"); err != nil {
		return err
	}
	if err := ValidatePeer(p); err != nil {
		return err
	}
	lock, err := s.Lock("peer enroll")
	if err != nil {
		return err
	}
	applied := false
	defer func() {
		if e := lock.Release(); e != nil && err == nil {
			if applied {
				err = fault.Applied("peer saved but lock release failed", p.Name)
			} else {
				err = e
			}
		}
	}()
	previous, e := s.Peer(p.Name)
	var fe *fault.Error
	missing := errors.As(e, &fe) && fe.Code == "peer.not_found"
	if e != nil && !missing {
		return e
	}
	if replace {
		if missing {
			return fault.New("peer.not_found", "peer does not exist")
		}
		if previous.Fingerprint != expected {
			return fault.New("peer.trust_mismatch", "peer record changed before replacement")
		}
	} else if !missing {
		return fault.New("peer.exists", "peer already exists; use peer rotate")
	}
	p.Enrolled = time.Now().UTC().Format(time.RFC3339Nano)
	p.DryRunIdentity = ""
	p.Activated = false
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	file := metadata + "/peers/" + p.Name + ".json"
	// Persist old public trust evidence before changing the live pin.
	if replace {
		receipt, _ := json.Marshal(map[string]any{"version": 1, "command": "peer rotate", "previous": previous, "replacement": p, "phase": "prepared"})
		if err := securefs.PublishNew(s.Root, metadata+"/receipts/"+securefs.ID()+".json", receipt); err != nil {
			return err
		}
		err = securefs.Replace(s.Root, file, data)
	} else {
		err = securefs.PublishNew(s.Root, file, data)
	}
	if err != nil {
		return err
	}
	applied = true
	return nil
}

func (s *Store) RemovePeer(name string) error { return s.RemovePeerConfirmed(name, nil) }

// RemovePeerConfirmed holds the shared lock while confirming the current pin.
func (s *Store) RemovePeerConfirmed(name string, confirm func(Peer) error) (err error) {
	p, err := s.Peer(name)
	if err != nil {
		return err
	}
	lock, err := s.Lock("peer remove")
	if err != nil {
		return err
	}
	applied := false
	defer func() {
		if releaseErr := lock.Release(); releaseErr != nil && err == nil {
			if applied {
				err = fault.Applied("peer removed but shared lock release failed", name)
			} else {
				err = releaseErr
			}
		}
	}()
	again, err := s.Peer(name)
	if err != nil {
		return err
	}
	if again.Fingerprint != p.Fingerprint {
		return fault.New("peer.trust_mismatch", "peer changed during removal")
	}
	if confirm != nil {
		if err := confirm(again); err != nil {
			return err
		}
	}
	data, _ := json.Marshal(map[string]any{"version": 1, "command": "peer remove", "previous": again, "phase": "prepared"})
	receipt := metadata + "/receipts/" + securefs.ID() + ".json"
	if err := securefs.PublishNew(s.Root, receipt, data); err != nil {
		return err
	}
	if err := s.Root.Remove(metadata + "/peers/" + name + ".json"); err != nil {
		return err
	}
	applied = true
	if err := securefs.SyncDir(s.Root, metadata+"/peers"); err != nil {
		return fault.Applied("peer removed but directory synchronization failed", name)
	}
	data, _ = json.Marshal(map[string]any{"version": 1, "command": "peer remove", "previous": again, "phase": "applied"})
	if err := securefs.Replace(s.Root, receipt, data); err != nil {
		return fault.Applied("peer removed but receipt finalization failed", name)
	}
	return nil
}

func (s *Store) MarkPeer(name, expected, localIdentity string, activated bool) error {
	return s.markPeer(name, expected, localIdentity, activated, nil)
}

// afterPublish is an internal failure-injection seam, never user-controlled.
func (s *Store) markPeer(name, expected, localIdentity string, activated bool, afterPublish func() error) (err error) {
	if err := s.RequireDomain("sync"); err != nil {
		return err
	}
	lock, err := s.Lock("peer sync-state")
	if err != nil {
		return err
	}
	applied := false
	defer func() {
		if releaseErr := lock.Release(); releaseErr != nil && err == nil {
			if applied {
				err = fault.Applied("peer sync state published but lock release failed", name)
			} else {
				err = releaseErr
			}
		}
	}()
	if err := s.RequireDomain("sync"); err != nil {
		return err
	}
	p, err := s.Peer(name)
	if err != nil {
		return err
	}
	if p.Fingerprint != expected {
		return fault.New("peer.trust_mismatch", "peer changed during synchronization")
	}
	p.DryRunIdentity = localIdentity
	p.Activated = p.Activated || activated
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	applied, err = securefs.ReplacePublished(s.Root, metadata+"/peers/"+name+".json", data)
	if err == nil && afterPublish != nil {
		err = afterPublish()
	}
	if err != nil && applied {
		return fault.Applied("peer sync state published but finalization failed", name)
	}
	return err
}
