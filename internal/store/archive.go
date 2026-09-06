package store

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"filippo.io/age"
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
	return s.exportFull(recipients, output, nil)
}

func (s *Store) exportFull(recipients []age.Recipient, output string, hook func(string) error) (result ArchiveResult, err error) {
	if err := s.RequireDomain("backup"); err != nil {
		return result, err
	}
	if len(recipients) == 0 {
		return result, fault.Interaction("full export requires separate recovery protection")
	}
	if err := s.CheckExportPath(output); err != nil {
		return result, err
	}
	lock, err := s.Lock("backup export")
	if err != nil {
		return result, err
	}
	defer func() {
		if err != nil && lock.ExportReceipt != "" {
			err = exportRecoveryRequired(err, lock.ExportReceipt)
			return
		}
		if e := lock.Release(); e != nil && err == nil {
			err = fault.Applied("full export completed but shared lock release failed", "export")
		}
	}()
	if err := s.RequireDomain("backup"); err != nil {
		return result, err
	}
	if err := s.CheckExportPath(output); err != nil {
		return result, err
	}
	if err := s.CheckLockCleanup(); err != nil {
		return result, err
	}
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
		if strings.HasPrefix(name, lockReleasePrefix) {
			return fault.New("store.lock_cleanup_pending", "full archive refuses detached lock cleanup")
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
	if hook != nil {
		if err := hook("encoded"); err != nil {
			return result, err
		}
	}
	receipt, err := s.prepareExport(lock, output, encrypted.Bytes(), exportReceipt{Version: 1, Command: "backup export", Full: true, Files: result.Files, At: time.Now().UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return result, err
	}
	if err := s.publishExport(receipt, encrypted.Bytes(), hook); err != nil {
		return result, err
	}
	result.Path = output
	result.Warning = "External archive copies cannot be revoked by deleting the local copy."
	return result, nil
}

func RestoreFull(ciphertext []byte, identities []age.Identity, target string) (ArchiveResult, error) {
	return RestoreFullConfirmed(ciphertext, identities, target, nil, nil)
}

// RestoreFullConfirmed validates private staging before asking to publish it.
// The callback receives only metadata; cancellation removes unpublished staging
// or reports that private staging cleanup could not be confirmed.
func RestoreFullConfirmed(ciphertext []byte, identities []age.Identity, target string, ui *crypt.UI, confirm func(ArchiveResult) error) (ArchiveResult, error) {
	return restoreFullConfirmed(ciphertext, identities, target, ui, confirm, nil)
}

// afterPhase is an internal failure-injection seam, never a command option.
func restoreFullConfirmed(ciphertext []byte, identities []age.Identity, target string, ui *crypt.UI, confirm func(ArchiveResult) error, afterPhase func(string) error) (result ArchiveResult, err error) {
	checkpoint := func(phase string) error {
		if afterPhase != nil {
			return afterPhase(phase)
		}
		return nil
	}
	result.Path = target
	result.IdentityCloned = true
	result.Warning = "This restore clones the original identity and peer authority. Use it to replace a lost machine, not to onboard a live peer."
	target, emptyExisting, err := checkRestoreTarget(target)
	if err != nil {
		return result, err
	}
	parent := filepath.Dir(target)
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
	restoreID := securefs.ID()
	stage := ".fulla-restore-" + restoreID
	if err := root.Mkdir(stage, 0o700); err != nil {
		return result, err
	}
	published := false
	var restoreLock *Lock
	defer func() {
		// Once renamed, this path no longer names our stage. Never remove a new
		// occupant that happens to reuse the old staging name.
		if published {
			return
		}
		var cleanupErr error
		if restoreLock != nil {
			owned, openErr := root.OpenRoot(stage)
			if openErr != nil {
				cleanupErr = openErr
			} else {
				owner, inspectErr := (&Store{Root: owned}).InspectLock()
				if inspectErr != nil || owner == nil || owner.Token != restoreLock.Token {
					cleanupErr = fault.New("store.lock_changed", "restore staging ownership changed")
				} else {
					cleanupErr = removeCreationContents(owned)
				}
				owned.Close()
			}
		}
		if cleanupErr == nil {
			cleanupErr = root.RemoveAll(stage)
		}
		if cleanupErr == nil {
			cleanupErr = securefs.SyncDir(root, ".")
		}
		if cleanupErr != nil {
			failure := fault.New("recovery.cleanup_failed", "could not confirm removal of private restore staging")
			failure.Details["cleanup_required"] = true
			failure.Details["staging_path"] = filepath.Join(parent, stage)
			failure.Details["target"] = target
			failure.Details["applied"] = false
			var original *fault.Error
			if errors.As(err, &original) {
				failure.Details["operation_code"] = original.Code
				switch original.Status {
				case 129, 130, 131, 143:
					failure.Status = original.Status
				}
			}
			err = failure
		}
	}()
	staged, err := root.OpenRoot(stage)
	if err != nil {
		return result, err
	}
	defer staged.Close()
	stageStore := &Store{Dir: filepath.Join(parent, stage), Root: staged}
	restoreLock, err = stageStore.lock("restore", func() error { return nil })
	if err != nil {
		return result, err
	}
	if err := stageStore.bindCreation(restoreLock, "restore", restoreID, target); err != nil {
		return result, err
	}
	if err := securefs.SyncDir(root, "."); err != nil {
		return result, err
	}
	if err := checkpoint("bound"); err != nil {
		return result, err
	}
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
		if name == "." || name == "" || path.Clean(name) != name || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "../") || name == ".." || name == "lock" || strings.HasPrefix(name, "lock/") || strings.HasPrefix(name, lockReleasePrefix) || seen[name] {
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
		if err := checkpoint("file:" + name); err != nil {
			return result, err
		}
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
	if err := s.bindRestorePublication(restoreLock); err != nil {
		s.Close()
		return result, err
	}
	s.Close()
	if err := syncStagedDirectories(staged, securefs.SyncDir); err != nil {
		return result, err
	}
	if err := checkpoint("validated"); err != nil {
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
		if err := checkpoint("target-vacated"); err != nil {
			return result, err
		}
	}
	if err := securefs.RenameNew(root, stage, filepath.Base(target)); err != nil {
		return result, fault.New("recovery.publish_failed", "could not publish restore without replacement")
	}
	published = true
	stageStore.Dir = target
	if err := checkpoint("published"); err != nil {
		return result, fault.Applied("restore published but completion interrupted", "restore")
	}
	if err := securefs.SyncDir(root, "."); err != nil {
		return result, fault.Applied("restore published but parent synchronization failed", "restore")
	}
	if err := checkpoint("synced"); err != nil {
		return result, fault.Applied("restore published but completion interrupted", "restore")
	}
	if err := restoreLock.Release(); err != nil {
		return result, fault.Applied("restore published but lock release failed", "restore")
	}
	result.Bytes = total
	return result, nil
}

// Files are synced when written. Persist every directory entry bottom-up too,
// including empty directories and ancestors created by MkdirAll.
func syncStagedDirectories(root *os.Root, syncDir func(*os.Root, string) error) error {
	var directories []string
	if err := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			directories = append(directories, name)
		}
		return nil
	}); err != nil {
		return err
	}
	for i := len(directories) - 1; i >= 0; i-- {
		if err := syncDir(root, directories[i]); err != nil {
			return err
		}
	}
	return nil
}
