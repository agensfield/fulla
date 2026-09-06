package store

import (
	"errors"
	"io"
	"sort"

	"github.com/agensfield/fulla/internal/fault"
)

// inspectStaging lists locations, not private contents or deletion candidates.
// A directory can belong to a live writer, pending recovery, or an interrupted
// unpublished operation. A lock-free observation cannot establish ownership.
func (s *Store) inspectStaging() ([]string, error) {
	if err := s.RequireDomain("transactions"); err != nil {
		return nil, err
	}
	dir, err := s.Root.Open(metadata + "/transactions")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(1025)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > 1024 {
		return nil, fault.New("transaction.too_many_stages", "transaction staging count exceeds inspection limit")
	}
	paths := []string{}
	for _, entry := range entries {
		if !entry.IsDir() || !validID(entry.Name()) {
			return nil, fault.New("transaction.invalid_staging", "unexpected transaction staging path")
		}
		paths = append(paths, metadata+"/transactions/"+entry.Name())
	}
	sort.Strings(paths)
	return paths, nil
}
