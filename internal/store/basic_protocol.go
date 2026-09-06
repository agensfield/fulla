package store

import (
	"errors"
	"io/fs"
	"path"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

// These paths and codec are fixed throughout v1, independent of upgradeable
// feature metadata. Future transaction writers must not reinterpret them.
const basicProtocol = "basic-v1"
const basicBase = metadata + "/" + basicProtocol

func basicCommand(command string) bool {
	switch command {
	case "add", "edit", "remove", "move":
		return true
	}
	return false
}

func stageProtocol(domain string) string {
	if domain == basicProtocol {
		return basicProtocol
	}
	return ""
}

func transactionBase(domain string) string {
	if domain == basicProtocol {
		return basicBase + "/transactions"
	}
	return metadata + "/transactions"
}

func journalPath(domain string) string {
	if domain == basicProtocol {
		return basicBase + "/pending.json"
	}
	return metadata + "/pending.json"
}

func receiptBase(domain string) string {
	if domain == basicProtocol {
		return basicBase + "/receipts"
	}
	return metadata + "/receipts"
}

func (s *Store) prepareBasicProtocol() error {
	for _, dir := range []string{basicBase, transactionBase(basicProtocol), snapshotBase(basicProtocol), receiptBase(basicProtocol)} {
		if err := s.Root.MkdirAll(dir, 0700); err != nil {
			return err
		}
		if err := securefs.SyncDir(s.Root, path.Dir(dir)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) validateStageProtocol(info *LockInfo) error {
	// Creation recovery owns its entire unpublished clone, including opaque journals.
	if info.InitID != "" || info.RestoreID != "" {
		return nil
	}
	if info.ExportReceipt != "" {
		for _, name := range []string{"pending.json", "rotation.json", "prune.json", "basic-v1/pending.json"} {
			if _, err := s.Root.Lstat(metadata + "/" + name); err == nil {
				return fault.New("transaction.conflict", "export recovery refuses competing journals")
			} else if !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
		return nil
	}
	if info.StageProtocol == "" {
		if _, err := s.Root.Lstat(journalPath(basicProtocol)); err == nil {
			return fault.New("transaction.conflict", "basic journal requires its explicit staging protocol binding")
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return s.validateStagingBinding(info.StageID)
	}
	if info.StageProtocol != basicProtocol || !validID(info.StageID) {
		return fault.New("transaction.invalid", "unsupported basic staging ownership")
	}
	for _, name := range []string{"pending.json", "rotation.json", "prune.json"} {
		if _, err := s.Root.Lstat(metadata + "/" + name); err == nil {
			return fault.New("transaction.conflict", "basic recovery cannot interpret competing feature journals")
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if entry, err := s.Root.Lstat(transactionBase(basicProtocol) + "/" + info.StageID); err == nil {
		if !entry.IsDir() {
			return fault.New("transaction.invalid_staging", "bound basic staging is not a directory")
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	data, err := securefs.Read(s.Root, journalPath(basicProtocol), maxMetadata)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var j Journal
	if err := StrictJSON(data, &j); err != nil {
		return err
	}
	if j.ID != info.StageID || j.Version != 1 || j.SnapshotDomain != basicProtocol || !basicCommand(j.Command) {
		return fault.New("transaction.conflict", "basic journal differs from bound operation")
	}
	return nil
}

func (s *Store) recoverBasic(info *LockInfo, token string) (map[string]any, error) {
	if err := s.validateStageProtocol(info); err != nil {
		return nil, err
	}
	data, err := securefs.Read(s.Root, journalPath(basicProtocol), maxMetadata)
	if errors.Is(err, fs.ErrNotExist) {
		dir := transactionBase(basicProtocol) + "/" + info.StageID
		if entry, err := s.Root.Lstat(dir); err == nil {
			if !entry.IsDir() {
				return nil, fault.New("transaction.invalid_staging", "bound basic staging is not a directory")
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if err := s.Root.RemoveAll(dir); err != nil {
			return nil, err
		}
		if err := securefs.SyncDir(s.Root, transactionBase(basicProtocol)); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else {
		var j Journal
		if err := StrictJSON(data, &j); err != nil {
			return nil, err
		}
		if err := s.finishJournal(&j, nil); err != nil {
			return nil, fault.Applied("basic entry recovery incomplete; shared lock retained", j.ID)
		}
	}
	if err := s.Root.Remove("lock/recovery"); err != nil {
		return nil, err
	}
	lock := &Lock{store: s, Token: token, held: true}
	if err := lock.Release(); err != nil {
		return nil, err
	}
	return map[string]any{"recovered": true, "transaction": info.StageID, "protocol": basicProtocol, "lock_released": true}, nil
}
