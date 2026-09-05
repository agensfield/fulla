package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"time"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

type Retention struct {
	// nil means no count criterion. When both criteria are set, both must select
	// a backup before it can be deleted.
	Keep      *int
	OlderThan time.Duration
}

type PruneResult struct {
	DryRun      bool     `json:"dry_run"`
	Candidates  []Backup `json:"candidates"`
	Retained    int      `json:"retained"`
	Bytes       int64    `json:"bytes"`
	Deleted     []string `json:"deleted"`
	Transaction string   `json:"transaction,omitempty"`
	Receipt     string   `json:"receipt,omitempty"`
}

type pruneJournal struct {
	Version  int      `json:"version"`
	ID       string   `json:"id"`
	Started  string   `json:"started"`
	Selected []Backup `json:"selected"`
	Done     int      `json:"done"`
}

func (s *Store) Prune(retention Retention, dryRun bool) (PruneResult, error) {
	return s.prune(retention, dryRun, nil)
}

func (s *Store) prune(retention Retention, dryRun bool, hook func(string) error) (result PruneResult, err error) {
	result = PruneResult{DryRun: dryRun, Candidates: []Backup{}, Deleted: []string{}}
	if (retention.Keep != nil && *retention.Keep < 0) || retention.OlderThan < 0 || (retention.Keep == nil && retention.OlderThan == 0) {
		return result, fault.Usage("prune requires --keep N or a positive --older-than duration")
	}
	if err := s.RequireDomain("backup"); err != nil {
		return result, err
	}
	if err := s.Unlocked(); err != nil {
		return result, err
	}
	var lock *Lock
	if !dryRun {
		lock, err = s.Lock("backup prune")
		if err != nil {
			return result, err
		}
		defer func() {
			// A published journal owns the lock until completion or explicit recovery,
			// including a publication syscall that returned an ambiguous fsync error.
			if _, e := s.Root.Lstat(metadata + "/prune.json"); errors.Is(e, fs.ErrNotExist) {
				if e := lock.Release(); e != nil && err == nil {
					err = fault.Applied("pruning finished but lock release failed", result.Transaction)
				}
			}
		}()
	}
	backups, err := s.Backups()
	if err != nil {
		return result, err
	}
	cutoff := time.Now().Add(-retention.OlderThan)
	for rank, backup := range backups {
		started, e := time.Parse(time.RFC3339Nano, backup.Started)
		if e != nil {
			return result, fault.New("backup.invalid", "backup timestamp is invalid")
		}
		selected := (retention.Keep == nil || rank >= *retention.Keep) && (retention.OlderThan == 0 || started.Before(cutoff))
		if selected {
			result.Candidates = append(result.Candidates, backup)
			result.Bytes += backup.Bytes
		} else {
			result.Retained++
		}
	}
	if dryRun || len(result.Candidates) == 0 {
		return result, nil
	}
	j := pruneJournal{Version: 1, ID: securefs.ID(), Started: time.Now().UTC().Format(time.RFC3339Nano), Selected: result.Candidates}
	data, err := json.Marshal(j)
	if err != nil {
		return result, err
	}
	if len(data) > maxMetadata-1024 {
		return result, fault.New("backup.too_many", "prune selection exceeds the journal limit; select fewer backups")
	}
	result.Transaction = j.ID
	result.Receipt = metadata + "/receipts/" + j.ID + ".json"
	if err := securefs.PublishNew(s.Root, metadata+"/prune.json", data); err != nil {
		if _, e := s.Root.Lstat(metadata + "/prune.json"); e == nil {
			return result, fault.Applied("prune journal publication requires inspection; shared lock retained", j.ID)
		}
		return result, err
	}
	if hook != nil {
		if err := hook("prepared"); err != nil {
			return result, fault.Applied("prune requires recovery; shared lock retained", j.ID)
		}
	}
	if err := s.finishPrune(&j, hook); err != nil {
		return result, fault.Applied("prune requires recovery; shared lock retained", j.ID)
	}
	for _, backup := range j.Selected {
		result.Deleted = append(result.Deleted, backup.ID)
	}
	return result, nil
}

func (s *Store) finishPrune(j *pruneJournal, hook func(string) error) error {
	if err := s.RequireDomain("backup"); err != nil {
		return err
	}
	if j.Version != 1 || !validID(j.ID) || len(j.Selected) == 0 || j.Done < 0 || j.Done > len(j.Selected) {
		return fault.New("backup.invalid", "invalid prune journal")
	}
	if _, err := time.Parse(time.RFC3339Nano, j.Started); err != nil {
		return fault.New("backup.invalid", "invalid prune timestamp")
	}
	seen := map[string]bool{}
	for index, backup := range j.Selected {
		if !validID(backup.ID) || seen[backup.ID] || backup.Bytes < 0 {
			return fault.New("backup.invalid", "invalid prune selection")
		}
		seen[backup.ID] = true
		if index < j.Done {
			if _, err := s.Root.Lstat(metadata + "/backups/" + backup.ID); err == nil {
				return fault.New("backup.conflict", "completed prune selection reappeared")
			} else if !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}
	for j.Done < len(j.Selected) {
		backup := j.Selected[j.Done]
		// RemoveAll is intentionally idempotent: a process can die after any file
		// unlink, including removal of the snapshot's own journal.
		if err := s.Root.RemoveAll(metadata + "/backups/" + backup.ID); err != nil {
			return err
		}
		if err := securefs.SyncDir(s.Root, metadata+"/backups"); err != nil {
			return err
		}
		if hook != nil {
			if err := hook("removed"); err != nil {
				return err
			}
		}
		j.Done++
		data, err := json.Marshal(j)
		if err != nil {
			return err
		}
		if err := securefs.Replace(s.Root, metadata+"/prune.json", data); err != nil {
			return err
		}
	}
	receipt, err := json.Marshal(map[string]any{"version": 1, "command": "backup prune", "prune": j})
	if err != nil {
		return err
	}
	file := metadata + "/receipts/" + j.ID + ".json"
	existing, err := securefs.Read(s.Root, file, maxMetadata)
	if errors.Is(err, fs.ErrNotExist) {
		if err := securefs.PublishNew(s.Root, file, receipt); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if !bytes.Equal(existing, receipt) {
		return fault.New("backup.conflict", "prune receipt conflicts with existing evidence")
	}
	if hook != nil {
		if err := hook("receipted"); err != nil {
			return err
		}
	}
	if err := s.Root.Remove(metadata + "/prune.json"); err != nil {
		return err
	}
	return securefs.SyncDir(s.Root, metadata)
}
