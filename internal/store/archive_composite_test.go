package store

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type archiveTreeEntry struct {
	Mode   fs.FileMode
	Digest string
}

func archiveTree(t *testing.T, s *Store, normalized bool) map[string]archiveTreeEntry {
	t.Helper()
	tree := map[string]archiveTreeEntry{}
	err := fs.WalkDir(s.Root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()
		hash := ""
		if entry.IsDir() {
			if normalized {
				mode = fs.ModeDir | 0700
			}
		} else {
			data, err := fs.ReadFile(s.Root.FS(), name)
			if err != nil {
				return err
			}
			hash = digest(data)
			if normalized {
				mode = 0600
			}
		}
		tree[name] = archiveTreeEntry{mode, hash}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func TestCompositeDisasterArchiveRestoresAllState(t *testing.T) {
	for _, git := range []bool{false, true} {
		t.Run(fmt.Sprintf("git=%v", git), func(t *testing.T) {
			source, recovery, remote := fixture(t, git), fixture(t, false), fixture(t, false)
			original := []byte{0, 255, 10, 10}
			for _, entry := range []struct {
				name  string
				value []byte
			}{
				{"binary", original}, {"empty", []byte{}}, {"nested/newlines", []byte("one\n\n")}, {"deleted", []byte("recover from the past")},
			} {
				if _, err := source.Write(entry.name, entry.value, false); err != nil {
					t.Fatal(err)
				}
			}
			oldCommit := ""
			if git {
				history, err := source.History("binary")
				if err != nil || len(history) == 0 {
					t.Fatal("missing original history", err)
				}
				oldCommit = history[0].Commit
			}
			edited, err := source.Write("binary", []byte("current binary value"), true)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := source.Remove("deleted", !git); err != nil {
				t.Fatal(err)
			}
			// Two rotations require the restored sealed-key chain to reach the oldest
			// snapshot/history. No old cleartext identity is supplied to restoration.
			for range 2 {
				rotation, err := source.Rotate(false, "", false)
				if err != nil || rotation.RetiredArtifact == "" {
					t.Fatal("missing retired identity", err)
				}
			}
			local, err := source.IdentityShow()
			if err != nil {
				t.Fatal(err)
			}
			other, err := remote.IdentityShow()
			if err != nil {
				t.Fatal(err)
			}
			peer := Peer{Version: 1, Name: "archive-peer", Host: "fixture.invalid", Store: "/fixture/store", Recipient: other.Recipient, Fingerprint: other.Fingerprint}
			if err := source.SavePeer(peer, false, ""); err != nil {
				t.Fatal(err)
			}
			if err := source.MarkPeer(peer.Name, peer.Fingerprint, local.Fingerprint, true); err != nil {
				t.Fatal(err)
			}
			peers, err := source.Peers()
			if err != nil || len(peers) != 1 || !peers[0].Activated {
				t.Fatal("missing peer state", err)
			}
			backups, err := source.Backups()
			if err != nil || len(backups) < 5 {
				t.Fatal("missing snapshots", err)
			}
			var history []HistoryEntry
			if git {
				history, err = source.History("")
				if err != nil {
					t.Fatal(err)
				}
			}
			before := archiveTree(t, source, false)
			expected := archiveTree(t, source, true)
			for _, prefix := range []string{".fulla/backups/", ".fulla/peers/", ".fulla/receipts/", ".fulla/retired/"} {
				found := false
				for name := range before {
					if strings.HasPrefix(name, prefix) {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("fixture lacks %s", prefix)
				}
			}
			identities, recipients, err := recovery.Keys()
			if err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(filepath.Dir(source.Dir), "composite.age")
			if _, err := source.ExportFull(recipients, output); err != nil {
				t.Fatal(err)
			}
			after := archiveTree(t, source, false)
			for name, entry := range before {
				if after[name] != entry {
					t.Fatalf("export changed source path %s", name)
				}
			}
			added := 0
			for name := range after {
				if _, ok := before[name]; !ok {
					added++
					if !strings.HasPrefix(name, ".fulla/receipts/") || !strings.HasSuffix(name, ".json") {
						t.Fatalf("unexpected export addition %s", name)
					}
				}
			}
			if added != 1 {
				t.Fatalf("expected one export receipt, got %d", added)
			}
			ciphertext, err := ReadArtifact(output, MaxBundleBytes)
			if err != nil {
				t.Fatal(err)
			}
			// Simulate loss of the source directory. Only the independently protected
			// archive and recovery store remain available for both target variants.
			if err := source.Close(); err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(source.Dir); err != nil {
				t.Fatal(err)
			}
			for _, existing := range []bool{false, true} {
				t.Run(fmt.Sprintf("existing-empty=%v", existing), func(t *testing.T) {
					target := filepath.Join(filepath.Dir(recovery.Dir), fmt.Sprintf("restore-%v", existing))
					if existing {
						if err := os.Mkdir(target, 0700); err != nil {
							t.Fatal(err)
						}
					}
					result, err := RestoreFull(ciphertext, identities, target)
					if err != nil || !result.IdentityCloned {
						t.Fatal("full restore failed", err)
					}
					restored, err := Open(target, true, nil)
					if err != nil {
						t.Fatal(err)
					}
					defer restored.Close()
					if !reflect.DeepEqual(expected, archiveTree(t, restored, false)) {
						t.Fatal("complete restored paths, bytes, or private modes differ")
					}
					if err := restored.DeepVerify(); err != nil {
						t.Fatal(err)
					}
					gotIdentity, err := restored.IdentityShow()
					if err != nil || !reflect.DeepEqual(local, gotIdentity) {
						t.Fatal("identity state differs", err)
					}
					gotPeers, err := restored.Peers()
					if err != nil || !reflect.DeepEqual(peers, gotPeers) {
						t.Fatal("peer pins or activation state differ", err)
					}
					gotBackups, err := restored.Backups()
					if err != nil || !reflect.DeepEqual(backups, gotBackups) {
						t.Fatal("snapshot inventory differs", err)
					}
					if git {
						gotHistory, err := restored.History("")
						if err != nil || !reflect.DeepEqual(history, gotHistory) {
							t.Fatal("Git history differs", err)
						}
					}
					for _, entry := range []struct {
						name  string
						value []byte
					}{{"binary", []byte("current binary value")}, {"empty", []byte{}}, {"nested/newlines", []byte("one\n\n")}} {
						got, err := restored.Read(entry.name)
						if err != nil || !bytes.Equal(got, entry.value) {
							t.Fatalf("live value %s differs: %v", entry.name, err)
						}
					}
					if _, err := restored.BackupRestore(edited.Transaction, "before"); err != nil {
						t.Fatal("old-key snapshot unusable", err)
					}
					got, err := restored.Read("binary")
					if err != nil || !bytes.Equal(got, original) {
						t.Fatal("old-key snapshot lost exact bytes", err)
					}
					got, err = restored.Read("deleted")
					if err != nil || string(got) != "recover from the past" {
						t.Fatal("snapshot lost deleted value", err)
					}
					if git {
						if _, err := restored.Write("binary", []byte("post-restore edit"), true); err != nil {
							t.Fatal(err)
						}
						if _, err := restored.HistoryRestore(oldCommit, "binary"); err != nil {
							t.Fatal("old-key history unusable", err)
						}
						got, err := restored.Read("binary")
						if err != nil || !bytes.Equal(got, original) {
							t.Fatal("old-key history lost exact bytes", err)
						}
					}
					gotPeers, err = restored.Peers()
					if err != nil || !reflect.DeepEqual(peers, gotPeers) {
						t.Fatal("recovery mutation changed restored peer authority", err)
					}
				})
			}
		})
	}
}
