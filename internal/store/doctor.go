package store

import (
	"bytes"
	"errors"

	"filippo.io/age"
	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

type EntryVerification struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type DoctorResult struct {
	Verification    []EntryVerification  `json:"verification,omitempty"`
	IdentityValid   bool                 `json:"identity_valid"`
	RecipientsValid bool                 `json:"recipients_valid"`
	Plugins         []crypt.PluginStatus `json:"plugins"`
	StorePath       string               `json:"store_path"`
	ConfigPath      string               `json:"config_path"`
	Sources         map[string]string    `json:"sources"`
	Healthy         bool                 `json:"healthy"`
	Deep            bool                 `json:"deep"`
	Entries         int                  `json:"entries"`
	Git             bool                 `json:"git"`
	Lock            *LockInfo            `json:"lock"`
	Issues          []string             `json:"issues"`
	Backups         *BackupSummary       `json:"backups"`
	Peers           int                  `json:"peers"`
	Staging         []string             `json:"staging,omitempty"`
}

// structuralIdentity never attempts key unwrapping. It permits the public age
// parser to finish header validation without any usable identity material.
type structuralIdentity struct{}

func (structuralIdentity) Unwrap([]*age.Stanza) ([]byte, error) { return nil, age.ErrIncorrectIdentity }

func (s *Store) Doctor(deep bool) (DoctorResult, error) {
	r := DoctorResult{Healthy: true, Deep: deep, Issues: []string{}}
	if err := s.Validate(); err != nil {
		return r, err
	}
	lock, err := s.InspectLock()
	if err != nil {
		return r, err
	}
	r.Lock = lock
	if lock != nil {
		r.Healthy = false
		r.Issues = append(r.Issues, "store.locked")
	}
	r.Git, err = s.CleanGit()
	if err != nil {
		r.Healthy = false
		r.Issues = append(r.Issues, "git.dirty_or_unavailable")
	}
	names, err := s.Names()
	if err != nil {
		return r, err
	}
	r.Entries = len(names)
	r.Staging, err = s.inspectStaging()
	if err != nil {
		r.Healthy = false
		r.Issues = append(r.Issues, "transaction.staging_unavailable")
	} else if len(r.Staging) != 0 {
		r.Healthy = false
		r.Issues = append(r.Issues, "transaction.staging_present")
	}
	if err := s.Unlocked(); err != nil {
		r.Healthy = false
		if lock == nil {
			r.Issues = append(r.Issues, "transaction.pending")
		}
	} else {
		backups, err := s.BackupSummary()
		if err != nil {
			r.Healthy = false
			r.Issues = append(r.Issues, "backup.invalid")
		} else {
			r.Backups = &backups
		}
	}
	peers, err := s.Peers()
	if err != nil {
		r.Healthy = false
		r.Issues = append(r.Issues, "peer.invalid")
	} else {
		r.Peers = len(peers)
	}
	private, err := securefs.Read(s.Root, "identities", maxMetadata)
	if err != nil {
		return r, err
	}
	public, err := securefs.Read(s.Root, "recipients", maxMetadata)
	if err != nil {
		return r, err
	}
	ids, err := crypt.Identities(private, nil)
	r.IdentityValid = err == nil
	if err != nil {
		r.Healthy = false
		r.Issues = append(r.Issues, "identity.invalid")
	}
	rs, err := crypt.Recipients(public, nil)
	r.RecipientsValid = err == nil
	if err != nil {
		r.Healthy = false
		r.Issues = append(r.Issues, "recipients.invalid")
	}
	r.Plugins = crypt.Plugins(ids, rs)
	for _, p := range r.Plugins {
		if p.Issue != "" {
			r.Healthy = false
			r.Issues = append(r.Issues, p.Issue+":"+p.Name)
		}
	}
	// The official parser receives an inert identity. It parses the age header
	// without unwrapping a file key or decrypting content; no plugins run.
	for _, name := range names {
		c, err := s.Ciphertext(name)
		if err != nil {
			return r, err
		}
		_, err = age.Decrypt(bytes.NewReader(c), structuralIdentity{})
		var noIdentity *age.NoIdentityMatchError
		if !errors.As(err, &noIdentity) {
			r.Healthy = false
			r.Issues = append(r.Issues, "ciphertext.invalid_header:"+name)
		}
	}
	if deep {
		if err := s.Unlocked(); err != nil {
			return r, err
		}
		ids, recipients, err := s.Keys()
		if err != nil {
			return r, err
		}
		if err := crypt.VerifyRecipient(recipients, ids); err != nil {
			return r, err
		}
		r.Verification = []EntryVerification{}
		for _, name := range names {
			check := EntryVerification{Name: name}
			ciphertext, err := s.Ciphertext(name)
			if err == nil {
				var plaintext []byte
				plaintext, err = crypt.Decrypt(ciphertext, ids)
				clear(plaintext)
			}
			check.OK = err == nil
			if err != nil {
				check.Error = "entry.verification_failed"
				var problem *fault.Error
				if errors.As(err, &problem) {
					check.Error = problem.Code
				}
				r.Healthy = false
				r.Issues = append(r.Issues, "entry.verification_failed:"+name)
			}
			r.Verification = append(r.Verification, check)
		}
	}
	return r, nil
}
