package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"path"
	"sort"
	"time"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

type Change struct {
	Name   string `json:"name"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type Journal struct {
	Version   int      `json:"version"`
	ID        string   `json:"id"`
	Command   string   `json:"command"`
	Started   string   `json:"started"`
	Phase     string   `json:"phase"`
	GitBefore string   `json:"git_before,omitempty"`
	Changes   []Change `json:"changes"`
}

type MutationResult struct {
	Transaction string   `json:"transaction"`
	Names       []string `json:"names"`
	Applied     bool     `json:"applied"`
	Receipt     string   `json:"receipt"`
	Backup      string   `json:"backup"`
}

func digest(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }

// mutate is called with an owned lock after semantic validation. Values are
// already encrypted; nil means deletion, never an empty plaintext value.
// hook is an internal failure-injection seam, never a user environment switch.
func (s *Store) mutate(lock *Lock, command string, values map[string][]byte, hook func(string) error) (result MutationResult, err error) {
	if err := s.RequireDomain("transactions"); err != nil {
		_ = lock.Release()
		return result, err
	}
	prospective := make([]string, 0, len(values))
	for name := range values {
		prospective = append(prospective, name)
	}
	if err := s.checkGitConversions(prospective); err != nil {
		_ = lock.Release()
		return result, err
	}
	if _, err := s.CleanGit(); err != nil {
		_ = lock.Release()
		return result, err
	}
	if len(values) == 0 {
		err = lock.Release()
		result.Names = []string{}
		return result, err
	}
	id := securefs.ID()
	dir := metadata + "/transactions/" + id
	pending := false
	defer func() {
		if !pending {
			_ = s.Root.RemoveAll(dir)
			releaseErr := lock.Release()
			if err == nil {
				err = releaseErr
			}
		}
	}()
	if err := s.Root.Mkdir(dir, 0o700); err != nil {
		return result, err
	}
	for _, sub := range []string{"before", "after"} {
		if err := s.Root.Mkdir(dir+"/"+sub, 0o700); err != nil {
			return result, err
		}
	}
	j := Journal{Version: 1, ID: id, Command: command, Started: time.Now().UTC().Format(time.RFC3339Nano), Phase: "prepared", Changes: []Change{}}
	if enabled, _ := s.GitEnabled(); enabled {
		j.GitBefore, err = s.Head()
		if err != nil {
			return result, err
		}
	}
	if err := copyTree(s.Root, "passwords", dir+"/before/passwords"); err != nil {
		return result, err
	}
	names := []string{}
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p, err := EntryPath(name)
		if err != nil {
			return result, err
		}
		change := Change{Name: name}
		old, err := s.Ciphertext(name)
		if err == nil {
			change.Before = digest(old)
		} else {
			exists, e := s.Exists(name)
			if e != nil {
				return result, e
			}
			if exists {
				return result, err
			}
		}
		if value := values[name]; value != nil {
			change.After = digest(value)
			if err := s.Root.MkdirAll(dir+"/after/"+path.Dir(p), 0o700); err != nil {
				return result, err
			}
			if err := securefs.WriteNew(s.Root, dir+"/after/"+p, value); err != nil {
				return result, err
			}
		}
		j.Changes = append(j.Changes, change)
	}
	if hook != nil {
		if err := hook("staged"); err != nil {
			return result, err
		}
	}
	data, err := json.Marshal(j)
	if err != nil {
		return result, err
	}
	if err := securefs.WriteNew(s.Root, dir+"/journal.json", data); err != nil {
		return result, err
	}
	if err := securefs.PublishNew(s.Root, metadata+"/pending.json", data); err != nil {
		return result, err
	}
	pending = true
	result = MutationResult{Transaction: id, Names: names, Receipt: metadata + "/receipts/" + id + ".json", Backup: metadata + "/backups/" + id}
	if err := s.finishJournal(&j, hook); err != nil {
		return result, fault.Applied("transaction requires explicit recovery; shared lock retained", id)
	}
	result.Applied = true
	if err := lock.Release(); err != nil {
		return result, fault.Applied("transaction finalized but lock release failed", id)
	}
	return result, nil
}

func (s *Store) journalWrite(j *Journal) error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return securefs.Replace(s.Root, metadata+"/pending.json", data)
}

func (s *Store) finishJournal(j *Journal, hook func(string) error) error {
	if j.Version != 1 || !validID(j.ID) {
		return fault.New("transaction.invalid", "invalid pending transaction")
	}
	if j.Phase != "prepared" && j.Phase != "published" && j.Phase != "committed" {
		return fault.New("transaction.invalid", "unknown journal phase")
	}
	dir := metadata + "/transactions/" + j.ID
	if j.Phase == "prepared" {
		// Validate all old/new hashes before publishing the first path. A retry
		// accepts only old or staged-new state, never unrelated external edits.
		seen := map[string]bool{}
		for _, c := range j.Changes {
			if seen[c.Name] {
				return fault.New("transaction.invalid", "duplicate journal change")
			}
			seen[c.Name] = true
			p, err := EntryPath(c.Name)
			if err != nil {
				return err
			}
			current, err := s.currentDigest(c.Name)
			if err != nil {
				return err
			}
			if current != c.Before && current != c.After {
				return fault.New("transaction.conflict", "live ciphertext differs from both journal states")
			}
			if c.After != "" {
				data, err := securefs.Read(s.Root, dir+"/after/"+p, 65<<20)
				if err != nil || digest(data) != c.After {
					return fault.New("transaction.corrupt", "staged ciphertext failed verification")
				}
			}
			if c.Before != "" {
				data, err := securefs.Read(s.Root, dir+"/before/"+p, 65<<20)
				if err != nil || digest(data) != c.Before {
					return fault.New("transaction.corrupt", "backup ciphertext failed verification")
				}
			}
		}
		for _, c := range j.Changes {
			p, _ := EntryPath(c.Name)
			current, err := s.currentDigest(c.Name)
			if err != nil {
				return err
			}
			if current == c.After {
				continue
			}
			if c.After == "" {
				if err := s.Root.Remove(p); err != nil {
					return err
				}
				if err := securefs.SyncDir(s.Root, path.Dir(p)); err != nil {
					return err
				}
			} else {
				data, err := securefs.Read(s.Root, dir+"/after/"+p, 65<<20)
				if err != nil {
					return err
				}
				if err := s.Root.MkdirAll(path.Dir(p), 0o700); err != nil {
					return err
				}
				if c.Before == "" {
					if err := securefs.PublishNew(s.Root, p, data); err != nil {
						return err
					}
				} else {
					if err := securefs.Replace(s.Root, p, data); err != nil {
						return err
					}
				}
			}
			if hook != nil {
				if err := hook("published:" + c.Name); err != nil {
					return err
				}
			}
		}
		j.Phase = "published"
		if err := s.journalWrite(j); err != nil {
			return err
		}
	}
	if j.Phase == "published" {
		if err := s.verifyJournalLive(j); err != nil {
			return err
		}
		if enabled, _ := s.GitEnabled(); enabled {
			head, err := s.Head()
			if err != nil {
				return err
			}
			message := "Fulla " + j.Command + "\n\nfulla-transaction: " + j.ID
			if head != j.GitBefore {
				out, err := s.Git("log", "-1", "--format=%B")
				if err != nil {
					return err
				}
				if string(out) != message+"\n\n" && string(out) != message+"\n" {
					return fault.New("transaction.conflict", "Git HEAD changed outside the pending transaction")
				}
			} else {
				names := []string{}
				for _, c := range j.Changes {
					names = append(names, c.Name)
				}
				if err := s.Commit(names, message); err != nil {
					return err
				}
			}
		}
		if hook != nil {
			if err := hook("committed"); err != nil {
				return err
			}
		}
		j.Phase = "committed"
		if err := s.journalWrite(j); err != nil {
			return err
		}
	}
	if err := s.verifyJournalLive(j); err != nil {
		return err
	}
	// Backups retain encrypted old/new material and a complete journal. Copying
	// preserves staged evidence if final receipt creation is interrupted.
	backup := metadata + "/backups/" + j.ID
	if _, err := s.Root.Lstat(backup); errors.Is(err, fs.ErrNotExist) {
		stage := metadata + "/backups/.fulla-stage-" + j.ID
		if err := s.Root.RemoveAll(stage); err != nil {
			return err
		}
		if err := copyTree(s.Root, dir, stage); err != nil {
			return err
		}
		if err := s.Root.RemoveAll(stage + "/after"); err != nil {
			return err
		}
		if err := s.Root.Mkdir(stage+"/after", 0o700); err != nil {
			return err
		}
		if err := copyTree(s.Root, "passwords", stage+"/after/passwords"); err != nil {
			return err
		}
		finalJournal, err := json.Marshal(j)
		if err != nil {
			return err
		}
		if err := securefs.Replace(s.Root, stage+"/journal.json", finalJournal); err != nil {
			return err
		}
		parent, err := s.Root.OpenRoot(metadata + "/backups")
		if err != nil {
			return err
		}
		err = securefs.RenameNew(parent, path.Base(stage), j.ID)
		parent.Close()
		if err != nil {
			return err
		}
		if err := securefs.SyncDir(s.Root, metadata+"/backups"); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	receipt := metadata + "/receipts/" + j.ID + ".json"
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	if _, err := s.Root.Lstat(receipt); errors.Is(err, fs.ErrNotExist) {
		if err := securefs.PublishNew(s.Root, receipt, data); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if hook != nil {
		if err := hook("receipted"); err != nil {
			return err
		}
	}
	if err := s.Root.Remove(metadata + "/pending.json"); err != nil {
		return err
	}
	if err := securefs.SyncDir(s.Root, metadata); err != nil {
		return err
	}
	return s.Root.RemoveAll(dir)
}

func (s *Store) currentDigest(name string) (string, error) {
	exists, err := s.Exists(name)
	if err != nil || !exists {
		return "", err
	}
	data, err := s.Ciphertext(name)
	if err != nil {
		return "", err
	}
	return digest(data), nil
}
func (s *Store) verifyJournalLive(j *Journal) error {
	for _, c := range j.Changes {
		d, err := s.currentDigest(c.Name)
		if err != nil {
			return err
		}
		if d != c.After {
			return fault.New("transaction.conflict", "published ciphertext does not match journal")
		}
	}
	return nil
}
func validID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
