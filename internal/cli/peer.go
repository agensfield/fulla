package cli

import (
	"context"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/remote"
	"github.com/agensfield/fulla/internal/store"
)

func (a *App) peers(p invocation, s *store.Store) (any, error) {
	if p.Command == "peer list" {
		if len(p.Args) != 0 {
			return nil, fault.Usage("peer list takes no positional arguments")
		}
		peers, err := s.Peers()
		return map[string]any{"peers": peers}, err
	}
	if len(p.Args) != 1 || !store.PeerName(p.Args[0]) {
		return nil, fault.Usage(p.Command + " requires one valid peer name")
	}
	name := p.Args[0]
	if p.Command == "peer show" {
		return s.Peer(name)
	}
	if p.Command == "peer remove" {
		if !p.has("yes") {
			return nil, fault.Interaction("peer removal requires --yes")
		}
		err := s.RemovePeer(name)
		return map[string]any{"name": name, "removed": err == nil}, err
	}
	if p.Command == "sync" {
		peer, err := s.Peer(name)
		if err != nil {
			return nil, err
		}
		conn, err := remote.Dial(context.Background(), peer)
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		client := &remote.Client{Conn: conn}
		return remote.Sync(s, peer, client, p.has("dry-run"), p.has("fail-on-skip"))
	}
	peer := store.Peer{Version: 1, Name: name, Host: p.value("host"), Store: p.value("remote-store"), Binary: p.value("remote-binary")}
	expectedOld := ""
	if p.Command == "peer rotate" {
		previous, err := s.Peer(name)
		if err != nil {
			return nil, err
		}
		expectedOld = previous.Fingerprint
		if peer.Host == "" {
			peer.Host = previous.Host
		}
		if !p.has("remote-store") {
			peer.Store = previous.Store
		}
		if !p.has("remote-binary") {
			peer.Binary = previous.Binary
		}
	}
	identity, err := s.IdentityShow()
	if err != nil {
		return nil, err
	}
	// The transport validates endpoint fields before connecting. Until discovery
	// returns the remote public identity, this provisional record is never saved.
	peer.Recipient = identity.Recipient
	peer.Fingerprint = identity.Fingerprint
	conn, err := remote.Dial(context.Background(), peer)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	client := &remote.Client{Conn: conn}
	discovered, err := client.Hello()
	if err != nil {
		return nil, err
	}
	if err := client.Close(); err != nil {
		return nil, err
	}
	if p.value("expect-fingerprint") == "" {
		e := fault.Interaction("independently verify the remote identity, then repeat with --expect-fingerprint")
		e.Details["observed_fingerprint"] = discovered.Fingerprint
		e.Details["remote_verification_argv"] = []string{"fulla", "--store", peer.Store, "identity", "show"}
		return nil, e
	}
	if p.value("expect-fingerprint") != discovered.Fingerprint {
		return nil, fault.New("peer.trust_mismatch", "discovered identity does not match independently expected fingerprint")
	}
	peer.Recipient = discovered.Recipient
	peer.Fingerprint = discovered.Fingerprint
	if err := s.SavePeer(peer, p.Command == "peer rotate", expectedOld); err != nil {
		return nil, err
	}
	return s.Peer(name)
}
