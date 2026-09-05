// Package store implements the pa-v1 layout and Fulla's private metadata.
package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"filippo.io/age"
	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/protocol"
	"github.com/agensfield/fulla/internal/securefs"
)

const metadata = ".fulla"
const maxMetadata = 4 << 20

type Metadata struct {
	Version int            `json:"version"`
	Profile string         `json:"profile"`
	StoreID string         `json:"store_id"`
	Domains map[string]int `json:"domains"`
}

type Store struct {
	Dir                     string
	Root                    *os.Root
	Meta                    Metadata
	UI                      *crypt.UI
	ExpectedFingerprint     string
	ExpectedPeerName        string
	ExpectedPeerFingerprint string
}

func Open(directory string, adopted bool, ui *crypt.UI) (*Store, error) {
	root, err := securefs.Open(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fault.New("store.uninitialized", "store does not exist; run fulla init")
	}
	if err != nil {
		return nil, fault.New("store.unsafe", err.Error())
	}
	s := &Store{Dir: directory, Root: root, UI: ui}
	if err := s.Validate(); err != nil {
		root.Close()
		return nil, err
	}
	data, err := securefs.Read(root, metadata+"/store.json", maxMetadata)
	if errors.Is(err, fs.ErrNotExist) && !adopted {
		return s, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		root.Close()
		return nil, fault.New("store.uninitialized", "compatible candidate requires fulla init --adopt --dry-run")
	}
	if err != nil {
		root.Close()
		return nil, fault.New("metadata.invalid", "cannot read store metadata")
	}
	if err := decodeMetadata(data, &s.Meta); err != nil {
		root.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.Root.Close() }

func (s *Store) Validate() error {
	if err := securefs.ValidateTree(s.Root); err != nil {
		return fault.New("store.unsafe", err.Error())
	}
	for _, name := range []string{"identities", "recipients"} {
		if _, err := securefs.Read(s.Root, name, maxMetadata); err != nil {
			return fault.New("store.invalid", "missing or unsafe "+name)
		}
	}
	info, err := s.Root.Lstat("passwords")
	if err != nil || !info.IsDir() {
		return fault.New("store.invalid", "missing passwords directory")
	}
	_, err = s.Names()
	return err
}

func decodeMetadata(data []byte, out *Metadata) error {
	if err := StrictJSON(data, out); err != nil {
		return err
	}
	if out.Version != 1 || out.Profile != "pa-v1" || out.StoreID == "" {
		return fault.New("metadata.unsupported", "unsupported store metadata")
	}
	return nil
}

func (s *Store) domainVersion(domain string) (int, error) {
	// Long-lived RPC/store handles must not authorize writes using the manifest
	// cached at Open. Repeated checks under mutation locks must stay authoritative.
	data, err := securefs.Read(s.Root, metadata+"/store.json", maxMetadata)
	if err != nil {
		return 0, fault.New("metadata.invalid", "cannot read current store metadata")
	}
	var current Metadata
	if err := decodeMetadata(data, &current); err != nil {
		return 0, err
	}
	v, ok := current.Domains[domain]
	if !ok || v < 1 {
		return 0, fault.New("metadata.unsupported", "unsupported "+domain+" metadata; use a compatible Fulla binary")
	}
	return v, nil
}

func (s *Store) RequireDomain(domain string) error {
	v, err := s.domainVersion(domain)
	if err != nil {
		return err
	}
	if v != 1 {
		return fault.New("metadata.unsupported", "unsupported "+domain+" metadata; use a compatible Fulla binary")
	}
	return nil
}

func StrictJSON(data []byte, out any) error {
	if !json.Valid(data) {
		return fault.New("metadata.invalid", "malformed JSON metadata")
	}
	// A token walk rejects duplicate object members, which encoding/json's
	// ordinary decoder otherwise silently accepts.
	d := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueObjectKeys(d); err != nil {
		return err
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return fault.New("metadata.invalid", "unknown field or invalid metadata shape")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fault.New("metadata.invalid", "trailing metadata")
	}
	return nil
}

func uniqueObjectKeys(d *json.Decoder) error {
	tok, err := d.Token()
	if err != nil {
		return fault.New("metadata.invalid", "malformed metadata")
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	if delim == '{' {
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fault.New("metadata.invalid", "duplicate metadata field")
			}
			seen[name] = true
			if err := uniqueObjectKeys(d); err != nil {
				return err
			}
		}
	} else if delim == '[' {
		for d.More() {
			if err := uniqueObjectKeys(d); err != nil {
				return err
			}
		}
	}
	_, err = d.Token()
	return err
}

func (s *Store) Names() ([]string, error) {
	names := []string{}
	err := fs.WalkDir(s.Root.FS(), "passwords", func(name string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "passwords" {
			return nil
		}
		if e.IsDir() && path.Base(name) == ".git" {
			return fs.SkipDir
		}
		if e.IsDir() {
			return nil
		}
		// Shell pa creates this Git metadata file beside its encrypted entries.
		// Preserve it as repository configuration, never as a password entry.
		if name == "passwords/.gitattributes" {
			return nil
		}
		if !strings.HasSuffix(name, ".age") {
			return fault.New("store.invalid", "unexpected file in password directory")
		}
		entry := strings.TrimSuffix(strings.TrimPrefix(name, "passwords/"), ".age")
		if err := protocol.ValidateName(entry); err != nil {
			return fault.New("entry.invalid_name", "invalid stored entry name")
		}
		names = append(names, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func EntryPath(name string) (string, error) {
	if err := protocol.ValidateName(name); err != nil {
		return "", fault.Usage("invalid entry name")
	}
	return "passwords/" + name + ".age", nil
}

func (s *Store) Exists(name string) (bool, error) {
	p, err := EntryPath(name)
	if err != nil {
		return false, err
	}
	info, err := s.Root.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := securefs.ValidateInfo(p, info, true); err != nil {
		return false, fault.New("store.unsafe", err.Error())
	}
	return info.Mode().IsRegular(), nil
}

func (s *Store) Keys() ([]age.Identity, []age.Recipient, error) {
	private, err := securefs.Read(s.Root, "identities", maxMetadata)
	if err != nil {
		return nil, nil, err
	}
	public, err := securefs.Read(s.Root, "recipients", maxMetadata)
	if err != nil {
		return nil, nil, err
	}
	ids, err := crypt.Identities(private, s.UI)
	if err != nil {
		return nil, nil, err
	}
	rs, err := crypt.Recipients(public, s.UI)
	return ids, rs, err
}

func (s *Store) Ciphertext(name string) ([]byte, error) {
	p, err := EntryPath(name)
	if err != nil {
		return nil, err
	}
	data, err := securefs.Read(s.Root, p, crypt.MaxEntryBytes+1<<20)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fault.New("entry.not_found", "entry does not exist")
	}
	return data, err
}

func (s *Store) Read(name string) ([]byte, error) {
	if err := s.Unlocked(); err != nil {
		return nil, err
	}
	c, err := s.Ciphertext(name)
	if err != nil {
		return nil, err
	}
	ids, _, err := s.Keys()
	if err != nil {
		return nil, err
	}
	return crypt.Decrypt(c, ids)
}

func (s *Store) DeepVerify() error {
	ids, rs, err := s.Keys()
	if err != nil {
		return err
	}
	if err := crypt.VerifyRecipient(rs, ids); err != nil {
		return err
	}
	names, err := s.Names()
	if err != nil {
		return err
	}
	for _, name := range names {
		c, err := s.Ciphertext(name)
		if err != nil {
			return err
		}
		plain, err := crypt.Decrypt(c, ids)
		clear(plain)
		if err != nil {
			classified := streamFailure(err, "store.decrypt_failed", "entry verification failed")
			var e *fault.Error
			if !errors.As(classified, &e) || e.Code != "store.decrypt_failed" {
				return classified
			}
			e.Details["name"] = name
			return e
		}
	}
	return nil
}

func (s *Store) Unlocked() error {
	if _, err := s.Root.Lstat("lock"); err == nil {
		return fault.New("store.locked", "store is locked; inspect with fulla doctor")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, pending := range []string{"pending.json", "rotation.json", "prune.json"} {
		if _, err := s.Root.Lstat(metadata + "/" + pending); err == nil {
			return fault.New("transaction.pending", "explicit transaction recovery is required")
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (s *Store) PasswordDirectory() string { return filepath.Join(s.Dir, "passwords") }
