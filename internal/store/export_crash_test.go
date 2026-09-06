package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"filippo.io/age"
)

func TestExportCrashHelper(t *testing.T) {
	dir := os.Getenv("FULLA_EXPORT_CRASH_STORE")
	if dir == "" {
		return
	}
	s, err := Open(dir, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	recipient, err := age.ParseX25519Recipient(os.Getenv("FULLA_EXPORT_CRASH_RECIPIENT"))
	if err != nil {
		t.Fatal(err)
	}
	hook := func(phase string) error {
		if phase == os.Getenv("FULLA_EXPORT_CRASH_PHASE") {
			fmt.Println("export-boundary")
			time.Sleep(time.Minute)
		}
		return nil
	}
	if os.Getenv("FULLA_EXPORT_CRASH_KIND") == "full" {
		_, err = s.exportFull([]age.Recipient{recipient}, os.Getenv("FULLA_EXPORT_CRASH_OUTPUT"), hook)
	} else {
		_, err = s.exportLogical(nil, []age.Recipient{recipient}, os.Getenv("FULLA_EXPORT_CRASH_OUTPUT"), io.Discard, hook)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestKilledExportPreservesArtifactAndStore(t *testing.T) {
	for _, git := range []bool{false, true} {
		for _, kind := range []string{"logical", "full"} {
			for _, phase := range []string{"encoded", "published", "receipted"} {
				t.Run(fmt.Sprintf("git=%t/%s/%s", git, kind, phase), func(t *testing.T) {
					s := fixture(t, git)
					values := map[string][]byte{"binary": {0, 255, 10, 42}, "empty": {}}
					for name, value := range values {
						if _, err := s.Write(name, value, false); err != nil {
							t.Fatal(err)
						}
					}
					before := archiveTree(t, s, false)
					expectedRestore := archiveTree(t, s, true)
					identity, err := age.GenerateX25519Identity()
					if err != nil {
						t.Fatal(err)
					}
					parent, err := filepath.EvalSymlinks(t.TempDir())
					if err != nil {
						t.Fatal(err)
					}
					output := filepath.Join(parent, "archive.age")
					cmd := exec.Command(os.Args[0], "-test.run=^TestExportCrashHelper$")
					cmd.Env = append(os.Environ(), "FULLA_EXPORT_CRASH_STORE="+s.Dir, "FULLA_EXPORT_CRASH_KIND="+kind, "FULLA_EXPORT_CRASH_PHASE="+phase, "FULLA_EXPORT_CRASH_OUTPUT="+output, "FULLA_EXPORT_CRASH_RECIPIENT="+identity.Recipient().String())
					stdout, err := cmd.StdoutPipe()
					if err != nil {
						t.Fatal(err)
					}
					var diagnostics bytes.Buffer
					cmd.Stderr = &diagnostics
					if err := cmd.Start(); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
					ready := make(chan bool, 1)
					go func() {
						scanner := bufio.NewScanner(stdout)
						ready <- scanner.Scan() && scanner.Text() == "export-boundary"
					}()
					select {
					case ok := <-ready:
						if !ok {
							t.Fatal("export exited before boundary")
						}
					case <-time.After(20 * time.Second):
						t.Fatal("export boundary timeout")
					}
					owner, err := s.InspectLock()
					if err != nil || owner == nil || !owner.Alive {
						t.Fatal("missing live owner", err)
					}
					if _, err := s.Recover(owner.Token); err == nil {
						t.Fatal("recovered live exporter")
					}
					if err := cmd.Process.Kill(); err != nil {
						t.Fatal(err)
					}
					_ = cmd.Wait()
					if diagnostics.Len() != 0 {
						t.Fatal("unexpected export diagnostics")
					}
					if _, err := s.Recover("wrong-token"); err == nil {
						t.Fatal("accepted wrong owner")
					}
					result, err := s.Recover(owner.Token)
					if err != nil || result["lock_released"] != true || result["recovered"] != false {
						t.Fatal("unexpected export lock recovery", result, err)
					}
					after := archiveTree(t, s, false)
					newFiles := 0
					for name := range after {
						if _, exists := before[name]; exists {
							continue
						}
						newFiles++
						if phase != "receipted" || filepath.Dir(name) != metadata+"/receipts" {
							t.Fatal("unexpected store residue", name)
						}
						receiptBytes, err := os.ReadFile(filepath.Join(s.Dir, name))
						if err != nil {
							t.Fatal(err)
						}
						var receipt struct {
							Command string
							Applied bool
						}
						if err := json.Unmarshal(receiptBytes, &receipt); err != nil {
							t.Fatal(err)
						}
						command := "transfer export"
						if kind == "full" {
							command = "backup export"
						}
						if receipt.Command != command || !receipt.Applied {
							t.Fatal("inaccurate export receipt")
						}
						delete(after, name)
					}
					expectedNew := 0
					if phase == "receipted" {
						expectedNew = 1
					}
					if newFiles != expectedNew || !reflect.DeepEqual(before, after) {
						t.Fatal("changed source state")
					}
					entries, err := os.ReadDir(parent)
					if err != nil {
						t.Fatal(err)
					}
					if phase == "encoded" {
						if len(entries) != 0 {
							t.Fatal("unpublished export left artifacts")
						}
						if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
							t.Fatal("unexpected output", err)
						}
						return
					}
					if len(entries) != 1 {
						t.Fatal("unexpected external staging")
					}
					archive, err := ReadArtifact(output, MaxBundleBytes)
					if err != nil {
						t.Fatal(err)
					}
					targetDir := filepath.Join(parent, "restored")
					var target *Store
					if kind == "full" {
						if _, err := RestoreFull(archive, []age.Identity{identity}, targetDir); err != nil {
							t.Fatal(err)
						}
						target, err = Open(targetDir, true, nil)
						if err != nil {
							t.Fatal(err)
						}
						defer target.Close()
						restored := archiveTree(t, target, false)
						if !reflect.DeepEqual(expectedRestore, restored) {
							for name, want := range expectedRestore {
								if got, ok := restored[name]; !ok || got != want {
									t.Errorf("restored mismatch %s: %v want %v", name, got, want)
								}
							}
							for name := range restored {
								if _, ok := expectedRestore[name]; !ok {
									t.Errorf("unexpected restored path %s", name)
								}
							}
							t.Fatal("incomplete full archive")
						}
					} else {
						if _, err := VerifyLogical(archive, []age.Identity{identity}); err != nil {
							t.Fatal(err)
						}
						target = fixture(t, git)
						if _, err := target.ImportLogical(archive, []age.Identity{identity}); err != nil {
							t.Fatal(err)
						}
					}
					for name, want := range values {
						got, err := target.Read(name)
						if err != nil || !bytes.Equal(got, want) {
							t.Fatal("lost exported bytes", name, err)
						}
					}
					if kind == "full" {
						_, err = s.ExportFull([]age.Recipient{identity.Recipient()}, output)
					} else {
						_, err = s.ExportLogical(nil, []age.Recipient{identity.Recipient()}, output, io.Discard)
					}
					if err == nil {
						t.Fatal("retry replaced archive")
					}
					again, err := ReadArtifact(output, MaxBundleBytes)
					if err != nil || !bytes.Equal(archive, again) {
						t.Fatal("retry changed archive", err)
					}
				})
			}
		}
	}
}
