package remote

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"time"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
)

type challenge struct {
	Version   int    `json:"version"`
	Session   []byte `json:"session"`
	Issuer    string `json:"issuer"`
	Responder string `json:"responder"`
}

func newChallenge(version int, issuer, responder string) ([]byte, error) {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return json.Marshal(challenge{Version: version, Session: nonce, Issuer: issuer, Responder: responder})
}

func checkChallenge(data []byte, version int, issuer, responder string) error {
	var c challenge
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil || c.Version != version || c.Issuer != issuer || c.Responder != responder || len(c.Session) != 32 {
		return fault.New("peer.authentication_failed", "challenge transcript does not match this session")
	}
	return nil
}

func checkProof(proof, expected []byte, started time.Time) error {
	if time.Since(started) > 60*time.Second || subtle.ConstantTimeCompare(proof, expected) != 1 {
		return fault.New("peer.authentication_failed", "challenge proof is wrong, expired, or replayed")
	}
	return nil
}

func validateGreeting(m message) error {
	if m.Version < PreviousProtocol || m.Version > CurrentProtocol {
		return fault.New("peer.protocol_unsupported", "upgrade to a Fulla binary supporting the current or immediately previous protocol")
	}
	if m.Fingerprint != crypt.Fingerprint(m.Recipient) {
		return fault.New("peer.trust_mismatch", "remote public recipient does not match its fingerprint")
	}
	return nil
}
