package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

type LockInfo struct {
	Token       string `json:"token"`
	Operation   string `json:"operation"`
	PeerReceipt string `json:"peer_receipt,omitempty"`
	StageID     string `json:"stage_id,omitempty"`
	AdoptionID  string `json:"adoption_id,omitempty"`
	PID         int    `json:"pid"`
	Host        string `json:"host"`
	Alive       bool   `json:"alive"`
	Local       bool   `json:"local"`
}

func (s *Store) InspectLock() (*LockInfo, error) {
	owner, err := securefs.Read(s.Root, "lock/owner", 256)
	if errors.Is(err, fs.ErrNotExist) {
		if _, statErr := s.Root.Lstat("lock"); errors.Is(statErr, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fault.New("store.lock_unknown", "shared lock exists without a complete owner record")
	}
	if err != nil {
		return nil, err
	}
	data, err := securefs.Read(s.Root, "lock/info", 4096)
	if err != nil {
		return nil, fault.New("store.lock_unknown", "lock owner information is incomplete; manual inspection required")
	}
	info := &LockInfo{Token: strings.TrimSpace(string(owner))}
	for _, word := range strings.Fields(string(data)) {
		key, value, ok := strings.Cut(word, "=")
		if !ok {
			continue
		}
		switch key {
		case "pid":
			info.PID, _ = strconv.Atoi(value)
		case "host":
			info.Host = value
		case "operation":
			info.Operation = value
		case "peer_receipt":
			if info.PeerReceipt != "" || !validID(value) {
				return nil, fault.New("store.lock_unknown", "invalid peer receipt binding")
			}
			info.PeerReceipt = value
		case "stage_id":
			if info.StageID != "" || !validID(value) {
				return nil, fault.New("store.lock_unknown", "invalid staging binding")
			}
			info.StageID = value
		case "adoption_id":
			if info.AdoptionID != "" || !validID(value) {
				return nil, fault.New("store.lock_unknown", "invalid adoption binding")
			}
			info.AdoptionID = value
		}
	}
	if (info.StageID != "" && info.PeerReceipt != "") || (info.AdoptionID != "" && (info.StageID != "" || info.PeerReceipt != "")) {
		return nil, fault.New("store.lock_unknown", "conflicting recovery bindings")
	}
	if info.PID <= 0 || info.Token == "" || info.Host == "" {
		return nil, fault.New("store.lock_unknown", "cannot prove lock ownership")
	}
	host, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	info.Local = host == info.Host
	if info.Local {
		err := syscall.Kill(info.PID, 0)
		info.Alive = !errors.Is(err, syscall.ESRCH)
	}
	return info, nil
}

func (s *Store) Recover(expected string) (map[string]any, error) {
	if _, err := s.Root.Lstat(metadata); errors.Is(err, fs.ErrNotExist) {
		info, err := s.InspectLock()
		if err != nil {
			return nil, err
		}
		if info == nil || info.AdoptionID == "" {
			return nil, fault.New("store.uninitialized", "unadopted recovery requires an explicit adoption binding")
		}
	} else if err != nil {
		return nil, err
	}
	return s.recover(expected, s.Validate)
}

func (s *Store) recover(expected string, validate func() error) (map[string]any, error) {
	if expected == "" {
		return nil, fault.Interaction("recovery requires --recover-lock with the inspected owner token")
	}
	info, err := s.InspectLock()
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, fault.New("store.not_locked", "no stale shared lock exists")
	}
	if info.Token != expected {
		return nil, fault.New("store.lock_changed", "lock token differs from expected owner")
	}
	if !info.Local || info.Alive {
		return nil, fault.New("store.lock_active", "refusing live, remote, or unverifiable lock owner")
	}
	guard, err := s.Root.OpenFile("lock/recovery", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	defer guard.Close()
	if err := unix.Flock(int(guard.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return nil, fault.New("store.locked", "another recovery owns this lock")
	}
	again, err := s.InspectLock()
	if err != nil {
		return nil, err
	}
	if again == nil || again.Token != expected || again.Alive || !again.Local {
		return nil, fault.New("store.lock_changed", "lock changed during recovery preflight")
	}
	if err := validate(); err != nil {
		return nil, err
	}
	count := 0
	if info.AdoptionID != "" {
		count++
		if _, err := s.adoptionPublished(info.AdoptionID); err != nil {
			return nil, err
		}
	}
	if info.PeerReceipt != "" {
		count++
	}
	for _, name := range []string{"pending.json", "rotation.json", "prune.json"} {
		if _, err := s.Root.Lstat(metadata + "/" + name); err == nil {
			count++
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	if count > 1 {
		return nil, fault.New("transaction.conflict", "multiple pending journals require inspection")
	}
	if err := s.validateStagingBinding(info.StageID); err != nil {
		return nil, err
	}
	// Publish this invocation as owner before touching the journal. flock is
	// released by the kernel on process death, so interrupted recovery retries.
	operation := "recover"
	if info.Operation == "fix-permissions" {
		operation = info.Operation
	}
	newInfo := fmt.Sprintf("pid=%d host=%s operation=%s started=%s\n", os.Getpid(), info.Host, operation, time.Now().UTC().Format(time.RFC3339))
	if info.PeerReceipt != "" {
		newInfo = strings.TrimSpace(newInfo) + " peer_receipt=" + info.PeerReceipt + "\n"
	}
	if info.StageID != "" {
		newInfo = strings.TrimSpace(newInfo) + " stage_id=" + info.StageID + "\n"
	}
	if info.AdoptionID != "" {
		newInfo = strings.TrimSpace(newInfo) + " adoption_id=" + info.AdoptionID + "\n"
	}
	if err := securefs.Replace(s.Root, "lock/info", []byte(newInfo)); err != nil {
		return nil, err
	}
	newToken := securefs.ID()
	if err := securefs.Replace(s.Root, "lock/owner", []byte(newToken+"\n")); err != nil {
		return nil, err
	}
	expected = newToken
	if info.AdoptionID != "" {
		return s.recoverAdoption(info.AdoptionID, expected)
	}
	pruneData, pruneErr := securefs.Read(s.Root, metadata+"/prune.json", maxMetadata)
	if pruneErr == nil {
		var j pruneJournal
		if err := StrictJSON(pruneData, &j); err != nil {
			return nil, err
		}
		if err := s.finishPrune(&j, nil); err != nil {
			return nil, fault.Applied("backup prune recovery incomplete; lock retained", j.ID)
		}
		if err := s.Root.Remove("lock/recovery"); err != nil {
			return nil, err
		}
		l := &Lock{store: s, Token: expected, held: true}
		if err := l.Release(); err != nil {
			return nil, fault.Applied("prune recovered but lock release failed", j.ID)
		}
		return map[string]any{"recovered": true, "transaction": j.ID, "lock_released": true}, nil
	}
	if !errors.Is(pruneErr, fs.ErrNotExist) {
		return nil, pruneErr
	}
	data, err := securefs.Read(s.Root, metadata+"/pending.json", maxMetadata)
	if errors.Is(err, fs.ErrNotExist) {
		rotationData, rotationErr := securefs.Read(s.Root, metadata+"/rotation.json", maxMetadata)
		if rotationErr == nil {
			if err := s.RequireDomain("identity"); err != nil {
				return nil, err
			}
			var rotation Rotation
			if err := StrictJSON(rotationData, &rotation); err != nil {
				return nil, err
			}
			if err := s.finishRotation(&rotation, nil); err != nil {
				return nil, fault.Applied("identity rotation recovery incomplete; lock retained", rotation.ID)
			}
			if err := s.Root.Remove("lock/recovery"); err != nil {
				return nil, err
			}
			l := &Lock{store: s, Token: expected, held: true}
			if err := l.Release(); err != nil {
				return nil, fault.Applied("rotation recovered but lock release failed", rotation.ID)
			}
			return map[string]any{"recovered": true, "transaction": rotation.ID, "lock_released": true}, nil
		}
		if !errors.Is(rotationErr, fs.ErrNotExist) {
			return nil, rotationErr
		}
		if info.PeerReceipt != "" {
			if err := s.reconcilePeerReceipt(info.PeerReceipt); err != nil {
				return nil, err
			}
		}
		if info.StageID != "" {
			if err := s.RequireDomain("transactions"); err != nil {
				return nil, err
			}
			dir := metadata + "/transactions/" + info.StageID
			if entry, err := s.Root.Lstat(dir); err == nil {
				if !entry.IsDir() {
					return nil, fault.New("transaction.invalid_staging", "bound staging is not a directory; recovery lock retained")
				}
			} else if !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
			if err := s.Root.RemoveAll(dir); err != nil {
				return nil, fault.New("transaction.cleanup_failed", "unpublished staging cleanup failed; recovery lock retained")
			}
			if err := securefs.SyncDir(s.Root, metadata+"/transactions"); err != nil {
				return nil, fault.New("transaction.cleanup_failed", "could not confirm staging removal; recovery lock retained")
			}
		}
		if err := s.Root.Remove("lock/recovery"); err != nil {
			return nil, err
		}
		l := &Lock{store: s, Token: expected, held: true}
		if err := l.Release(); err != nil {
			if info.PeerReceipt != "" {
				return nil, fault.Applied("peer receipt reconciled but lock release failed", info.PeerReceipt)
			}
			return nil, err
		}
		return map[string]any{"recovered": info.PeerReceipt != "" || info.StageID != "", "lock_released": true, "peer_receipt": info.PeerReceipt, "staging_cleaned": info.StageID}, nil
	}
	if err != nil {
		return nil, err
	}
	var j Journal
	if err := StrictJSON(data, &j); err != nil {
		return nil, err
	}
	if err := s.RequireDomain("transactions"); err != nil {
		return nil, err
	}
	if err := s.finishJournal(&j, nil); err != nil {
		return nil, fault.Applied("recovery did not finish; journal and shared lock retained", j.ID)
	}
	if err := s.Root.Remove("lock/recovery"); err != nil {
		return nil, fault.Applied("recovery finalized but recovery lock cleanup failed", j.ID)
	}
	l := &Lock{store: s, Token: expected, held: true}
	if err := l.Release(); err != nil {
		return nil, fault.Applied("recovery finalized but shared lock cleanup failed", j.ID)
	}
	return map[string]any{"recovered": true, "transaction": j.ID, "lock_released": true}, nil
}
