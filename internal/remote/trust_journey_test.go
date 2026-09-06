package remote

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"filippo.io/age/plugin"
	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
	"github.com/agensfield/fulla/internal/store"
)

func trustJourneyModes(t *testing.T, s *store.Store) map[string]fs.FileMode {
	t.Helper()
	modes := map[string]fs.FileMode{}
	if err := fs.WalkDir(s.Root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err == nil {
			modes[name] = info.Mode()
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return modes
}

// A sole plugin identity makes every attempted local decryption observable.
// It intentionally cannot decrypt: a positive control first proves that Read
// invokes it, and failed trust must never get as far as that invocation.
func armDecryptionSentinel(t *testing.T, s *store.Store, marker string) func() {
	t.Helper()
	original, err := securefs.Read(s.Root, "identities", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	identity := []byte(plugin.EncodeIdentity("trustsentinel", []byte("synthetic-identity")) + "\n")
	if err := securefs.Replace(s.Root, "identities", identity); err != nil {
		t.Fatal(err)
	}
	restore := func() {
		t.Helper()
		if err := securefs.Replace(s.Root, "identities", original); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Read("shared"); err == nil {
		t.Fatal("sentinel unexpectedly decrypted")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "invoked\n" {
		t.Fatal("positive control did not observe one decryption attempt", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	return restore
}

func TestTrustChangeJourneyWithoutPrematureDecryption(t *testing.T) {
	bin := t.TempDir()
	marker := filepath.Join(bin, "attempts")
	if err := os.WriteFile(filepath.Join(bin, "age-plugin-trustsentinel"), []byte("#!/bin/sh\nprintf 'invoked\\n' >> \"${0%/*}/attempts\"\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AGEDEBUG", "")
	for _, noGit := range []bool{false, true} {
		for _, version := range []int{CurrentProtocol, PreviousProtocol} {
			t.Run(fmt.Sprintf("no-git=%v/protocol=%d", noGit, version), func(t *testing.T) {
				parent, err := filepath.EvalSymlinks(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				var sides []*store.Store
				for _, name := range []string{"left", "right"} {
					dir := filepath.Join(parent, name)
					if _, err := store.Init(dir, noGit, false); err != nil {
						t.Fatal(err)
					}
					s, err := store.Open(dir, true, nil)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { s.Close() })
					sides = append(sides, s)
					for entry, value := range map[string][]byte{name: {0, 255, '\n'}, "shared": []byte(name)} {
						if _, err := s.Write(entry, value, false); err != nil {
							t.Fatal(err)
						}
					}
				}
				left, right := sides[0], sides[1]
				enroll := func(local, other *store.Store, replace bool, previous string) store.Peer {
					t.Helper()
					identity, err := other.IdentityShow()
					if err != nil {
						t.Fatal(err)
					}
					peer := store.Peer{Version: 1, Name: "other", Host: "fixture", Recipient: identity.Recipient, Fingerprint: identity.Fingerprint}
					if err := local.SavePeer(peer, replace, previous); err != nil {
						t.Fatal(err)
					}
					return peer
				}
				peer := enroll(left, right, false, "")
				restoreLeft := armDecryptionSentinel(t, left, marker)
				restoreRight := armDecryptionSentinel(t, right, marker)
				refused := func(code string, action func() error) {
					t.Helper()
					beforeLeft, beforeRight := storeDigest(t, left), storeDigest(t, right)
					leftModes, rightModes := trustJourneyModes(t, left), trustJourneyModes(t, right)
					err := action()
					var problem *fault.Error
					if !errors.As(err, &problem) || problem.Code != code {
						t.Fatalf("expected %s, got %v", code, err)
					}
					if _, err := os.Stat(marker); !os.IsNotExist(err) {
						t.Fatal("failed trust attempted local decryption", err)
					}
					if !reflect.DeepEqual(beforeLeft, storeDigest(t, left)) || !reflect.DeepEqual(beforeRight, storeDigest(t, right)) {
						t.Fatal("failed trust mutated a store")
					}
					if !reflect.DeepEqual(leftModes, trustJourneyModes(t, left)) || !reflect.DeepEqual(rightModes, trustJourneyModes(t, right)) {
						t.Fatal("failed trust changed paths or modes")
					}
				}
				attemptSync := func() error {
					c := session(t, right, version)
					defer c.Conn.Close()
					_, err := Sync(left, peer, c, true, false)
					return err
				}
				refused("peer.unauthorized", attemptSync)
				restoreRight()
				enroll(right, left, false, "")
				if _, err := right.Rotate(false, "", false); err != nil {
					t.Fatal(err)
				}
				restoreRight = armDecryptionSentinel(t, right, marker)
				refused("peer.trust_mismatch", attemptSync)
				peer = enroll(left, right, true, peer.Fingerprint)
				// Capture a valid client proof without ever allowing the server to
				// decrypt. Replay it against a fresh session's different challenge.
				restoreLeft()
				first, issued := startChallenge(t, left, right, version)
				ids, _, err := left.Keys()
				if err != nil {
					t.Fatal(err)
				}
				captured, err := crypt.Decrypt(issued.Challenge, ids)
				if err != nil {
					t.Fatal(err)
				}
				first.Conn.Close()
				restoreLeft = armDecryptionSentinel(t, left, marker)
				refused("peer.authentication_failed", func() error {
					c, _ := startChallenge(t, left, right, version)
					defer c.Conn.Close()
					if err := writeMessage(c.Conn, message{Operation: "proof", Proof: captured, Challenge: issued.Challenge}); err != nil {
						return err
					}
					_, err := readMessage(c.Conn)
					return err
				})
				restoreLeft()
				restoreRight()
				if err := attemptSync(); err != nil {
					t.Fatal("repinned mutually authorized dry-run", err)
				}
				peer, err = left.Peer("other")
				if err != nil {
					t.Fatal(err)
				}
				c := session(t, right, version)
				result, err := Sync(left, peer, c, false, false)
				c.Conn.Close()
				if err != nil || !result.Pushed || !result.Pulled || !reflect.DeepEqual(result.Skipped, []string{"shared"}) {
					t.Fatal("authorized sync did not converge with shared skip", result, err)
				}
				for index, s := range sides {
					for _, name := range []string{"left", "right"} {
						value, err := s.Read(name)
						if err != nil || !bytes.Equal(value, []byte{0, 255, '\n'}) {
							t.Fatal("authorized transfer changed exact bytes", err)
						}
					}
					value, err := s.Read("shared")
					if err != nil || string(value) != []string{"left", "right"}[index] {
						t.Fatal("shared name overwritten", err)
					}
				}
			})
		}
	}
}
