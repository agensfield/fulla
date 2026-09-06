package store

import (
	"fmt"
	"os"
	"time"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

type Lock struct {
	store         *Store
	Token         string
	StageID       string
	StageProtocol string
	releaseDir    string
	held          bool
}

func (s *Store) Lock(operation string) (*Lock, error) { return s.lock(operation, s.Validate) }

// Entry mutations must know how to journal and retain their snapshots before
// requesting input or decrypting. Repeat the check under the shared lock, and
// leave mutate's publication-time check in place for long-lived callers.
func (s *Store) lockMutation(operation string) (*Lock, error) {
	if err := s.checkMutationCommand(operation); err != nil {
		return nil, err
	}
	return s.lock(operation, func() error {
		if err := s.Validate(); err != nil {
			return err
		}
		return s.checkMutationCommand(operation)
	})
}

func (s *Store) checkMutationCommand(operation string) error {
	if !basicCommand(operation) {
		if err := s.RequireDomain("transactions"); err != nil {
			return err
		}
	}
	return s.CheckMutationDomains()
}

// CheckMutationDomains lets callers refuse unavailable entry mutations before
// consuming secret input. It is advisory; lockMutation and mutate revalidate.
func (s *Store) CheckMutationDomains() error {
	_, err := s.transactionSnapshotDomain()
	return err
}

func (s *Store) lock(operation string, validate func() error) (*Lock, error) {
	if err := s.Unlocked(); err != nil {
		return nil, err
	}
	if err := s.Root.Mkdir("lock", 0o700); err != nil {
		return nil, fault.New("store.locked", "could not exclusively acquire shared store lock")
	}
	l := &Lock{store: s, Token: securefs.ID(), held: true}
	if err := securefs.WriteNew(s.Root, "lock/owner", []byte(l.Token+"\n")); err != nil {
		_ = s.Root.Remove("lock")
		return nil, err
	}
	host, err := os.Hostname()
	if err != nil {
		_ = l.Release()
		return nil, err
	}
	info := fmt.Sprintf("pid=%d host=%s operation=%s started=%s\n", os.Getpid(), host, operation, time.Now().UTC().Format(time.RFC3339))
	if err := securefs.WriteNew(s.Root, "lock/info", []byte(info)); err != nil {
		_ = l.Release()
		return nil, err
	}
	if err := securefs.SyncDir(s.Root, "."); err != nil {
		_ = l.Release()
		return nil, err
	}
	// Revalidate under the shared lock; another cooperating writer may have
	// committed between initial validation and lock acquisition.
	if err := validate(); err != nil {
		_ = l.Release()
		return nil, err
	}
	if s.ExpectedFingerprint != "" {
		identity, err := s.IdentityShow()
		if err != nil {
			_ = l.Release()
			return nil, err
		}
		if identity.Fingerprint != s.ExpectedFingerprint {
			_ = l.Release()
			return nil, fault.New("peer.trust_mismatch", "local identity changed during peer session")
		}
	}
	if s.ExpectedPeerName != "" {
		if err := s.RequireDomain("sync"); err != nil {
			_ = l.Release()
			return nil, err
		}
		peer, err := s.Peer(s.ExpectedPeerName)
		if err != nil {
			_ = l.Release()
			return nil, err
		}
		if peer.Fingerprint != s.ExpectedPeerFingerprint {
			_ = l.Release()
			return nil, fault.New("peer.trust_mismatch", "peer authorization changed during session")
		}
	}
	return l, nil
}

func (l *Lock) Release() error { return l.release(nil) }
