package store

import (
	"errors"
	"io/fs"
	"path"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

// Publication copies stay inside the journal's owned transaction directory.
// Keep after/ intact for validation/retry; rename only the separate copy.
func (s *Store) publishRotationFile(dir string, change RotationChange, data []byte, hook func(string) error) error {
	staged := dir + "/publish-" + digest([]byte(change.Path))
	existing, err := securefs.Read(s.Root, staged, 65<<20)
	if errors.Is(err, fs.ErrNotExist) {
		if err := securefs.WriteNew(s.Root, staged, data); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if digest(existing) != change.After {
		return fault.New("identity.corrupt_rotation", "rotation publication copy mismatch")
	}
	if hook != nil {
		if err := hook("publication-staged:" + change.Path); err != nil {
			return err
		}
	}
	if change.Before == "" {
		from, openErr := s.Root.OpenRoot(dir)
		if openErr != nil {
			return openErr
		}
		defer from.Close()
		to, openErr := s.Root.OpenRoot(path.Dir(change.Path))
		if openErr != nil {
			return openErr
		}
		defer to.Close()
		err = securefs.RenameNewBetween(from, path.Base(staged), to, path.Base(change.Path))
	} else {
		err = s.Root.Rename(staged, change.Path)
	}
	if err != nil {
		return err
	}
	return s.syncRotationPublication(dir, change.Path)
}

func (s *Store) syncRotationPublication(dir, target string) error {
	// Cross-directory rename changes both parents. Retry also repeats these
	// flushes when the journal's destination already has the expected bytes.
	return errors.Join(securefs.SyncDir(s.Root, dir), securefs.SyncDir(s.Root, path.Dir(target)))
}
