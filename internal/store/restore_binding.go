package store

import (
	"errors"
	"io/fs"
	"strings"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

// Called only after archive EOF authentication, live verification and clean Git.
// Bind the restored identity before the validated checkpoint and publication.
func (s *Store) bindRestorePublication(lock *Lock) error {
	info, err := s.InspectLock()
	if err != nil {
		return err
	}
	if info == nil || info.Token != lock.Token || info.RestoreID == "" || info.RestoreStoreID != "" || !validID(s.Meta.StoreID) {
		return fault.New("store.lock_changed", "restore ownership changed before publication")
	}
	for _, name := range []string{"pending.json", "rotation.json", "prune.json"} {
		if _, err := s.Root.Lstat(metadata + "/" + name); !errors.Is(err, fs.ErrNotExist) {
			return fault.New("transaction.pending", "restore refuses archived pending operations")
		}
	}
	for _, domain := range []string{"peers", "sync", "backup", "identity", "transactions"} {
		if err := s.RequireDomain(domain); err != nil {
			return err
		}
	}
	data, err := securefs.Read(s.Root, "lock/info", 4096)
	if err != nil {
		return err
	}
	bound := strings.TrimSpace(string(data)) + " restore_store_id=" + s.Meta.StoreID + "\n"
	if len(bound) > 4000 {
		return fault.New("store.lock_unknown", "restore binding exceeds supported size")
	}
	return securefs.Replace(s.Root, "lock/info", []byte(bound))
}
