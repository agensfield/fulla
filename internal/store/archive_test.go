package store

import (
	"archive/tar"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestFullRestoreRefusesDetachedCleanupOwnership(t *testing.T) {
	recovery := fixture(t, false)
	ids, recipients, err := recovery.Keys()
	if err != nil {
		t.Fatal(err)
	}
	var plain bytes.Buffer
	plain.WriteString(fullArchiveMagic)
	tw := tar.NewWriter(&plain)
	if err := tw.WriteHeader(&tar.Header{Name: lockReleasePrefix + "future-owner", Typeflag: tar.TypeDir, Mode: 0700}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	encrypted, err := crypt.Encrypt(plain.Bytes(), recipients)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = RestoreFull(encrypted, ids, filepath.Join(parent, "restored"))
	var failure *fault.Error
	if !errors.As(err, &failure) || failure.Code != "recovery.unsafe_archive" {
		t.Fatal("did not reject archived cleanup ownership", err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatal("refusal left extracted ownership or staging", entries, err)
	}
}

func TestFullDisasterRestoreAndCircularProtection(t *testing.T) {
	source := fixture(t, true)
	recovery := fixture(t, false)
	if _, err := source.Write("entry", []byte{0, 255, 10}, false); err != nil {
		t.Fatal(err)
	}
	_, active, err := source.Keys()
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(filepath.Dir(source.Dir), "full.age")
	if _, err := source.ExportFull(active, output); err == nil {
		t.Fatal("accepted circular recovery")
	}
	ids, rs, err := recovery.Keys()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.ExportFull(rs, output); err != nil {
		t.Fatal(err)
	}
	data, err := ReadArtifact(output, MaxBundleBytes)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(filepath.Dir(recovery.Dir), "restored")
	if _, err := RestoreFull(data, ids, target); err != nil {
		t.Fatal(err)
	}
	s, err := Open(target, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Read("entry")
	if err != nil || !bytes.Equal(got, []byte{0, 255, 10}) {
		t.Fatal("recovery lost entry", err)
	}
	before, err := securefs.Read(source.Root, "identities", maxMetadata)
	if err != nil {
		t.Fatal(err)
	}
	after, err := securefs.Read(s.Root, "identities", maxMetadata)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("did not restore original identity", err)
	}
	if _, err := RestoreFull(data, ids, target); err == nil {
		t.Fatal("overwrote restore target")
	}
	if _, err := s.CleanGit(); err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 1
	bad := filepath.Join(filepath.Dir(recovery.Dir), "bad")
	if _, err := RestoreFull(data, ids, bad); err == nil {
		t.Fatal("accepted corrupt full archive")
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatal("published corrupt restored state")
	}
}

func TestFullRestoreConfirmation(t *testing.T) {
	source := fixture(t, false)
	recovery := fixture(t, false)
	if _, err := source.Write("entry", []byte{0, 255, 10}, false); err != nil {
		t.Fatal(err)
	}
	ids, rs, err := recovery.Keys()
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(filepath.Dir(source.Dir), "confirmed-full.age")
	if _, err := source.ExportFull(rs, output); err != nil {
		t.Fatal(err)
	}
	data, err := ReadArtifact(output, MaxBundleBytes)
	if err != nil {
		t.Fatal(err)
	}
	for _, existing := range []bool{false, true} {
		name := "absent"
		if existing {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			target := filepath.Join(filepath.Dir(recovery.Dir), name)
			if existing {
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
			}
			cancelled := errors.New("fixture cancellation")
			calls := 0
			_, err := RestoreFullConfirmed(data, ids, target, nil, func(plan ArchiveResult) error {
				calls++
				if plan.Files == 0 || plan.Bytes == 0 || !plan.IdentityCloned || plan.Path != target {
					t.Fatal("incomplete confirmation metadata")
				}
				return cancelled
			})
			if !errors.Is(err, cancelled) || calls != 1 {
				t.Fatalf("confirmation: calls=%d error=%v", calls, err)
			}
			if existing {
				entries, err := os.ReadDir(target)
				if err != nil || len(entries) != 0 {
					t.Fatal("changed empty target", err)
				}
			} else if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatal("published cancelled restore")
			}
			stages, err := filepath.Glob(filepath.Join(filepath.Dir(target), ".fulla-restore-*"))
			if err != nil || len(stages) != 0 {
				t.Fatal("left restore staging", err)
			}
			_, err = RestoreFullConfirmed(data, ids, target, nil, func(ArchiveResult) error {
				if !existing {
					if err := os.Mkdir(target, 0700); err != nil {
						return err
					}
				}
				return os.WriteFile(filepath.Join(target, "occupant"), []byte("preserved"), 0600)
			})
			if err == nil {
				t.Fatal("overwrote target occupied during confirmation")
			}
			got, err := os.ReadFile(filepath.Join(target, "occupant"))
			if err != nil || string(got) != "preserved" {
				t.Fatal("lost target occupant", err)
			}
		})
	}
	corrupt := append([]byte(nil), data...)
	corrupt[len(corrupt)-1] ^= 1
	called := false
	_, err = RestoreFullConfirmed(corrupt, ids, filepath.Join(filepath.Dir(recovery.Dir), "corrupt"), nil, func(ArchiveResult) error { called = true; return nil })
	if err == nil || called {
		t.Fatal("asked confirmation before archive authentication")
	}
}
