package store

import (
	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
)

func (s *Store) Write(name string, value []byte, edit bool) (MutationResult, error) {
	return s.write(name, value, edit, nil)
}

// WriteInteractive holds the shared lock while obtaining input. For edits the
// callback receives the exact current value, so an editor cannot overwrite a
// concurrent cooperating writer's change.
func (s *Store) WriteInteractive(name string, edit bool, input func([]byte) ([]byte, error)) (MutationResult, error) {
	return s.write(name, nil, edit, input)
}

func (s *Store) write(name string, value []byte, edit bool, input func([]byte) ([]byte, error)) (MutationResult, error) {
	result := MutationResult{}
	if _, err := EntryPath(name); err != nil {
		return result, err
	}
	if len(value) > crypt.MaxEntryBytes {
		return result, fault.New("entry.too_large", "entry exceeds the supported size limit")
	}
	command := "add"
	if edit {
		command = "edit"
	}
	lock, err := s.Lock(command)
	if err != nil {
		return result, err
	}
	exists, err := s.Exists(name)
	if err != nil {
		_ = lock.Release()
		return result, err
	}
	if exists && !edit {
		_ = lock.Release()
		return result, fault.New("entry.exists", "entry already exists; use edit")
	}
	if !exists && edit {
		_ = lock.Release()
		return result, fault.New("entry.not_found", "entry does not exist; use add")
	}
	if _, err := s.CleanGit(); err != nil {
		_ = lock.Release()
		return result, err
	}
	ids, rs, err := s.Keys()
	if err != nil {
		_ = lock.Release()
		return result, err
	}
	if input != nil {
		var original []byte
		if edit {
			ciphertext, e := s.Ciphertext(name)
			if e != nil {
				_ = lock.Release()
				return result, e
			}
			original, err = crypt.Decrypt(ciphertext, ids)
			if err != nil {
				_ = lock.Release()
				return result, err
			}
		}
		value, err = input(original)
		if err != nil {
			_ = lock.Release()
			return result, err
		}
		if len(value) > crypt.MaxEntryBytes {
			_ = lock.Release()
			return result, fault.New("entry.too_large", "entry exceeds the supported size limit")
		}
	}
	ciphertext, err := crypt.Encrypt(value, rs)
	if err != nil {
		_ = lock.Release()
		return result, err
	}
	if _, err := crypt.Decrypt(ciphertext, ids); err != nil {
		_ = lock.Release()
		return result, err
	}
	return s.mutate(lock, command, map[string][]byte{name: ciphertext}, nil)
}

func (s *Store) Remove(name string, permanent bool) (MutationResult, error) {
	result := MutationResult{}
	if _, err := EntryPath(name); err != nil {
		return result, err
	}
	lock, err := s.Lock("remove")
	if err != nil {
		return result, err
	}
	enabled, err := s.CleanGit()
	if err != nil {
		_ = lock.Release()
		return result, err
	}
	if !enabled && !permanent {
		_ = lock.Release()
		return result, fault.Interaction("untracked deletion requires --permanent-delete acknowledgement")
	}
	exists, err := s.Exists(name)
	if err != nil {
		_ = lock.Release()
		return result, err
	}
	if !exists {
		_ = lock.Release()
		return result, fault.New("entry.not_found", "entry does not exist")
	}
	return s.mutate(lock, "remove", map[string][]byte{name: nil}, nil)
}

func (s *Store) Move(from, to string) (MutationResult, error) {
	result := MutationResult{}
	for _, name := range []string{from, to} {
		if _, err := EntryPath(name); err != nil {
			return result, err
		}
	}
	lock, err := s.Lock("move")
	if err != nil {
		return result, err
	}
	exists, err := s.Exists(to)
	if err != nil {
		_ = lock.Release()
		return result, err
	}
	if exists {
		_ = lock.Release()
		return result, fault.New("entry.exists", "move destination already exists")
	}
	ciphertext, err := s.Ciphertext(from)
	if err != nil {
		_ = lock.Release()
		return result, err
	}
	return s.mutate(lock, "move", map[string][]byte{from: nil, to: ciphertext}, nil)
}
