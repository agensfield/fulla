package store

import (
	"bytes"
	"errors"

	"filippo.io/age"
	"github.com/agensfield/fulla/internal/fault"
)

type DoctorResult struct {
	Healthy bool      `json:"healthy"`
	Deep    bool      `json:"deep"`
	Entries int       `json:"entries"`
	Git     bool      `json:"git"`
	Lock    *LockInfo `json:"lock"`
	Issues  []string  `json:"issues"`
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
		if lock != nil {
			return r, fault.New("store.locked", "deep verification refuses a locked store")
		}
		if err := s.DeepVerify(); err != nil {
			return r, err
		}
	}
	return r, nil
}
