// Package crypt owns age encryption and identity parsing. Errors deliberately
// omit identity text and plugin-controlled output.
package crypt

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"filippo.io/age/plugin"
	"github.com/agensfield/fulla/internal/fault"
)

const MaxEntryBytes = 64 << 20

func Generate() (identity, recipient string, err error) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return "", "", fault.New("identity.generate_failed", "could not generate identity")
	}
	return id.String() + "\n", id.Recipient().String() + "\n", nil
}

func Fingerprint(recipient string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(recipient)))
	return "SHA256:" + hex.EncodeToString(h[:])
}

func Recipients(data []byte, ui *plugin.ClientUI) ([]age.Recipient, error) {
	var out []age.Recipient
	s := bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var r age.Recipient
		var err error
		switch {
		case strings.HasPrefix(line, "ssh-"):
			r, err = agessh.ParseRecipient(line)
		case strings.HasPrefix(line, "age1pq1"):
			r, err = age.ParseHybridRecipient(line)
		case strings.HasPrefix(line, "age1"):
			r, err = age.ParseX25519Recipient(line)
			if err != nil {
				r, err = plugin.NewRecipient(line, ui)
			}
		default:
			err = fault.New("identity.unsupported", "unsupported recipient format")
		}
		if err != nil {
			return nil, fault.New("identity.invalid", "invalid recipient encoding")
		}
		out = append(out, r)
	}
	if s.Err() != nil || len(out) == 0 {
		return nil, fault.New("identity.invalid", "recipient file is empty or malformed")
	}
	return out, nil
}

func Identities(data []byte, ui *plugin.ClientUI) ([]age.Identity, error) {
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("-----BEGIN")) {
		id, err := agessh.ParseIdentity(data)
		if err != nil {
			return nil, fault.New("identity.invalid", "invalid or locked SSH identity; explicit unlocking is required")
		}
		return []age.Identity{id}, nil
	}
	var out []age.Identity
	s := bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "AGE-PLUGIN-") {
			id, err := plugin.NewIdentity(line, ui)
			if err != nil {
				return nil, fault.New("identity.invalid", "invalid plugin identity")
			}
			out = append(out, id)
		} else {
			ids, err := age.ParseIdentities(strings.NewReader(line))
			if err != nil {
				return nil, fault.New("identity.invalid", "invalid private identity encoding")
			}
			out = append(out, ids...)
		}
	}
	if s.Err() != nil || len(out) == 0 {
		return nil, fault.New("identity.invalid", "identity file is empty or malformed")
	}
	return out, nil
}

func Encrypt(value []byte, recipients []age.Recipient) ([]byte, error) {
	var output bytes.Buffer
	w, err := age.Encrypt(&output, recipients...)
	if err != nil {
		return nil, fault.New("crypto.encrypt_failed", "could not wrap encrypted file key")
	}
	if _, err = w.Write(value); err != nil {
		return nil, fault.New("crypto.encrypt_failed", "could not encrypt value")
	}
	if err = w.Close(); err != nil {
		return nil, fault.New("crypto.encrypt_failed", "could not finalize ciphertext")
	}
	return output.Bytes(), nil
}

func Decrypt(ciphertext []byte, identities []age.Identity) ([]byte, error) {
	r, err := age.Decrypt(bytes.NewReader(ciphertext), identities...)
	if err != nil {
		return nil, fault.New("crypto.decrypt_failed", "could not decrypt with the selected identity")
	}
	value, err := io.ReadAll(io.LimitReader(r, MaxEntryBytes+1))
	if err != nil {
		return nil, fault.New("crypto.decrypt_failed", "ciphertext authentication failed")
	}
	if len(value) > MaxEntryBytes {
		return nil, fault.New("entry.too_large", "entry exceeds the supported size limit")
	}
	return value, nil
}

// VerifyRecipient authenticates a temporary challenge without writing a canary.
func VerifyRecipient(recipients []age.Recipient, identities []age.Identity) error {
	const marker = "Fulla identity consistency check v1"
	c, err := Encrypt([]byte(marker), recipients)
	if err != nil {
		return err
	}
	p, err := Decrypt(c, identities)
	if err != nil || string(p) != marker {
		return fault.New("identity.mismatch", "identity cannot decrypt the configured recipient")
	}
	return nil
}
