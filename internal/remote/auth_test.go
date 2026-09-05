package remote

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io/fs"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/store"
)

func TestChallengeTranscriptIsStrict(t *testing.T) {
	valid, err := newChallenge(CurrentProtocol, "issuer", "responder")
	if err != nil {
		t.Fatal(err)
	}
	if err := checkChallenge(valid, CurrentProtocol, "issuer", "responder"); err != nil {
		t.Fatal(err)
	}
	var parsed challenge
	if err := json.Unmarshal(valid, &parsed); err != nil {
		t.Fatal(err)
	}
	cases := map[string][]byte{
		"second document":  append(bytes.Clone(valid), []byte(" {}")...),
		"trailing garbage": append(bytes.Clone(valid), 'x'),
		"duplicate field":  append([]byte(`{"version":2,`), valid[1:]...),
		"unknown field":    append([]byte(`{"extra":true,`), valid[1:]...),
	}
	for _, field := range []string{"version", "issuer", "responder", "nonce"} {
		changed := parsed
		switch field {
		case "version":
			changed.Version--
		case "issuer":
			changed.Issuer = "other"
		case "responder":
			changed.Responder = "other"
		case "nonce":
			changed.Session = changed.Session[:31]
		}
		data, err := json.Marshal(changed)
		if err != nil {
			t.Fatal(err)
		}
		cases[field] = data
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if err := checkChallenge(data, CurrentProtocol, "issuer", "responder"); err == nil {
				t.Fatal("accepted malformed or misbound transcript")
			}
		})
	}
	if err := checkProof(valid, valid, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := checkProof(valid, valid, time.Now().Add(-61*time.Second)); err == nil {
		t.Fatal("accepted expired proof")
	}
	other, err := newChallenge(CurrentProtocol, "issuer", "responder")
	if err != nil {
		t.Fatal(err)
	}
	if err := checkProof(valid, other, time.Now()); err == nil {
		t.Fatal("accepted another session proof")
	}
}

func storeDigest(t *testing.T, s *store.Store) map[string][32]byte {
	t.Helper()
	result := map[string][32]byte{}
	err := fs.WalkDir(s.Root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(s.Root.FS(), name)
		if err != nil {
			return err
		}
		result[name] = sha256.Sum256(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func startChallenge(t *testing.T, left, right *store.Store, version int) (*Client, message) {
	t.Helper()
	c := session(t, right, version)
	greeting, err := c.Hello()
	if err != nil {
		t.Fatal(err)
	}
	own, err := left.IdentityShow()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeMessage(c.Conn, message{Operation: "authenticate", Version: version, Fingerprint: own.Fingerprint, Expected: greeting.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	reply, err := readMessage(c.Conn)
	if err != nil || reply.Operation != "challenge" {
		t.Fatal("challenge not issued", err)
	}
	return c, reply
}

func TestCapturedProofCannotAuthorizeFreshSession(t *testing.T) {
	for _, version := range []int{CurrentProtocol, PreviousProtocol} {
		t.Run(strconv.Itoa(version), func(t *testing.T) {
			left, right := paired(t)
			if _, err := right.Write("remote", []byte{0, 255, 10}, false); err != nil {
				t.Fatal(err)
			}
			beforeLeft, beforeRight := storeDigest(t, left), storeDigest(t, right)
			c, issued := startChallenge(t, left, right, version)
			ids, _, err := left.Keys()
			if err != nil {
				t.Fatal(err)
			}
			captured, err := crypt.Decrypt(issued.Challenge, ids)
			if err != nil {
				t.Fatal(err)
			}
			own, err := left.IdentityShow()
			if err != nil {
				t.Fatal(err)
			}
			challenge, err := newChallenge(version, own.Fingerprint, c.Greeting.Fingerprint)
			if err != nil {
				t.Fatal(err)
			}
			_, recipients, err := right.Keys()
			if err != nil {
				t.Fatal(err)
			}
			encrypted, err := crypt.Encrypt(challenge, recipients)
			if err != nil {
				t.Fatal(err)
			}
			proof := message{Operation: "proof", Proof: captured, Challenge: encrypted}
			if err := writeMessage(c.Conn, proof); err != nil {
				t.Fatal(err)
			}
			reply, err := readMessage(c.Conn)
			if err != nil || reply.Operation != "authenticated" || !bytes.Equal(reply.Proof, challenge) {
				t.Fatal("positive control failed mutual authentication", err)
			}
			if err := writeMessage(c.Conn, proof); err != nil {
				t.Fatal(err)
			}
			_, err = readMessage(c.Conn)
			var duplicate *fault.Error
			if !errors.As(err, &duplicate) || duplicate.Code != "protocol.invalid" {
				t.Fatal("proof reused in authenticated session", err)
			}
			_ = c.Conn.Close()
			replay, _ := startChallenge(t, left, right, version)
			if err := writeMessage(replay.Conn, proof); err != nil {
				t.Fatal(err)
			}
			_, err = readMessage(replay.Conn)
			var problem *fault.Error
			if !errors.As(err, &problem) || problem.Code != "peer.authentication_failed" {
				t.Fatal("captured proof was accepted", err)
			}
			_ = replay.Conn.Close()
			if !reflect.DeepEqual(beforeLeft, storeDigest(t, left)) || !reflect.DeepEqual(beforeRight, storeDigest(t, right)) {
				t.Fatal("authentication replay changed store files")
			}
		})
	}
}
