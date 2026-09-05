package crypt

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"errors"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/agensfield/fulla/internal/fault"
	"golang.org/x/crypto/ssh"
)

// Only disposable test keys use this fixed passphrase.
func sshFixture() ([]byte, []byte, error) {
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(key, "", []byte("fixture-ssh-passphrase"))
	if err != nil {
		return nil, nil, err
	}
	recipient, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(block), ssh.MarshalAuthorizedKey(recipient), nil
}

func TestEncryptedSSHIdentityInteraction(t *testing.T) {
	private, public, err := sshFixture()
	if err != nil {
		t.Fatal(err)
	}
	rs, err := Recipients(public, nil)
	if err != nil {
		t.Fatal(err)
	}
	value := []byte{0, 255, 10}
	ciphertext, err := Encrypt(value, rs)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := Identities(private, nil)
	if err != nil {
		t.Fatal("structural parsing must not unlock", err)
	}
	for _, operation := range []func() error{
		func() error { _, err := Decrypt(ciphertext, ids); return err },
		func() error { return VerifyRecipient(rs, ids) },
	} {
		var problem *fault.Error
		if err := operation(); !errors.As(err, &problem) || problem.Code != "interaction.required" {
			t.Fatal("lost typed interaction refusal", err)
		}
	}
	calls := 0
	var provided []byte
	response := "wrong-passphrase-sentinel"
	ui := &UI{SSHPassphrase: func() ([]byte, error) { calls++; provided = []byte(response); return provided, nil }}
	privateCopy := bytes.Clone(private)
	ids, err = Identities(privateCopy, ui)
	clear(privateCopy)
	if err != nil || calls != 0 {
		t.Fatal("parsing requested input", err)
	}
	other, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyRecipient([]age.Recipient{other.Recipient()}, ids); err == nil || calls != 0 {
		t.Fatal("recipient mismatch requested unlocking", err)
	}
	var problem *fault.Error
	if _, err := Decrypt(ciphertext, ids); !errors.As(err, &problem) || problem.Code != "identity.unlock_failed" || strings.Contains(err.Error(), "sentinel") {
		t.Fatal("bad passphrase not redacted", err)
	}
	if !bytes.Equal(provided, make([]byte, len(provided))) {
		t.Fatal("passphrase buffer retained")
	}
	response = "fixture-ssh-passphrase"
	for range 2 {
		plain, err := Decrypt(ciphertext, ids)
		if err != nil || !bytes.Equal(plain, value) {
			t.Fatal("SSH decrypt failed", err)
		}
	}
	if calls != 2 {
		t.Fatalf("expected retry then cached unlock; calls=%d", calls)
	}
	if !bytes.Equal(provided, make([]byte, len(provided))) {
		t.Fatal("successful passphrase retained")
	}
	cancelled := fault.New("identity.unlock_cancelled", "private callback sentinel")
	cancelled.Status = 143
	ids, err = Identities(private, &UI{SSHPassphrase: func() ([]byte, error) { return []byte("discard"), cancelled }})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyRecipient(rs, ids); !errors.As(err, &problem) || problem.Code != "identity.unlock_cancelled" || problem.Status != 143 || strings.Contains(err.Error(), "sentinel") {
		t.Fatal("cancellation lost status or exposed callback", err)
	}
}

func TestEncryptedRSASSHIdentity(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(key, "", []byte("rsa-fixture-passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := Identities(pem.EncodeToMemory(block), &UI{SSHPassphrase: func() ([]byte, error) { return []byte("rsa-fixture-passphrase"), nil }})
	if err != nil {
		t.Fatal(err)
	}
	rs, err := Recipients(ssh.MarshalAuthorizedKey(pub), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyRecipient(rs, ids); err != nil {
		t.Fatal(err)
	}
}
