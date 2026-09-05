package remote

import (
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/agensfield/fulla/internal/store"
)

func TestMain(m *testing.M) { syscall.Umask(0o077); os.Exit(m.Run()) }

func paired(t *testing.T) (*store.Store, *store.Store) {
	t.Helper()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stores := []*store.Store{}
	for _, name := range []string{"left", "right"} {
		dir := filepath.Join(parent, name)
		if _, err := store.Init(dir, true, false); err != nil {
			t.Fatal(err)
		}
		s, err := store.Open(dir, true, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		stores = append(stores, s)
	}
	for i, s := range stores {
		other, err := stores[1-i].IdentityShow()
		if err != nil {
			t.Fatal(err)
		}
		if err := s.SavePeer(store.Peer{Version: 1, Name: "other", Host: "test", Recipient: other.Recipient, Fingerprint: other.Fingerprint}, false, ""); err != nil {
			t.Fatal(err)
		}
	}
	return stores[0], stores[1]
}

func session(t *testing.T, server *store.Store, version int) *Client {
	t.Helper()
	opened, err := store.Open(server.Dir, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	left, right := net.Pipe()
	left.SetDeadline(time.Now().Add(20 * time.Second))
	right.SetDeadline(time.Now().Add(20 * time.Second))
	done := make(chan struct{})
	go func() { defer close(done); defer opened.Close(); defer right.Close(); _ = Serve(opened, right, right) }()
	t.Cleanup(func() { left.Close(); right.Close(); <-done })
	return &Client{Conn: left, Version: version}
}

func TestMutualSyncCurrentAndPrevious(t *testing.T) {
	for _, version := range []int{CurrentProtocol, PreviousProtocol} {
		t.Run(string(rune('0'+version)), func(t *testing.T) {
			left, right := paired(t)
			for _, item := range []struct {
				s           *store.Store
				name, value string
			}{{left, "left", "L"}, {right, "right", "R"}, {left, "shared", "different-left"}, {right, "shared", "different-right"}} {
				if _, err := item.s.Write(item.name, []byte(item.value), false); err != nil {
					t.Fatal(err)
				}
			}
			peer, err := left.Peer("other")
			if err != nil {
				t.Fatal(err)
			}
			c := session(t, right, version)
			if _, err := Sync(left, peer, c, true, false); err != nil {
				t.Fatal(err)
			}
			peer, err = left.Peer("other")
			if err != nil {
				t.Fatal(err)
			}
			c = session(t, right, version)
			result, err := Sync(left, peer, c, false, false)
			if err != nil {
				t.Fatal(err)
			}
			if !result.Pushed || !result.Pulled || len(result.Skipped) != 1 {
				t.Fatal(result)
			}
			for _, s := range []*store.Store{left, right} {
				names, err := s.Names()
				if err != nil || len(names) != 3 {
					t.Fatal(names, err)
				}
			}
			value, err := left.Read("shared")
			if err != nil || string(value) != "different-left" {
				t.Fatal("overwrote shared", err)
			}
			value, err = right.Read("shared")
			if err != nil || string(value) != "different-right" {
				t.Fatal("overwrote shared", err)
			}
		})
	}
}

func TestUnpairedAndChangedIdentityFailClosed(t *testing.T) {
	left, right := paired(t)
	peer, err := left.Peer("other")
	if err != nil {
		t.Fatal(err)
	}
	if err := right.RemovePeer("other"); err != nil {
		t.Fatal(err)
	}
	c := session(t, right, CurrentProtocol)
	if err := c.Authenticate(left, peer); err == nil {
		t.Fatal("SSH-only access authorized")
	}
	c.Conn.Close()
	if _, err := right.Rotate(false, "", false); err != nil {
		t.Fatal(err)
	}
	c = session(t, right, CurrentProtocol)
	if err := c.Authenticate(left, peer); err == nil {
		t.Fatal("changed recipient accepted")
	}
	c.Conn.Close()
	c = session(t, right, PreviousProtocol-1)
	c.Version = -1
	if _, err := c.Hello(); err == nil {
		t.Fatal("obsolete protocol accepted")
	}
	c.Conn.Close()
}
