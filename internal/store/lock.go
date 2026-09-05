package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

type Lock struct {
	store *Store
	Token string
	held  bool
}

func (s *Store) Lock(operation string) (*Lock, error) {
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
	if err := s.Validate(); err != nil {
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

func (l *Lock) Release() error {
	if !l.held {
		return nil
	}
	owner, err := securefs.Read(l.store.Root, "lock/owner", 256)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(owner)) != l.Token {
		return fault.New("store.lock_changed", "lock ownership changed")
	}
	for _, name := range []string{"lock/info", "lock/owner"} {
		if err := l.store.Root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if err := l.store.Root.Remove("lock"); err != nil {
		return err
	}
	l.held = false
	return securefs.SyncDir(l.store.Root, ".")
}
