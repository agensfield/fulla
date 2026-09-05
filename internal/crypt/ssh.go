package crypt

import (
	"bytes"
	"errors"
	"sync"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"filippo.io/age/plugin"
	"github.com/agensfield/fulla/internal/fault"
	"golang.org/x/crypto/ssh"
)

// UI separates built-in identity unlocking from externally supplied plugin UI.
// Nil callbacks never acquire input. SSHPassphrase transfers ownership of its
// returned buffer to the caller, which clears it after unlocking.
type UI struct {
	plugin.ClientUI
	SSHPassphrase func() ([]byte, error)
}

func (ui *UI) plugin() *plugin.ClientUI {
	if ui == nil {
		return nil
	}
	return &ui.ClientUI
}

type encryptedSSHIdentity struct {
	mu         sync.Mutex
	identity   *agessh.EncryptedSSHIdentity
	passphrase []byte
	failure    error
}

func parseSSHIdentity(data []byte, ui *UI) (age.Identity, error) {
	id, err := agessh.ParseIdentity(data)
	if err == nil {
		return id, nil
	}
	var locked *ssh.PassphraseMissingError
	if !errors.As(err, &locked) {
		return nil, fault.New("identity.invalid", "invalid or unsupported SSH identity")
	}
	if locked.PublicKey == nil {
		return nil, fault.New("identity.unsupported", "encrypted SSH identity requires an OpenSSH file with an embedded public key")
	}
	wrapper := &encryptedSSHIdentity{}
	wrapper.identity, err = agessh.NewEncryptedSSHIdentity(locked.PublicKey, bytes.Clone(data), func() ([]byte, error) {
		if ui == nil || ui.SSHPassphrase == nil {
			wrapper.failure = fault.Interaction("encrypted SSH identity requires a controlling-terminal passphrase")
			return nil, wrapper.failure
		}
		value, err := ui.SSHPassphrase()
		wrapper.passphrase = value
		if err != nil {
			wrapper.failure = fault.New("identity.unlock_failed", "could not obtain SSH identity passphrase")
			var problem *fault.Error
			if errors.As(err, &problem) {
				switch problem.Code {
				case "interaction.required":
					wrapper.failure = fault.Interaction("encrypted SSH identity requires a controlling-terminal passphrase")
				case "identity.unlock_cancelled":
					cancelled := fault.New("identity.unlock_cancelled", "SSH identity unlocking cancelled")
					switch problem.Status {
					case 129, 130, 131, 143:
						cancelled.Status = problem.Status
					}
					wrapper.failure = cancelled
				}
			}
			return nil, wrapper.failure
		}
		return value, nil
	})
	if err != nil {
		return nil, fault.New("identity.unsupported", "unsupported encrypted SSH identity")
	}
	return wrapper, nil
}

func (i *encryptedSSHIdentity) Unwrap(stanzas []*age.Stanza) ([]byte, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.failure = nil
	defer func() { clear(i.passphrase); i.passphrase = nil }()
	key, err := i.identity.Unwrap(stanzas)
	if i.failure != nil {
		clear(key)
		return nil, i.failure
	}
	if err != nil && !errors.Is(err, age.ErrIncorrectIdentity) {
		clear(key)
		return nil, fault.New("identity.unlock_failed", "could not unlock or use encrypted SSH identity")
	}
	return key, err
}
