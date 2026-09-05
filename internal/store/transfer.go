package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/protocol"
	"github.com/agensfield/fulla/internal/securefs"
)

const MaxBundleBytes = 512 << 20

type TransferResult struct {
	Names    []string        `json:"names"`
	Skipped  []string        `json:"skipped"`
	Entries  int             `json:"entries"`
	Bytes    uint64          `json:"bytes"`
	Output   string          `json:"output,omitempty"`
	Receipt  string          `json:"receipt,omitempty"`
	Mutation *MutationResult `json:"mutation,omitempty"`
}

// ExportLogical uses PAXFER1 framing inside age. The selector is validated in
// full before the first entry is decrypted or any destination is created.
func (s *Store) ExportLogical(names []string, recipients []age.Recipient, output string, stdout io.Writer) (result TransferResult, err error) {
	result.Skipped = []string{}
	if len(recipients) == 0 {
		return result, fault.Interaction("export requires an explicit recovery recipient")
	}
	if output != "-" {
		if err := s.CheckArtifactPath(output); err != nil {
			return result, err
		}
	}
	lock, err := s.Lock("transfer export")
	if err != nil {
		return result, err
	}
	defer func() {
		if e := lock.Release(); e != nil && err == nil {
			err = fault.Applied("export complete but shared lock release failed", "export")
		}
	}()
	if names == nil {
		names, err = s.Names()
		if err != nil {
			return result, err
		}
	}
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			return result, fault.Usage("duplicate selected entry")
		}
		seen[name] = true
		exists, err := s.Exists(name)
		if err != nil {
			return result, err
		}
		if !exists {
			return result, fault.New("entry.not_found", "selected entry does not exist")
		}
	}
	names = append([]string{}, names...)
	sort.Strings(names)
	ids, _, err := s.Keys()
	if err != nil {
		return result, err
	}
	var encoded bytes.Buffer
	bounded := &boundedWriter{writer: &encoded, remaining: MaxBundleBytes}
	w, err := crypt.EncryptStream(bounded, recipients)
	if err != nil {
		return result, streamFailure(err, "crypto.encrypt_failed", "could not protect recovery bundle")
	}
	e, err := protocol.NewEncoder(w)
	if err != nil {
		return result, err
	}
	for _, name := range names {
		c, err := s.Ciphertext(name)
		if err != nil {
			return result, err
		}
		value, err := crypt.Decrypt(c, ids)
		if err != nil {
			return result, err
		}
		if err := e.Add(name, bytes.NewReader(value)); err != nil {
			return result, err
		}
	}
	stats, err := e.Close()
	if err != nil {
		return result, err
	}
	if err := w.Close(); err != nil {
		return result, err
	}
	result.Names = names
	result.Entries = int(stats.Entries)
	result.Bytes = stats.Bytes
	result.Output = output
	if output == "-" {
		if _, err := stdout.Write(encoded.Bytes()); err != nil {
			return result, err
		}
	} else {
		if err := s.PublishArtifact(output, encoded.Bytes()); err != nil {
			return result, err
		}
	}
	id := securefs.ID()
	result.Receipt = metadata + "/receipts/" + id + ".json"
	data, _ := json.Marshal(map[string]any{"version": 1, "command": "transfer export", "names": names, "entries": stats.Entries, "at": time.Now().UTC().Format(time.RFC3339Nano), "applied": true})
	if err := securefs.PublishNew(s.Root, result.Receipt, data); err != nil {
		return result, fault.Applied("export published but receipt finalization failed", id)
	}
	return result, nil
}

type boundedWriter struct {
	writer    io.Writer
	remaining int64
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, fault.New("transfer.too_large", "bundle exceeds the supported size limit")
	}
	n, err := w.writer.Write(p)
	w.remaining -= int64(n)
	return n, err
}

func (s *Store) PublishArtifact(output string, data []byte) error {
	if err := s.CheckArtifactPath(output); err != nil {
		return err
	}
	if output == "" {
		return fault.Usage("export requires --output PATH or --output -")
	}
	abs, err := securefs.Canonical(output, true)
	if err != nil {
		return fault.New("export.unsafe", err.Error())
	}
	storePath, err := filepath.Abs(s.Dir)
	if err != nil {
		return err
	}
	if abs == storePath || strings.HasPrefix(abs, storePath+string(filepath.Separator)) {
		return fault.New("export.unsafe", "export must be outside the live store")
	}
	r, err := os.OpenRoot(filepath.Dir(abs))
	if err != nil {
		return fault.New("export.invalid_path", "export parent directory must exist")
	}
	defer r.Close()
	if err := securefs.PublishNew(r, filepath.Base(abs), data); err != nil {
		return fault.New("export.publish_failed", "could not publish export without replacement")
	}
	return nil
}

func (s *Store) CheckArtifactPath(output string) error {
	if output == "" {
		return fault.Usage("export requires --output PATH or --output -")
	}
	abs, err := securefs.Canonical(output, true)
	if err != nil {
		return fault.New("export.unsafe", err.Error())
	}
	storePath, err := filepath.Abs(s.Dir)
	if err != nil {
		return err
	}
	if abs == storePath || strings.HasPrefix(abs, storePath+string(filepath.Separator)) {
		return fault.New("export.unsafe", "export must be outside the live store")
	}
	if _, err := os.Lstat(abs); err == nil {
		return fault.New("export.exists", "export destination already exists")
	} else if !os.IsNotExist(err) {
		return fault.New("export.invalid_path", "cannot inspect export destination")
	}
	if _, err := securefs.Canonical(filepath.Dir(abs), false); err != nil {
		return fault.New("export.invalid_path", "export parent directory must exist without symlinks")
	}
	return nil
}

func ReadArtifact(name string, limit int64) ([]byte, error) {
	abs, err := securefs.Canonical(name, false)
	if err != nil {
		return nil, fault.New("artifact.unsafe", err.Error())
	}
	r, err := os.OpenRoot(filepath.Dir(abs))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	data, err := securefs.Read(r, filepath.Base(abs), limit)
	if err != nil {
		return nil, fault.New("artifact.unreadable", "artifact is missing, unsafe, or exceeds the size limit")
	}
	return data, nil
}

type bundleConsumer struct {
	values     map[string][]byte
	names      []string
	current    bytes.Buffer
	writer     *boundedWriter
	total      uint64
	verify     bool
	recipients []age.Recipient
}

func (c *bundleConsumer) BeginEntry(name string) (io.Writer, error) {
	c.names = append(c.names, name)
	c.current.Reset()
	destination := io.Writer(&c.current)
	if c.verify {
		destination = io.Discard
	}
	c.writer = &boundedWriter{writer: destination, remaining: crypt.MaxEntryBytes}
	return c.writer, nil
}
func (c *bundleConsumer) EndEntry(name string, size uint64, _ [sha256.Size]byte) error {
	c.total += size
	if c.total > MaxBundleBytes {
		return fault.New("transfer.too_large", "bundle plaintext exceeds the supported limit")
	}
	if c.verify {
		return nil
	}
	value, err := crypt.Encrypt(c.current.Bytes(), c.recipients)
	if err != nil {
		return err
	}
	c.values[name] = value
	return nil
}

func VerifyLogical(ciphertext []byte, identities []age.Identity) (TransferResult, error) {
	result := TransferResult{Names: []string{}, Skipped: []string{}}
	r, err := crypt.DecryptStream(bytes.NewReader(ciphertext), identities)
	if err != nil {
		return result, streamFailure(err, "transfer.decrypt_failed", "could not decrypt recovery bundle")
	}
	c := &bundleConsumer{verify: true, names: []string{}}
	stats, err := protocol.Decode(r, c)
	if err != nil {
		return result, fault.New("transfer.invalid", "bundle is malformed, exceeds limits, or failed authentication")
	}
	result.Names = c.names
	result.Entries = int(stats.Entries)
	result.Bytes = stats.Bytes
	return result, nil
}

func (s *Store) ImportLogical(ciphertext []byte, identities []age.Identity) (result TransferResult, err error) {
	result.Names = []string{}
	result.Skipped = []string{}
	lock, err := s.Lock("transfer import")
	if err != nil {
		return result, err
	}
	owned := true
	defer func() {
		if owned {
			_ = lock.Release()
		}
	}()
	if _, err := s.CleanGit(); err != nil {
		return result, err
	}
	ids, rs, err := s.Keys()
	if err != nil {
		return result, err
	}
	if identities == nil {
		identities = ids
	}
	r, err := crypt.DecryptStream(bytes.NewReader(ciphertext), identities)
	if err != nil {
		return result, streamFailure(err, "transfer.decrypt_failed", "could not decrypt transfer bundle")
	}
	c := &bundleConsumer{values: map[string][]byte{}, names: []string{}, recipients: rs}
	stats, err := protocol.Decode(r, c)
	if err != nil {
		return result, fault.New("transfer.invalid", "bundle is malformed, exceeds limits, or failed authentication")
	}
	for _, name := range c.names {
		exists, err := s.Exists(name)
		if err != nil {
			return result, err
		}
		if exists {
			delete(c.values, name)
			result.Skipped = append(result.Skipped, name)
		} else {
			result.Names = append(result.Names, name)
		}
	}
	result.Entries = int(stats.Entries)
	result.Bytes = stats.Bytes
	owned = false
	mutation, err := s.mutate(lock, "transfer import", c.values, nil)
	result.Mutation = &mutation
	return result, err
}
