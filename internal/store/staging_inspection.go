package store

import (
	"errors"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/agensfield/fulla/internal/fault"
)

// inspectStaging lists locations, not private contents or deletion candidates.
// A path can belong to a live writer, pending recovery, or an interrupted
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
	// Atomic key publication uses sibling files outside transaction staging.
	// A killed writer can leave plaintext identities or sealed retired keys here.
	for _, directory := range []string{".", metadata + "/retired"} {
		atomic, err := s.inspectAtomicKeyStaging(directory)
		if err != nil {
			return nil, err
		}
		paths = append(paths, atomic...)
	}
	sort.Strings(paths)
	return paths, nil
}

// Names establish suspicion, never ownership or authority to remove evidence.
// Scan in bounded batches and refuse unusually large directories explicitly.
func (s *Store) inspectAtomicKeyStaging(directory string) ([]string, error) {
	dir, err := s.Root.Open(directory)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	paths := []string{}
	count := 0
	for {
		entries, err := dir.ReadDir(256)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		count += len(entries)
		if count > 65536 {
			return nil, fault.New("identity.staging_scan_limit", "key staging directory exceeds inspection limit")
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".fulla-stage-") {
				paths = append(paths, path.Join(directory, entry.Name()))
			}
		}
		if errors.Is(err, io.EOF) {
			return paths, nil
		}
	}
}
