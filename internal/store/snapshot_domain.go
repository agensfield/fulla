package store

import (
	"errors"
	"io/fs"

	"github.com/agensfield/fulla/internal/fault"
)

// Transaction snapshots remain version-1 transaction data when a newer backup
// feature domain is present. They never share that domain's directory. The
// chosen domain is persisted in the journal before any live publication.
func (s *Store) transactionSnapshotDomain() (string, error) {
	version, err := s.domainVersion("backup")
	if err != nil {
		return "", err
	}
	if version > 1 {
		return "transactions", nil
	}
	return "", nil
}

func snapshotBase(domain string) string {
	if domain == "transactions" {
		return metadata + "/transaction-backups"
	}
	return metadata + "/backups"
}

func (s *Store) requireSnapshotDomain(domain string) error {
	switch domain {
	case "":
		return s.RequireDomain("backup")
	case "transactions":
		return s.RequireDomain("transactions")
	default:
		return fault.New("transaction.invalid", "unsupported snapshot domain")
	}
}

// A backup identifier must identify one snapshot, never two competing copies.
// Callers enforce the backup feature-domain gate before using this lookup.
func (s *Store) snapshotLocation(id string) (string, string, error) {
	if !validID(id) {
		return "", "", fault.Usage("invalid backup identifier")
	}
	found, domain := "", ""
	for _, candidate := range []string{"", "transactions"} {
		location := snapshotBase(candidate) + "/" + id
		info, err := s.Root.Lstat(location)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", "", err
		}
		if !info.IsDir() {
			return "", "", fault.New("backup.invalid", "snapshot is not a directory")
		}
		if found != "" {
			return "", "", fault.New("backup.conflict", "backup identifier has conflicting locations")
		}
		if err := s.requireSnapshotDomain(candidate); err != nil {
			return "", "", err
		}
		found, domain = location, candidate
	}
	if found == "" {
		return "", "", fault.New("backup.not_found", "backup is missing")
	}
	return found, domain, nil
}
