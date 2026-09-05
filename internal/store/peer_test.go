package store

import (
	"testing"

	"github.com/agensfield/fulla/internal/crypt"
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
