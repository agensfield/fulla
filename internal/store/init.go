package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

type InitResult struct {
	Store       string `json:"store"`
	Profile     string `json:"profile"`
	Git         bool   `json:"git"`
	Adopted     bool   `json:"adopted"`
	DryRun      bool   `json:"dry_run"`
	Entries     int    `json:"entries"`
	Fingerprint string `json:"fingerprint"`
	Receipt     string `json:"receipt,omitempty"`
}

func newMetadata() Metadata {
	return Metadata{Version: 1, Profile: "pa-v1", StoreID: securefs.ID(), Domains: map[string]int{"peers": 1, "sync": 1, "backup": 1, "identity": 1, "transactions": 1}}
}

func writeMetadata(root *os.Root, dir string, meta Metadata, operation string) error {
	if err := root.Mkdir(dir, 0o700); err != nil {
		return err
	}
	return writeMetadataContents(root, dir, meta, operation)
}

func writeMetadataContents(root *os.Root, dir string, meta Metadata, operation string) error {
	for _, sub := range []string{"receipts", "backups", "transactions", "peers", "retired"} {
		if err := root.Mkdir(dir+"/"+sub, 0o700); err != nil {
			return err
		}
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := securefs.WriteNew(root, dir+"/store.json", data); err != nil {
		return err
	}
	receipt := map[string]any{"version": 1, "command": operation, "at": time.Now().UTC().Format(time.RFC3339Nano), "profile": "pa-v1", "applied": true}
	data, err = json.Marshal(receipt)
	if err != nil {
		return err
	}
	return securefs.WriteNew(root, dir+"/receipts/init.json", data)
}

func Init(directory string, noGit, dryRun bool) (InitResult, error) {
	return initialize(directory, noGit, dryRun, nil)
}

func initialize(directory string, noGit, dryRun bool, hook func(string) error) (result InitResult, err error) {
	result = InitResult{Store: directory, Profile: "pa-v1", Git: !noGit, DryRun: dryRun}
	directory, err = securefs.Canonical(directory, true)
	if err != nil {
		return result, fault.New("store.unsafe", err.Error())
	}
	if _, err := os.Lstat(directory); err == nil {
		return result, fault.New("store.exists", "initialization destination already exists; adoption must be explicit")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return result, err
	}
	if dryRun {
		return result, nil
	}
	parent := filepath.Dir(directory)
	// Only init creates missing parent directories. No usable partial store is
	// exposed: all identity and metadata files are written in a private sibling.
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return result, err
	}
	if _, err := securefs.Canonical(parent, false); err != nil {
		return result, err
	}
	r, err := os.OpenRoot(parent)
	if err != nil {
		return result, err
	}
	defer r.Close()
	meta := newMetadata()
	stage := ".fulla-init-" + meta.StoreID
	if err := r.Mkdir(stage, 0o700); err != nil {
		return result, err
	}
	ownedStage := true
	var initLock *Lock
	defer func() {
		if !ownedStage {
			return
		}
		var cleanupErr error
		if initLock != nil {
			root, openErr := r.OpenRoot(stage)
			if openErr != nil {
				cleanupErr = openErr
			} else {
				owner, inspectErr := (&Store{Root: root}).InspectLock()
				if inspectErr != nil || owner == nil || owner.Token != initLock.Token {
					cleanupErr = fault.New("store.lock_changed", "initialization staging ownership changed")
				} else {
					cleanupErr = removeCreationContents(root)
				}
				root.Close()
			}
		}
		if cleanupErr == nil {
			cleanupErr = r.RemoveAll(stage)
		}
		if cleanupErr == nil {
			cleanupErr = securefs.SyncDir(r, ".")
		}
		if cleanupErr != nil {
			failure := fault.New("store.cleanup_failed", "could not confirm removal of private initialization staging")
			failure.Details["applied"] = false
			failure.Details["cleanup_required"] = true
			failure.Details["staging_path"] = filepath.Join(parent, stage)
			failure.Details["target"] = directory
			var original *fault.Error
			if errors.As(err, &original) {
				failure.Details["operation_code"] = original.Code
				if original.Status >= 128 {
					failure.Status = original.Status
				}
			}
			err = failure
		}
	}()
	staged, err := r.OpenRoot(stage)
	if err != nil {
		return result, err
	}
	defer staged.Close()
	s := &Store{Dir: filepath.Join(parent, stage), Root: staged}
	initLock, err = s.lock("init", func() error { return nil })
	if err != nil {
		return result, err
	}
	if err := s.bindCreation(initLock, "init", meta.StoreID, directory); err != nil {
		return result, err
	}
	if err := securefs.SyncDir(r, "."); err != nil {
		return result, err
	}
	if hook != nil {
		if err := hook("bound"); err != nil {
			return result, err
		}
	}
	if err := staged.Mkdir("passwords", 0o700); err != nil {
		return result, err
	}
	private, public, err := crypt.Generate()
	if err != nil {
		return result, err
	}
	for name, data := range map[string]string{"identities": private, "recipients": public} {
		if err := securefs.WriteNew(staged, name, []byte(data)); err != nil {
			return result, err
		}
	}
	if hook != nil {
		if err := hook("keys"); err != nil {
			return result, err
		}
	}
	if err := writeMetadata(staged, metadata, meta, "init"); err != nil {
		return result, err
	}
	if !noGit {
		if _, err := s.Git("init", "--initial-branch=main"); err != nil {
			return result, err
		}
		if _, err := s.Git("-c", "user.name=Fulla", "-c", "user.email=fulla@localhost", "commit", "--allow-empty", "-m", "Initialize encrypted Fulla history"); err != nil {
			return result, err
		}
		// Git obeys process umask but can derive modes from shared configuration.
		// Restrict only our newly created repository before publication.
		if err := privateModes(staged, "passwords/.git"); err != nil {
			return result, err
		}
	}
	if err := s.Validate(); err != nil {
		return result, err
	}
	if err := s.DeepVerify(); err != nil {
		return result, err
	}
	if err := syncInitializationAt(staged, "."); err != nil {
		return result, err
	}
	if hook != nil {
		if err := hook("staged"); err != nil {
			return result, err
		}
	}
	if err := securefs.RenameNew(r, stage, filepath.Base(directory)); err != nil {
		return result, fault.New("store.publish_failed", "could not publish initialization without replacement")
	}
	ownedStage = false
	s.Dir = directory
	if hook != nil {
		if err := hook("published"); err != nil {
			return result, fault.Applied("store initialized but finalization interrupted", "init")
		}
	}
	if err := securefs.SyncDir(r, "."); err != nil {
		return result, fault.Applied("store initialized but parent synchronization failed", "init")
	}
	if err := initLock.Release(); err != nil {
		return result, fault.Applied("store initialized but lock release failed", "init")
	}
	result.Fingerprint = crypt.Fingerprint(public)
	result.Receipt = metadata + "/receipts/init.json"
	return result, nil
}

func privateModes(root *os.Root, dir string) error {
	return fs.WalkDir(root.FS(), dir, func(name string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		mode := fs.FileMode(0o600)
		if e.IsDir() {
			mode = 0o700
		}
		return root.Chmod(name, mode)
	})
}

func Adopt(directory string, dryRun bool, ui *crypt.UI) (InitResult, error) {
	return adopt(directory, dryRun, ui, nil)
}

func adopt(directory string, dryRun bool, ui *crypt.UI, hook func(string) error) (result InitResult, err error) {
	result = InitResult{Store: directory, Profile: "pa-v1", Adopted: true, DryRun: dryRun}
	s, err := Open(directory, false, ui)
	if err != nil {
		return result, err
	}
	defer s.Close()
	if s.Meta.StoreID != "" {
		return result, fault.New("store.exists", "store already adopted")
	}
	if err := s.Unlocked(); err != nil {
		return result, err
	}
	var lock *Lock
	stage := ""
	published := false
	if !dryRun {
		lock, err = s.Lock("adopt")
		if err != nil {
			return result, err
		}
		defer func() {
			var cleanupErr error
			if stage != "" && !published {
				// Only the unchanged lock owner may remove unpublished staging.
				var owner []byte
				owner, cleanupErr = securefs.Read(s.Root, "lock/owner", 256)
				if cleanupErr == nil && strings.TrimSpace(string(owner)) != lock.Token {
					cleanupErr = fault.New("store.lock_changed", "lock ownership changed")
				}
				if cleanupErr == nil {
					cleanupErr = s.Root.RemoveAll(stage)
				}
				if cleanupErr == nil {
					cleanupErr = securefs.SyncDir(s.Root, ".")
				}
			}
			var releaseErr error
			if cleanupErr == nil {
				releaseErr = lock.Release()
			} else {
				// Keep the binding available for explicit recovery after failed
				// unpublished cleanup, including a failed directory synchronization.
				releaseErr = fault.New("store.cleanup_failed", "adoption cleanup requires lock inspection")
			}
			if cleanupErr != nil || releaseErr != nil {
				failure := fault.New("store.cleanup_failed", "could not confirm adoption staging or lock cleanup")
				failure.Details["applied"] = published
				failure.Details["cleanup_required"] = true
				failure.Details["target"] = s.Dir
				if cleanupErr != nil {
					failure.Details["staging_path"] = filepath.Join(s.Dir, stage)
				}
				if releaseErr != nil {
					failure.Details["lock_cleanup_required"] = true
				}
				var original *fault.Error
				if errors.As(err, &original) {
					failure.Details["operation_code"] = original.Code
					if original.Status >= 128 {
						failure.Status = original.Status
					}
				}
				if published {
					failure.Status = 3
				}
				err = failure
			}
		}()
		if hook != nil {
			if err := hook("locked"); err != nil {
				return result, err
			}
		}
	}
	result.Git, err = s.CleanGit()
	if err != nil {
		return result, err
	}
	if err := s.DeepVerify(); err != nil {
		return result, err
	}
	names, err := s.Names()
	if err != nil {
		return result, err
	}
	result.Entries = len(names)
	public, err := securefs.Read(s.Root, "recipients", maxMetadata)
	if err != nil {
		return result, err
	}
	result.Fingerprint = crypt.Fingerprint(string(public))
	if dryRun {
		return result, nil
	}
	meta := newMetadata()
	stageName := ".fulla-adopt-" + meta.StoreID
	if err := s.Root.Mkdir(stageName, 0o700); err != nil {
		return result, err
	}
	stage = stageName
	if err := s.bindAdoption(lock, meta.StoreID); err != nil {
		return result, err
	}
	if hook != nil {
		if err := hook("bound"); err != nil {
			return result, err
		}
	}
	if err := writeMetadataContents(s.Root, stage, meta, "init"); err != nil {
		return result, err
	}
	if err := syncInitializationAt(s.Root, stage); err != nil {
		return result, err
	}
	if hook != nil {
		if err := hook("staged"); err != nil {
			return result, err
		}
	}
	if err := securefs.RenameNew(s.Root, stage, metadata); err != nil {
		return result, fault.New("store.publish_failed", "could not publish adoption metadata without replacement")
	}
	published = true
	if hook != nil {
		if err := hook("published"); err != nil {
			return result, fault.Applied("adoption applied but finalization interrupted", "init")
		}
	}
	if err := securefs.SyncDir(s.Root, "."); err != nil {
		return result, fault.Applied("adoption applied but directory synchronization failed", "init")
	}
	result.Receipt = metadata + "/receipts/init.json"
	return result, nil
}
