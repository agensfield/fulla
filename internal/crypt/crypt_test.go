package crypt

import (
	"bytes"
	"testing"
)

func TestExactBytesAndAuthentication(t *testing.T) {
	private, public, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	ids, err := Identities([]byte(private), nil)
	if err != nil {
		t.Fatal(err)
	}
	rs, err := Recipients([]byte(public), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyRecipient(rs, ids); err != nil {
		t.Fatal(err)
	}
	for _, value := range [][]byte{nil, {}, []byte("trailing\n\n"), {0, 255, 254, 0}, bytes.Repeat([]byte{0, 255, 10}, 1<<18)} {
		ciphertext, err := Encrypt(value, rs)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := Decrypt(ciphertext, ids)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(value, actual) {
			t.Fatal("changed plaintext bytes")
		}
		ciphertext[len(ciphertext)-1] ^= 1
		if _, err := Decrypt(ciphertext, ids); err == nil {
			t.Fatal("accepted unauthenticated ciphertext")
		}
	}
}

func TestIdentityErrorsDoNotEchoInput(t *testing.T) {
	secret := "AGE-SECRET-KEY-private-sensitive-invalid"
	_, err := Identities([]byte(secret), nil)
	if err == nil || bytes.Contains([]byte(err.Error()), []byte(secret)) {
		t.Fatal("unredacted identity error")
	}
}
