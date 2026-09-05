package store

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"filippo.io/age"
	"filippo.io/age/plugin"
	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

const fullArchiveMagic = "FULLA-FULL1\n"

type ArchiveResult struct {
	Path           string `json:"path"`
	Files          int    `json:"files"`
	Bytes          int64  `json:"bytes"`
	IdentityCloned bool   `json:"identity_cloned"`
	Warning        string `json:"warning"`
}

func (s *Store) ExportFull(recipients []age.Recipient, output string) (result ArchiveResult, err error) {
	if err := s.RequireDomain("backup"); err != nil {
		return result, err
	}
	if len(recipients) == 0 {
		return result, fault.Interaction("full export requires separate recovery protection")
	}
	if err := s.CheckArtifactPath(output); err != nil {
		return result, err
	}
	lock, err := s.Lock("backup export")
	if err != nil {
		return result, err
	}
	defer func() {
		if e := lock.Release(); e != nil && err == nil {
			err = fault.Applied("full export completed but shared lock release failed", "export")
		}
	}()
	if _, err := s.CleanGit(); err != nil {
		return result, err
	}
	_, active, err := s.Keys()
	if err != nil {
		return result, err
	}
	separate := false
	for _, r := range recipients {
		if _, ok := r.(*age.ScryptRecipient); ok {
			separate = true
			continue
		}
		text, ok := r.(fmt.Stringer)
		if !ok {
			return result, fault.New("recovery.unsupported_recipient", "cannot compare recovery recipient")
		}
		matches := false
		for _, local := range active {
			if public, ok := local.(fmt.Stringer); ok && public.String() == text.String() {
				matches = true
			}
		}
		if !matches {
			separate = true
		}
	}
	if !separate {
		return result, fault.New("recovery.circular_protection", "full archive cannot be protected solely by its contained active identity")
	}
	var encrypted bytes.Buffer
	ageWriter, err := crypt.EncryptStream(&boundedWriter{writer: &encrypted, remaining: MaxBundleBytes}, recipients)
	if err != nil {
		return result, streamFailure(err, "crypto.encrypt_failed", "could not protect disaster archive")
	}
	if _, err := io.WriteString(ageWriter, fullArchiveMagic); err != nil {
		return result, err
	}
	tw := tar.NewWriter(ageWriter)
	err = fs.WalkDir(s.Root.FS(), ".", func(name string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		if name == "lock" {
			return fs.SkipDir
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		if err := securefs.ValidateInfo(name, info, true); err != nil {
			return err
		}
		header := &tar.Header{Name: name, Mode: 0o600, ModTime: info.ModTime(), Typeflag: tar.TypeReg, Size: info.Size()}
		if e.IsDir() {
			header.Typeflag = tar.TypeDir
			header.Mode = 0o700
			header.Size = 0
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if e.IsDir() {
			return nil
		}
		data, err := securefs.Read(s.Root, name, MaxBundleBytes)
		if err != nil {
			return err
		}
		if _, err := tw.Write(data); err != nil {
			return err
		}
		result.Files++
		result.Bytes += int64(len(data))
		return nil
	})
	if err != nil {
		return result, err
	}
	if err := tw.Close(); err != nil {
		return result, err
	}
	if err := ageWriter.Close(); err != nil {
		return result, err
	}
	if err := s.PublishArtifact(output, encrypted.Bytes()); err != nil {
		return result, err
	}
	result.Path = output
	result.Warning = "External archive copies cannot be revoked by deleting the local copy."
	id := securefs.ID()
	receipt, _ := json.Marshal(map[string]any{"version": 1, "command": "backup export", "full": true, "files": result.Files, "at": time.Now().UTC().Format(time.RFC3339Nano), "applied": true})
	if err := securefs.PublishNew(s.Root, metadata+"/receipts/"+id+".json", receipt); err != nil {
		return result, fault.Applied("full archive published but receipt finalization failed", id)
	}
	return result, nil
}

func RestoreFull(ciphertext []byte, identities []age.Identity, target string) (ArchiveResult, error) {
	return RestoreFullConfirmed(ciphertext, identities, target, nil, nil)
}

// RestoreFullConfirmed validates private staging before asking to publish it.
// The callback receives only metadata; cancellation removes unpublished staging.
func RestoreFullConfirmed(ciphertext []byte, identities []age.Identity, target string, ui *plugin.ClientUI, confirm func(ArchiveResult) error) (result ArchiveResult, err error) {
	result.Path = target
	result.IdentityCloned = true
	result.Warning = "This restore clones the original identity and peer authority. Use it to replace a lost machine, not to onboard a live peer."
	target, err = securefs.Canonical(target, true)
	if err != nil {
		return result, fault.New("recovery.unsafe_target", err.Error())
	}
	emptyExisting := false
	if info, err := os.Lstat(target); err == nil {
		if !info.IsDir() {
			return result, fault.New("recovery.target_exists", "restore target must be empty")
		}
		if err := securefs.ValidateInfo(target, info, true); err != nil {
			return result, err
		}
		entries, err := os.ReadDir(target)
		if err != nil || len(entries) != 0 {
			return result, fault.New("recovery.target_exists", "restore target must be empty")
		}
		emptyExisting = true
	} else if !os.IsNotExist(err) {
		return result, err
	}
	parent, err := securefs.Canonical(filepath.Dir(target), false)
	if err != nil {
		return result, fault.New("recovery.invalid_target", "restore parent must already exist")
	}
	r, err := crypt.DecryptStream(bytes.NewReader(ciphertext), identities)
	if err != nil {
		return result, streamFailure(err, "recovery.decrypt_failed", "could not decrypt full archive")
	}
	magic := make([]byte, len(fullArchiveMagic))
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != fullArchiveMagic {
		return result, fault.New("recovery.invalid_archive", "not a Fulla full-state archive")
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return result, err
	}
	defer root.Close()
	stage := ".fulla-restore-" + securefs.ID()
	if err := root.Mkdir(stage, 0o700); err != nil {
		return result, err
	}
	defer root.RemoveAll(stage)
	staged, err := root.OpenRoot(stage)
	if err != nil {
		return result, err
	}
	defer staged.Close()
	total := int64(0)
	seen := map[string]bool{}
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return result, fault.New("recovery.invalid_archive", "archive structure failed validation")
		}
		name := h.Name
		if name == "." || name == "" || path.Clean(name) != name || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "../") || name == ".." || name == "lock" || strings.HasPrefix(name, "lock/") || seen[name] {
			return result, fault.New("recovery.unsafe_archive", "archive contains an unsafe or duplicate path")
		}
		seen[name] = true
		if h.Typeflag == tar.TypeDir {
			if err := staged.MkdirAll(name, 0o700); err != nil {
				return result, err
			}
			continue
		}
		if h.Typeflag != tar.TypeReg || h.Size < 0 || h.Size > MaxBundleBytes || total > MaxBundleBytes-h.Size {
			return result, fault.New("recovery.unsafe_archive", "archive contains a link, special file, or excessive size")
		}
		total += h.Size
		if err := staged.MkdirAll(path.Dir(name), 0o700); err != nil {
			return result, err
		}
		data, err := io.ReadAll(io.LimitReader(tr, h.Size+1))
		if err != nil || int64(len(data)) != h.Size {
			return result, fault.New("recovery.invalid_archive", "archive file is truncated")
		}
		if err := securefs.WriteNew(staged, name, data); err != nil {
			return result, err
		}
		result.Files++
	}
	// Consume the authenticated age stream through EOF. A valid tar prefix is
	// insufficient if a later ciphertext chunk is corrupt or truncated.
	remaining, err := io.ReadAll(io.LimitReader(r, 1<<20))
	if err != nil {
		return result, fault.New("recovery.invalid_archive", "archive authentication failed")
	}
	for _, b := range remaining {
		if b != 0 {
			return result, fault.New("recovery.invalid_archive", "unexpected trailing archive bytes")
		}
	}
	if len(remaining) == 1<<20 {
		return result, fault.New("recovery.invalid_archive", "excessive archive padding")
	}
	s, err := Open(filepath.Join(parent, stage), true, ui)
	if err != nil {
		return result, err
	}
	if _, err := s.CleanGit(); err != nil {
		s.Close()
		return result, err
	}
	if err := s.DeepVerify(); err != nil {
		s.Close()
		return result, err
	}
	if err := s.Unlocked(); err != nil {
		s.Close()
		return result, err
	}
	s.Close()
	if err := securefs.SyncDir(staged, "."); err != nil {
		return result, err
	}
	result.Bytes = total
	if confirm != nil {
		if err := confirm(result); err != nil {
			return result, err
		}
	}
	if emptyExisting {
		if err := root.Remove(filepath.Base(target)); err != nil {
			return result, fault.New("recovery.target_changed", "restore target is no longer empty")
		}
	}
	if err := securefs.RenameNew(root, stage, filepath.Base(target)); err != nil {
		return result, fault.New("recovery.publish_failed", "could not publish restore without replacement")
	}
	if err := securefs.SyncDir(root, "."); err != nil {
		return result, fault.Applied("restore published but parent synchronization failed", "restore")
	}
	result.Bytes = total
	return result, nil
}
