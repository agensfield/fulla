package store

import (
	"errors"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

type Backup struct {
	SnapshotDomain string `json:"snapshot_domain,omitempty"`
	ID             string `json:"id"`
	Started        string `json:"started"`
	Command        string `json:"command"`
	Bytes          int64  `json:"bytes"`
}

func (s *Store) Backups() ([]Backup, error) {
	if err := s.RequireDomain("backup"); err != nil {
		return nil, err
	}
	backups := []Backup{}
	for _, domain := range []string{"", "transactions"} {
		items, err := s.snapshotBackups(domain)
		if err != nil {
			return nil, err
		}
		backups = append(backups, items...)
	}
	sort.Slice(backups, func(i, j int) bool {
		a, _ := time.Parse(time.RFC3339Nano, backups[i].Started)
		b, _ := time.Parse(time.RFC3339Nano, backups[j].Started)
		if a.Equal(b) {
			return backups[i].ID > backups[j].ID
		}
		return a.After(b)
	})
	return backups, nil
}

func (s *Store) snapshotBackups(domain string) ([]Backup, error) {
	base := snapshotBase(domain)
	root, err := s.Root.OpenRoot(base)
	if errors.Is(err, fs.ErrNotExist) && domain == "transactions" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := s.requireSnapshotDomain(domain); err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, err
	}
	backups := []Backup{}
	for _, entry := range entries {
		if !validID(entry.Name()) {
			if strings.HasPrefix(entry.Name(), ".fulla-stage-") {
				continue
			}
			return nil, fault.New("backup.invalid", "invalid backup directory")
		}
		j, err := s.BackupShow(entry.Name())
		if err != nil {
			return nil, err
		}
		if _, err := time.Parse(time.RFC3339Nano, j.Started); err != nil {
			return nil, fault.New("backup.invalid", "backup timestamp is invalid")
		}
		b := Backup{SnapshotDomain: domain, ID: j.ID, Started: j.Started, Command: j.Command}
		err = fs.WalkDir(root.FS(), entry.Name(), func(name string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !e.IsDir() {
				info, err := e.Info()
				if err != nil {
					return err
				}
				b.Bytes += info.Size()
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		backups = append(backups, b)
	}
	return backups, nil
}

func (s *Store) BackupShow(id string) (Journal, error) {
	var j Journal
	if err := s.RequireDomain("backup"); err != nil {
		return j, err
	}
	if !validID(id) {
		return j, fault.Usage("invalid backup identifier")
	}
	location, domain, err := s.snapshotLocation(id)
	if err != nil {
		return j, err
	}
	data, err := securefs.Read(s.Root, location+"/journal.json", maxMetadata)
	if err != nil {
		return j, fault.New("backup.not_found", "backup metadata is missing or unreadable")
	}
	if err := StrictJSON(data, &j); err != nil {
		return j, err
	}
	if j.Version != 1 || j.ID != id || j.SnapshotDomain != domain {
		return j, fault.New("backup.invalid", "backup identity/version mismatch")
	}
	return j, nil
}

type BackupRestorePlan struct {
	ID       string   `json:"id"`
	Phase    string   `json:"phase"`
	Added    []string `json:"added"`
	Replaced []string `json:"replaced"`
	Removed  []string `json:"removed"`
}

func (s *Store) BackupRestore(id, phase string) (MutationResult, error) {
	return s.BackupRestoreConfirmed(id, phase, nil)
}

func (s *Store) BackupRestoreConfirmed(id, phase string, confirm func(BackupRestorePlan) error) (result MutationResult, err error) {
	if !validID(id) {
		return result, fault.Usage("invalid backup identifier")
	}
	if phase != "before" && phase != "after" {
		return result, fault.Usage("backup phase must be before or after")
	}
	lock, err := s.Lock("backup restore")
	if err != nil {
		return result, err
	}
	owned := true
	defer func() {
		if owned {
			if e := lock.Release(); e != nil && err == nil {
				err = e
			}
		}
	}()
	if _, err := s.BackupShow(id); err != nil {
		return result, err
	}
	if _, err := s.CleanGit(); err != nil {
		return result, err
	}
	active, rs, err := s.Keys()
	if err != nil {
		return result, err
	}
	if err := crypt.VerifyRecipient(rs, active); err != nil {
		return result, err
	}
	ids, err := s.RecoveryKeys()
	if err != nil {
		return result, err
	}
	location, _, err := s.snapshotLocation(id)
	if err != nil {
		return result, err
	}
	base := location + "/" + phase + "/passwords"
	values := map[string][]byte{}
	err = fs.WalkDir(s.Root.FS(), base, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			if path.Base(p) == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if p == base+"/.gitattributes" {
			// Entry-set recovery preserves the live repository configuration.
			return nil
		}
		if !strings.HasSuffix(p, ".age") {
			return fault.New("backup.invalid", "unexpected backup file")
		}
		name := strings.TrimSuffix(strings.TrimPrefix(p, base+"/"), ".age")
		if _, err := EntryPath(name); err != nil {
			return err
		}
		c, err := securefs.Read(s.Root, p, 65<<20)
		if err != nil {
			return err
		}
		plain, err := crypt.Decrypt(c, ids)
		if err != nil {
			return err
		}
		c, err = crypt.Encrypt(plain, rs)
		clear(plain)
		if err != nil {
			return err
		}
		verified, err := crypt.Decrypt(c, active)
		clear(verified)
		if err != nil {
			return err
		}
		values[name] = c
		return nil
	})
	if err != nil {
		return result, err
	}
	live, err := s.Names()
	if err != nil {
		return result, err
	}
	plan := BackupRestorePlan{ID: id, Phase: phase, Added: []string{}, Replaced: []string{}, Removed: []string{}}
	liveSet := map[string]bool{}
	for _, name := range live {
		liveSet[name] = true
		if _, ok := values[name]; !ok {
			values[name] = nil
			plan.Removed = append(plan.Removed, name)
		}
	}
	for name, ciphertext := range values {
		if ciphertext == nil {
			continue
		}
		if liveSet[name] {
			plan.Replaced = append(plan.Replaced, name)
		} else {
			plan.Added = append(plan.Added, name)
		}
	}
	sort.Strings(plan.Added)
	sort.Strings(plan.Replaced)
	sort.Strings(plan.Removed)
	if confirm != nil {
		if err := confirm(plan); err != nil {
			return result, err
		}
	}
	owned = false
	return s.mutate(lock, "backup restore", values, nil)
}

type BackupSummary struct {
	Count            int     `json:"count"`
	Bytes            int64   `json:"bytes"`
	Oldest           string  `json:"oldest,omitempty"`
	Newest           string  `json:"newest,omitempty"`
	OldestAgeSeconds float64 `json:"oldest_age_seconds"`
}

func (s *Store) BackupSummary() (BackupSummary, error) {
	result := BackupSummary{}
	backups, err := s.Backups()
	if err != nil {
		return result, err
	}
	result.Count = len(backups)
	for _, backup := range backups {
		result.Bytes += backup.Bytes
	}
	if len(backups) > 0 {
		result.Newest = backups[0].Started
		result.Oldest = backups[len(backups)-1].Started
		started, _ := time.Parse(time.RFC3339Nano, result.Oldest)
		result.OldestAgeSeconds = time.Since(started).Seconds()
	}
	return result, nil
}
