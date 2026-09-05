package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"filippo.io/age/plugin"
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

func Init(directory string, noGit, dryRun bool) (result InitResult, err error) {
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
	stage := ".fulla-init-" + securefs.ID()
	if err := r.Mkdir(stage, 0o700); err != nil {
		return result, err
	}
	defer r.RemoveAll(stage)
	staged, err := r.OpenRoot(stage)
	if err != nil {
		return result, err
	}
	defer staged.Close()
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
	if err := writeMetadata(staged, metadata, newMetadata(), "init"); err != nil {
		return result, err
	}
	s := &Store{Dir: filepath.Join(parent, stage), Root: staged}
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
	if err := securefs.SyncDir(staged, "."); err != nil {
		return result, err
	}
	if err := securefs.RenameNew(r, stage, filepath.Base(directory)); err != nil {
		return result, fault.New("store.publish_failed", "could not publish initialization without replacement")
	}
	if err := securefs.SyncDir(r, "."); err != nil {
		return result, fault.Applied("store initialized but parent synchronization failed", "init")
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

func Adopt(directory string, dryRun bool, ui *plugin.ClientUI) (result InitResult, err error) {
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
	if !dryRun {
		lock, err = s.Lock("adopt")
		if err != nil {
			return result, err
		}
		defer func() {
			if releaseErr := lock.Release(); releaseErr != nil && err == nil {
				err = fault.Applied("adoption finalized but lock release failed", "init")
			}
		}()
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
	stage := ".fulla-adopt-" + securefs.ID()
	defer s.Root.RemoveAll(stage)
	if err := writeMetadata(s.Root, stage, newMetadata(), "init"); err != nil {
		return result, err
	}
	if err := securefs.RenameNew(s.Root, stage, metadata); err != nil {
		return result, fault.New("store.publish_failed", "could not publish adoption metadata without replacement")
	}
	if err := securefs.SyncDir(s.Root, "."); err != nil {
		return result, fault.Applied("adoption applied but directory synchronization failed", "init")
	}
	result.Receipt = metadata + "/receipts/init.json"
	return result, nil
}
