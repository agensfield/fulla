package remote

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
	"github.com/agensfield/fulla/internal/store"
)

// The predecessor's internal/peerregistry/registry.go at
// f75734b8775f72d5d2f9630c08c2b48bdb6d8104 stores these four fields in
// STORE/peers/NAME.json. These are pa-xfer pins, not Fulla lock records.
func TestLegacyPAXferPinsDoNotAuthorizeAdoptedStores(t *testing.T) {
	for _, noGit := range []bool{false, true} {
		for _, version := range []int{CurrentProtocol, PreviousProtocol} {
			t.Run(fmt.Sprintf("no-git=%v/protocol=%d", noGit, version), func(t *testing.T) {
				parent, err := filepath.EvalSymlinks(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				var stores []*store.Store
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
					if _, err := s.Write(name, []byte{0, 255, 10}, false); err != nil {
						t.Fatal(err)
					}
					stores = append(stores, s)
				}
				pins := make([]store.Peer, 2)
				for i, s := range stores {
					other, err := stores[1-i].IdentityShow()
					if err != nil {
						t.Fatal(err)
					}
					pins[i] = store.Peer{Version: 1, Name: "other", Host: "test", Recipient: other.Recipient, Fingerprint: other.Fingerprint}
					legacy, err := json.MarshalIndent(struct {
						Version     int    `json:"version"`
						Name        string `json:"name"`
						Host        string `json:"host"`
						Fingerprint string `json:"fingerprint"`
					}{1, "other", "test", other.Fingerprint}, "", "  ")
					if err != nil {
						t.Fatal(err)
					}
					// Only disposable test stores are stripped to the predecessor layout.
					if err := s.Root.RemoveAll(".fulla"); err != nil {
						t.Fatal(err)
					}
					if err := s.Root.Mkdir("peers", 0o700); err != nil {
						t.Fatal(err)
					}
					if err := securefs.WriteNew(s.Root, "peers/other.json", append(legacy, '\n')); err != nil {
						t.Fatal(err)
					}
					before := storeDigest(t, s)
					if _, err := store.Adopt(s.Dir, true, nil); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(before, storeDigest(t, s)) {
						t.Fatal("adoption dry-run changed predecessor files")
					}
					if _, err := store.Adopt(s.Dir, false, nil); err != nil {
						t.Fatal(err)
					}
					after := storeDigest(t, s)
					for name, digest := range before {
						if after[name] != digest {
							t.Fatalf("adoption changed predecessor file %s", name)
						}
					}
					opened, err := store.Open(s.Dir, true, nil)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { opened.Close() })
					stores[i] = opened
					peers, err := opened.Peers()
					if err != nil || len(peers) != 0 {
						t.Fatal("legacy pins became Fulla peers", err)
					}
					if _, err := os.Stat(filepath.Join(s.Dir, "peers", "other.json")); err != nil {
						t.Fatal(err)
					}
				}
				left, right := stores[0], stores[1]
				if err := left.SavePeer(pins[0], false, ""); err != nil {
					t.Fatal(err)
				}
				beforeLeft, beforeRight := storeDigest(t, left), storeDigest(t, right)
				c := session(t, right, version)
				err = c.Authenticate(left, pins[0])
				var problem *fault.Error
				if !errors.As(err, &problem) || problem.Code != "peer.unauthorized" {
					t.Fatal("reciprocal legacy pin authorized caller", err)
				}
				c.Conn.Close()
				if !reflect.DeepEqual(beforeLeft, storeDigest(t, left)) || !reflect.DeepEqual(beforeRight, storeDigest(t, right)) {
					t.Fatal("refused authentication changed stores")
				}
				// Positive control: explicit reciprocal Fulla enrollment enables dry-run.
				if err := right.SavePeer(pins[1], false, ""); err != nil {
					t.Fatal(err)
				}
				c = session(t, right, version)
				if _, err := Sync(left, pins[0], c, true, false); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
