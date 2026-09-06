package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

type RotationChange struct {
	Path   string `json:"path"`
	Before string `json:"before"`
	After  string `json:"after"`
}
type Rotation struct {
	Version        int              `json:"version"`
	ID             string           `json:"id"`
	Phase          string           `json:"phase"`
	Started        string           `json:"started"`
	OldFingerprint string           `json:"old_fingerprint"`
	NewFingerprint string           `json:"new_fingerprint"`
	Retired        string           `json:"retired"`
	Destroy        bool             `json:"destroy"`
	Compromise     bool             `json:"compromise"`
	GitBefore      string           `json:"git_before,omitempty"`
	Changes        []RotationChange `json:"changes"`
	Names          []string         `json:"names"`
}
type RotationResult struct {
	Transaction     string   `json:"transaction"`
	OldFingerprint  string   `json:"old_fingerprint"`
	NewFingerprint  string   `json:"new_fingerprint"`
	RetiredArtifact string   `json:"retired_artifact,omitempty"`
	Destroyed       bool     `json:"destroyed"`
	AffectedEntries []string `json:"affected_entries"`
	Warning         string   `json:"warning"`
}

func (s *Store) Rotate(destroy bool, ack string, compromise bool) (RotationResult, error) {
	return s.rotate(destroy, ack, compromise, nil)
}

func (s *Store) rotate(destroy bool, ack string, compromise bool, hook func(string) error) (result RotationResult, err error) {
	if err := s.RequireDomain("identity"); err != nil {
		return result, err
	}
	old, err := s.IdentityShow()
	if err != nil {
		return result, err
	}
	if destroy && ack != "destroy-retired-key:"+old.Fingerprint {
		return result, fault.Interaction("destructive rotation requires --acknowledge destroy-retired-key:OLD_FINGERPRINT")
	}
	if destroy {
		if err := s.requireCleanRetirementStaging(""); err != nil {
			return result, err
		}
	}
	lock, err := s.Lock("identity rotate")
	if err != nil {
		return result, err
	}
	pending := false
	dir := ""
	ownedStage := ""
	defer func() {
		if !pending {
			err = s.finishUnpublished(lock, ownedStage, err)
		}
	}()
	if destroy {
		// Recheck under the shared lock before reading private identities or
		// creating this rotation's stage. An earlier abandoned rotation can
		// retain a new private identity together with a sealed copy of the old one.
		if err := s.requireCleanRetirementStaging(""); err != nil {
			return result, err
		}
	}
	if _, err := s.CleanGit(); err != nil {
		return result, err
	}
	if err := s.DeepVerify(); err != nil {
		return result, err
	}
	current, err := s.IdentityShow()
	if err != nil {
		return result, err
	}
	if current.Fingerprint != old.Fingerprint {
		return result, fault.New("identity.changed", "identity changed before rotation lock acquisition")
	}
	ids, _, err := s.Keys()
	if err != nil {
		return result, err
	}
	private, public, err := crypt.Generate()
	if err != nil {
		return result, err
	}
	newIDs, err := crypt.Identities([]byte(private), s.UI)
	if err != nil {
		return result, err
	}
	newRecipients, err := crypt.Recipients([]byte(public), s.UI)
	if err != nil {
		return result, err
	}
	id := securefs.ID()
	dir = metadata + "/transactions/" + id
	if err := s.Root.Mkdir(dir, 0o700); err != nil {
		return result, err
	}
	ownedStage = dir
	if err := s.bindStaging(lock, id); err != nil {
		return result, err
	}
	j := Rotation{Version: 1, ID: id, Phase: "prepared", Started: time.Now().UTC().Format(time.RFC3339Nano), OldFingerprint: old.Fingerprint, NewFingerprint: crypt.Fingerprint(public), Retired: metadata + "/retired/" + id + ".age", Destroy: destroy, Compromise: compromise, Changes: []RotationChange{}}
	if enabled, _ := s.GitEnabled(); enabled {
		j.GitBefore, err = s.Head()
		if err != nil {
			return result, err
		}
	}
	j.Names, err = s.Names()
	if err != nil {
		return result, err
	}
	stage := func(p string, before, after []byte) error {
		if err := s.Root.MkdirAll(dir+"/after/"+path.Dir(p), 0o700); err != nil {
			return err
		}
		if err := securefs.WriteNew(s.Root, dir+"/after/"+p, after); err != nil {
			return err
		}
		beforeHash := ""
		if before != nil {
			beforeHash = digest(before)
		}
		j.Changes = append(j.Changes, RotationChange{Path: p, Before: beforeHash, After: digest(after)})
		return nil
	}
	for _, name := range j.Names {
		c, err := s.Ciphertext(name)
		if err != nil {
			return result, err
		}
		plain, err := crypt.Decrypt(c, ids)
		if err != nil {
			return result, err
		}
		newCiphertext, err := crypt.Encrypt(plain, newRecipients)
		if err != nil {
			return result, err
		}
		if _, err := crypt.Decrypt(newCiphertext, newIDs); err != nil {
			return result, err
		}
		p, _ := EntryPath(name)
		if err := stage(p, c, newCiphertext); err != nil {
			return result, err
		}
	}
	oldPrivate, err := securefs.Read(s.Root, "identities", maxMetadata)
	if err != nil {
		return result, err
	}
	sealed, err := crypt.Encrypt(oldPrivate, newRecipients)
	if err != nil {
		return result, err
	}
	if _, err := crypt.Decrypt(sealed, newIDs); err != nil {
		return result, err
	}
	if err := stage(j.Retired, nil, sealed); err != nil {
		return result, err
	}
	oldPublic, err := securefs.Read(s.Root, "recipients", maxMetadata)
	if err != nil {
		return result, err
	}
	if err := stage("recipients", oldPublic, []byte(public)); err != nil {
		return result, err
	}
	if err := stage("identities", oldPrivate, []byte(private)); err != nil {
		return result, err
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
	pending, err = securefs.PublishNewPublished(s.Root, metadata+"/rotation.json", data)
	if pending && err == nil && hook != nil {
		err = hook("journaled")
	}
	if err != nil {
		if pending {
			return result, publishedJournalFailure(id)
		}
		return result, err
	}
	if err := s.finishRotation(&j, hook); err != nil {
		return result, fault.Applied("identity rotation requires explicit recovery; shared lock retained", id)
	}
	result = RotationResult{Transaction: id, OldFingerprint: j.OldFingerprint, NewFingerprint: j.NewFingerprint, RetiredArtifact: j.Retired, Destroyed: destroy, AffectedEntries: []string{}, Warning: "Peers must explicitly rotate their pins. External copies of old keys and ciphertext cannot be revoked."}
	if destroy {
		result.RetiredArtifact = ""
	}
	if compromise {
		result.AffectedEntries = j.Names
		result.Warning += " Re-encryption does not rotate underlying credentials; rotate the listed passwords and tokens separately."
	}
	if err := lock.Release(); err != nil {
		return result, fault.Applied("rotation finalized but lock release failed", id)
	}
	return result, nil
}

func (s *Store) requireCleanRetirementStaging(ownedID string) error {
	staging, err := s.inspectStaging()
	if err != nil {
		return err
	}
	other := []string{}
	for _, dir := range staging {
		if ownedID == "" || dir != metadata+"/transactions/"+ownedID {
			other = append(other, dir)
		}
	}
	if len(other) != 0 {
		err := fault.New("identity.staging_present", "inspect leftover key or transaction staging with fulla doctor before destructive identity rotation")
		err.Details["staging"] = other
		if ownedID == "" {
			err.Details["applied"] = false
		}
		return err
	}
	return nil
}

func (s *Store) rotationWrite(j *Rotation) error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return securefs.Replace(s.Root, metadata+"/rotation.json", data)
}

func (s *Store) finishRotation(j *Rotation, hook func(string) error) error {
	if j.Version != 1 || !validID(j.ID) || j.Retired != metadata+"/retired/"+j.ID+".age" {
		return fault.New("identity.invalid_rotation", "invalid rotation journal")
	}
	if j.Phase != "prepared" && j.Phase != "published" && j.Phase != "committed" && j.Phase != "retired" {
		return fault.New("identity.invalid_rotation", "unknown rotation phase")
	}
	if j.Destroy {
		// Recovery may be resuming a journal written by an older binary. Only
		// this validated journal's own stage is covered by its cleanup protocol.
		if err := s.requireCleanRetirementStaging(j.ID); err != nil {
			return err
		}
	}
	dir := metadata + "/transactions/" + j.ID
	if j.Phase == "prepared" {
		seen := map[string]bool{}
		for _, c := range j.Changes {
			if seen[c.Path] {
				return fault.New("identity.invalid_rotation", "duplicate rotation path")
			}
			seen[c.Path] = true
			if c.Path != "identities" && c.Path != "recipients" && c.Path != j.Retired {
				if !strings.HasPrefix(c.Path, "passwords/") || !strings.HasSuffix(c.Path, ".age") {
					return fault.New("identity.invalid_rotation", "invalid rotation target")
				}
				name := strings.TrimSuffix(strings.TrimPrefix(c.Path, "passwords/"), ".age")
				if _, err := EntryPath(name); err != nil {
					return err
				}
			}
			newData, err := securefs.Read(s.Root, dir+"/after/"+c.Path, 65<<20)
			if err != nil || digest(newData) != c.After {
				return fault.New("identity.corrupt_rotation", "staged rotation data mismatch")
			}
			oldData, err := securefs.Read(s.Root, c.Path, 65<<20)
			current := ""
			if err == nil {
				current = digest(oldData)
			} else if !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			if current != c.Before && current != c.After {
				return fault.New("identity.rotation_conflict", "rotation target changed outside the journal")
			}
		}
		if !seen["identities"] || !seen["recipients"] || !seen[j.Retired] {
			return fault.New("identity.invalid_rotation", "rotation journal lacks identity material")
		}
		for _, c := range j.Changes {
			oldData, err := securefs.Read(s.Root, c.Path, 65<<20)
			if err == nil && digest(oldData) == c.After {
				if err := s.syncRotationPublication(dir, c.Path); err != nil {
					return err
				}
				continue
			}
			data, err := securefs.Read(s.Root, dir+"/after/"+c.Path, 65<<20)
			if err != nil {
				return err
			}
			err = s.publishRotationFile(dir, c, data, hook)
			if err != nil {
				return err
			}
			if hook != nil {
				if err := hook("published:" + c.Path); err != nil {
					return err
				}
			}
		}
		j.Phase = "published"
		if err := s.rotationWrite(j); err != nil {
			return err
		}
	}
	if j.Phase == "published" {
		if err := s.DeepVerify(); err != nil {
			return err
		}
		if enabled, _ := s.GitEnabled(); enabled {
			head, err := s.Head()
			if err != nil {
				return err
			}
			message := "Fulla identity rotate\n\nfulla-transaction: " + j.ID
			if head == j.GitBefore {
				if err := s.Commit(j.Names, message); err != nil {
					return err
				}
			} else {
				out, err := s.Git("log", "-1", "--format=%B")
				if err != nil {
					return err
				}
				if strings.TrimSpace(string(out)) != message {
					return fault.New("identity.rotation_conflict", "Git changed outside rotation")
				}
			}
		}
		if hook != nil {
			if err := hook("committed"); err != nil {
				return err
			}
		}
		j.Phase = "committed"
		if err := s.rotationWrite(j); err != nil {
			return err
		}
	}
	if j.Phase == "committed" {
		if err := s.DeepVerify(); err != nil {
			return err
		}
		if j.Destroy {
			if err := s.Root.Remove(j.Retired); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			// Remove the staged sealed copy as well; retaining it would defeat
			// the user's explicit local retired-key destruction acknowledgement.
			if err := s.Root.Remove(dir + "/after/" + j.Retired); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			if err := securefs.SyncDir(s.Root, metadata+"/retired"); err != nil {
				return err
			}
		}
		j.Phase = "retired"
		if err := s.rotationWrite(j); err != nil {
			return err
		}
	}
	if err := s.DeepVerify(); err != nil {
		return err
	}
	receipt := metadata + "/receipts/" + j.ID + ".json"
	if _, err := s.Root.Lstat(receipt); errors.Is(err, fs.ErrNotExist) {
		data, _ := json.Marshal(map[string]any{"version": 1, "command": "identity rotate", "transaction": j.ID, "old_fingerprint": j.OldFingerprint, "new_fingerprint": j.NewFingerprint, "retired": j.Retired, "destroyed": j.Destroy, "at": j.Started, "applied": true})
		if err := securefs.PublishNew(s.Root, receipt, data); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	// Staging contains the new private identity. Keep the recovery journal
	// until its removal is durable, so a crash cannot orphan a key copy that
	// survives later explicit retired-key destruction. The retired phase can
	// finish without staging and therefore safely repeat this cleanup.
	if err := s.Root.RemoveAll(dir); err != nil {
		return err
	}
	if err := securefs.SyncDir(s.Root, metadata+"/transactions"); err != nil {
		return err
	}
	if hook != nil {
		if err := hook("cleaned"); err != nil {
			return err
		}
	}
	if err := s.Root.Remove(metadata + "/rotation.json"); err != nil {
		return err
	}
	if err := securefs.SyncDir(s.Root, metadata); err != nil {
		return err
	}
	return nil
}
