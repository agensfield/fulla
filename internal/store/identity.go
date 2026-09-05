package store

import (
	"io/fs"
	"strings"

	"filippo.io/age"
	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

type IdentityInfo struct {
	Recipient   string `json:"recipient"`
	Fingerprint string `json:"fingerprint"`
}

func (s *Store) IdentityShow() (IdentityInfo, error) {
	if err := s.RequireDomain("identity"); err != nil {
		return IdentityInfo{}, err
	}
	data, err := securefs.Read(s.Root, "recipients", maxMetadata)
	if err != nil {
		return IdentityInfo{}, err
	}
	return IdentityInfo{Recipient: strings.TrimSpace(string(data)), Fingerprint: crypt.Fingerprint(string(data))}, nil
}

// RecoveryKeys is deliberately used only by history/backup recovery. Ordinary
// entry reads never acquire retired keys. Successive rotations form a chain of
// sealed identities, unwrapped from the current key toward earlier generations.
func (s *Store) RecoveryKeys() ([]age.Identity, error) {
	if err := s.RequireDomain("identity"); err != nil {
		return nil, err
	}
	ids, _, err := s.Keys()
	if err != nil {
		return nil, err
	}
	root, err := s.Root.OpenRoot(metadata + "/retired")
	if err != nil {
		return nil, err
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, err
	}
	if len(entries) > 1024 {
		return nil, fault.New("identity.too_many_retired", "retired identity count exceeds supported recovery limit")
	}
	loaded := map[string]bool{}
	for {
		progress := false
		for _, entry := range entries {
			if loaded[entry.Name()] {
				continue
			}
			if !strings.HasSuffix(entry.Name(), ".age") || !validID(strings.TrimSuffix(entry.Name(), ".age")) {
				return nil, fault.New("identity.invalid_retired", "invalid retired identity filename")
			}
			data, err := securefs.Read(root, entry.Name(), maxMetadata)
			if err != nil {
				return nil, err
			}
			plain, err := crypt.Decrypt(data, ids)
			if err != nil {
				continue
			}
			more, err := crypt.Identities(plain, s.UI)
			if err != nil {
				return nil, err
			}
			ids = append(ids, more...)
			loaded[entry.Name()] = true
			progress = true
		}
		if !progress {
			break
		}
	}
	return ids, nil
}
