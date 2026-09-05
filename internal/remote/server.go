package remote

import (
	"bytes"
	"io"
	"time"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/store"
)

func Serve(s *store.Store, input io.Reader, output io.Writer) error {
	err := serve(s, input, output)
	if err != nil {
		_ = writeFailure(output, err)
	}
	return err
}

func serve(s *store.Store, input io.Reader, output io.Writer) error {
	request, err := readMessage(input)
	if err != nil {
		return err
	}
	if request.Operation != "hello" {
		return fault.New("protocol.invalid", "session must begin with hello")
	}
	if request.Version < PreviousProtocol || request.Version > CurrentProtocol {
		return fault.New("peer.protocol_unsupported", "unsupported peer protocol; upgrade Fulla")
	}
	identity, err := s.IdentityShow()
	if err != nil {
		return err
	}
	if err := writeMessage(output, message{Operation: "hello", Version: request.Version, Recipient: identity.Recipient, Fingerprint: identity.Fingerprint}); err != nil {
		return err
	}
	auth, err := readMessage(input)
	if err != nil {
		return err
	}
	if auth.Operation == "bye" {
		return nil
	}
	if auth.Operation != "authenticate" || auth.Expected != identity.Fingerprint || auth.Version != request.Version {
		return fault.New("peer.trust_mismatch", "authentication identity or protocol changed")
	}
	peers, err := s.Peers()
	if err != nil {
		return err
	}
	var peer *store.Peer
	for _, candidate := range peers {
		if candidate.Fingerprint == auth.Fingerprint {
			copy := candidate
			peer = &copy
			break
		}
	}
	if peer == nil {
		return fault.New("peer.unauthorized", "SSH access alone does not authorize Fulla; mutually enroll this identity")
	}
	rs, err := crypt.Recipients([]byte(peer.Recipient), s.UI)
	if err != nil {
		return err
	}
	expected, err := newChallenge(request.Version, identity.Fingerprint, peer.Fingerprint)
	if err != nil {
		return err
	}
	encrypted, err := crypt.Encrypt(expected, rs)
	if err != nil {
		return err
	}
	started := time.Now()
	if err := writeMessage(output, message{Operation: "challenge", Challenge: encrypted}); err != nil {
		return err
	}
	proof, err := readMessage(input)
	if err != nil {
		return err
	}
	if proof.Operation != "proof" {
		return fault.New("peer.authentication_failed", "expected one-use session proof")
	}
	if err := checkProof(proof.Proof, expected, started); err != nil {
		return err
	}
	ids, _, err := s.Keys()
	if err != nil {
		return err
	}
	clientChallenge, err := crypt.Decrypt(proof.Challenge, ids)
	if err != nil {
		return err
	}
	if err := checkChallenge(clientChallenge, request.Version, peer.Fingerprint, identity.Fingerprint); err != nil {
		return err
	}
	if err := writeMessage(output, message{Operation: "authenticated", Proof: clientChallenge}); err != nil {
		return err
	}
	s.ExpectedFingerprint = identity.Fingerprint
	s.ExpectedPeerName = peer.Name
	s.ExpectedPeerFingerprint = peer.Fingerprint
	defer func() { s.ExpectedFingerprint = ""; s.ExpectedPeerName = ""; s.ExpectedPeerFingerprint = "" }()
	for {
		m, err := readMessage(input)
		if err != nil {
			return err
		}
		switch m.Operation {
		case "inventory":
			lock, err := s.Lock("sync inventory")
			if err != nil {
				return err
			}
			names, err := s.Names()
			releaseErr := lock.Release()
			if err != nil {
				return err
			}
			if releaseErr != nil {
				return releaseErr
			}
			if err := writeMessage(output, message{Operation: "inventory", Names: names}); err != nil {
				return err
			}
		case "dry-run":
			if err := s.MarkPeer(peer.Name, peer.Fingerprint, identity.Fingerprint, false); err != nil {
				return err
			}
			if err := writeMessage(output, message{Operation: "ok"}); err != nil {
				return err
			}
		case "import":
			p, err := s.Peer(peer.Name)
			if err != nil {
				return err
			}
			if p.DryRunIdentity != identity.Fingerprint {
				return fault.New("sync.dry_run_required", "first authenticated sync dry-run is mandatory")
			}
			bundle, err := readBundle(input, m.Length)
			if err != nil {
				return err
			}
			result, err := s.ImportLogical(bundle, nil)
			if err != nil {
				return err
			}
			if err := writeMessage(output, message{Operation: "imported", Result: &result}); err != nil {
				return err
			}
		case "export":
			p, err := s.Peer(peer.Name)
			if err != nil {
				return err
			}
			if p.DryRunIdentity != identity.Fingerprint {
				return fault.New("sync.dry_run_required", "first authenticated sync dry-run is mandatory")
			}
			if m.Names == nil {
				m.Names = []string{}
			}
			var bundle bytes.Buffer
			result, err := s.ExportLogical(m.Names, rs, "-", &bundle)
			if err != nil {
				return err
			}
			if err := writeMessage(output, message{Operation: "exported", Length: uint64(bundle.Len()), Result: &result}); err != nil {
				return err
			}
			if _, err := output.Write(bundle.Bytes()); err != nil {
				return err
			}
		case "activate":
			p, err := s.Peer(peer.Name)
			if err != nil {
				return err
			}
			if p.DryRunIdentity != identity.Fingerprint {
				return fault.New("sync.dry_run_required", "first authenticated sync dry-run is mandatory")
			}
			if err := s.MarkPeer(peer.Name, peer.Fingerprint, identity.Fingerprint, true); err != nil {
				return err
			}
			if err := writeMessage(output, message{Operation: "ok"}); err != nil {
				return err
			}
		case "bye":
			return writeMessage(output, message{Operation: "bye"})
		default:
			return fault.New("protocol.invalid", "unexpected operation in authenticated session")
		}
	}
}
