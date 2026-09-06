package store

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agensfield/fulla/internal/fault"
)

func TestRestoreCrashHelper(t *testing.T) {
	recoveryPath := os.Getenv("FULLA_RESTORE_RECOVERY_FIXTURE")
	if recoveryPath == "" {
		t.Skip("subprocess fixture")
	}
	recovery, err := Open(recoveryPath, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer recovery.Close()
	identities, _, err := recovery.Keys()
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := ReadArtifact(os.Getenv("FULLA_RESTORE_ARCHIVE_FIXTURE"), MaxBundleBytes)
	if err != nil {
		t.Fatal(err)
	}
	_, err = restoreFullConfirmed(ciphertext, identities, os.Getenv("FULLA_RESTORE_TARGET_FIXTURE"), nil, nil, func(phase string) error {
		if phase == os.Getenv("FULLA_RESTORE_PHASE_FIXTURE") {
			fmt.Println("restore-ready")
			for {
				time.Sleep(time.Hour)
			}
		}
		return nil
	})
	t.Fatalf("restore returned before kill boundary: %v", err)
}

func TestRestoreHandledAndKilledPublicationBoundaries(t *testing.T) {
	for _, git := range []bool{false, true} {
		t.Run(fmt.Sprintf("git=%v", git), func(t *testing.T) {
			source, recovery := fixture(t, git), fixture(t, false)
			if _, err := source.Write("nested/value", []byte{0, 255, 10}, false); err != nil {
				t.Fatal(err)
			}
			if err := source.Root.MkdirAll("passwords/empty/deeper", 0700); err != nil {
				t.Fatal(err)
			}
			expected := archiveTree(t, source, true)
			identities, recipients, err := recovery.Keys()
			if err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(filepath.Dir(source.Dir), "crash.age")
			if _, err := source.ExportFull(recipients, output); err != nil {
				t.Fatal(err)
			}
			ciphertext, err := ReadArtifact(output, MaxBundleBytes)
			if err != nil {
				t.Fatal(err)
			}
			sourceStable := archiveTree(t, source, false)
			for _, existing := range []bool{false, true} {
				for _, phase := range []string{"bound", "file:identities", "validated", "target-vacated", "published", "synced"} {
					if phase == "target-vacated" && !existing {
						continue
					}
					for _, killed := range []bool{false, true} {
						t.Run(fmt.Sprintf("empty=%v/%s/killed=%v", existing, phase, killed), func(t *testing.T) {
							parent, err := filepath.EvalSymlinks(t.TempDir())
							if err != nil {
								t.Fatal(err)
							}
							target := filepath.Join(parent, "target")
							if existing {
								if err := os.Mkdir(target, 0700); err != nil {
									t.Fatal(err)
								}
							}
							published := phase == "published" || phase == "synced"
							if killed {
								cmd := exec.Command(os.Args[0], "-test.run=^TestRestoreCrashHelper$")
								cmd.Env = append(os.Environ(), "FULLA_RESTORE_RECOVERY_FIXTURE="+recovery.Dir, "FULLA_RESTORE_ARCHIVE_FIXTURE="+output, "FULLA_RESTORE_TARGET_FIXTURE="+target, "FULLA_RESTORE_PHASE_FIXTURE="+phase)
								stdout, err := cmd.StdoutPipe()
								if err != nil {
									t.Fatal(err)
								}
								if err := cmd.Start(); err != nil {
									t.Fatal(err)
								}
								defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
								ready := make(chan bool, 1)
								go func() {
									scanner := bufio.NewScanner(stdout)
									ready <- scanner.Scan() && scanner.Text() == "restore-ready"
								}()
								select {
								case ok := <-ready:
									if !ok {
										t.Fatal("child did not reach boundary")
									}
								case <-time.After(25 * time.Second):
									t.Fatal("restore boundary timed out")
								}
								location := target
								if !published {
									paths, err := filepath.Glob(filepath.Join(parent, ".fulla-restore-*"))
									if err != nil || len(paths) != 1 {
										t.Fatal("missing bound stage", err)
									}
									location = paths[0]
								}
								owner, err := PermissionLock(location)
								if err != nil || owner == nil || owner.RestoreID == "" || owner.RestoreTarget != target {
									t.Fatal("missing restore ownership", err)
								}
								if _, err := RecoverCreation(location, owner.Token); err == nil {
									t.Fatal("stole live restore")
								}
								if err := cmd.Process.Kill(); err != nil {
									t.Fatal(err)
								}
								_ = cmd.Wait()
							} else {
								injected := errors.New("fixture restore interruption")
								_, err := restoreFullConfirmed(ciphertext, identities, target, nil, nil, func(at string) error {
									if at == phase {
										return injected
									}
									return nil
								})
								if published {
									var problem *fault.Error
									if !errors.As(err, &problem) || problem.Status != 3 {
										t.Fatalf("lost applied-state error: %v", err)
									}
								} else if !errors.Is(err, injected) {
									t.Fatalf("lost pre-publication error: %v", err)
								}
							}
							stages, err := filepath.Glob(filepath.Join(parent, ".fulla-restore-*"))
							if err != nil {
								t.Fatal(err)
							}
							var orphan map[string]archiveTreeEntry
							if killed && !published {
								if len(stages) != 1 {
									t.Fatalf("expected one private orphan: %v", stages)
								}
								root, err := os.OpenRoot(stages[0])
								if err != nil {
									t.Fatal(err)
								}
								orphan = archiveTree(t, &Store{Root: root}, false)
								_ = root.Close()
								for name, entry := range orphan {
									mode := os.FileMode(0600)
									if entry.Mode.IsDir() {
										mode = os.ModeDir | 0700
									}
									if entry.Mode != mode {
										t.Fatalf("unsafe stage mode for %s: %v", name, entry.Mode)
									}
								}
							} else if len(stages) != 0 {
								t.Fatalf("handled/published restore left staging: %v", stages)
							}
							if published {
								restored, err := Open(target, true, nil)
								if err != nil {
									t.Fatal(err)
								}
								defer restored.Close()
								owner, err := restored.InspectLock()
								if err != nil || owner == nil || owner.RestoreID == "" || owner.RestoreStoreID != source.Meta.StoreID {
									t.Fatal("lost published binding", err)
								}
								if killed {
									reused := filepath.Join(parent, ".fulla-restore-"+owner.RestoreID)
									if err := os.Mkdir(reused, 0700); err != nil {
										t.Fatal(err)
									}
									if err := os.WriteFile(filepath.Join(reused, "new-occupant"), []byte("preserved"), 0600); err != nil {
										t.Fatal(err)
									}
									if _, err := RecoverCreation(target, "wrong-token"); err == nil {
										t.Fatal("accepted wrong restore token")
									}
									result, err := RecoverCreation(target, owner.Token)
									if err != nil || result["restore_applied"] != true {
										t.Fatal("published recovery failed", result, err)
									}
									value, err := os.ReadFile(filepath.Join(reused, "new-occupant"))
									if err != nil || string(value) != "preserved" {
										t.Fatal("recovery deleted reused stage", err)
									}
								} else if _, err := RecoverCreation(target, owner.Token); err == nil {
									t.Fatal("recovered live handled owner")
								}
								actual := archiveTree(t, restored, false)
								if !killed {
									// The handled failure retains exactly the verified owned lock.
									for name := range actual {
										if name == "lock" || strings.HasPrefix(name, "lock/") {
											delete(actual, name)
										}
									}
								}
								if !reflect.DeepEqual(expected, actual) {
									t.Fatal("published restore is not exact complete state")
								}
								if err := restored.DeepVerify(); err != nil {
									t.Fatal(err)
								}
								stable := archiveTree(t, restored, false)
								if _, err := RestoreFull(ciphertext, identities, target); err == nil {
									t.Fatal("retry overwrote published store")
								}
								if !reflect.DeepEqual(stable, archiveTree(t, restored, false)) {
									t.Fatal("refused retry changed published state")
								}
							} else {
								if existing && phase != "target-vacated" {
									entries, err := os.ReadDir(target)
									if err != nil || len(entries) != 0 {
										t.Fatal("pre-publication target changed", err)
									}
								} else if _, err := os.Lstat(target); !os.IsNotExist(err) {
									t.Fatal("partial target became visible", err)
								}
								if _, err := RestoreFull(ciphertext, identities, target); err != nil {
									t.Fatal("fresh restore retry failed", err)
								}
								restored, err := Open(target, true, nil)
								if err != nil {
									t.Fatal(err)
								}
								if !reflect.DeepEqual(expected, archiveTree(t, restored, false)) {
									t.Fatal("retry lost complete state")
								}
								_ = restored.Close()
								if orphan != nil {
									root, err := os.OpenRoot(stages[0])
									if err != nil {
										t.Fatal(err)
									}
									if !reflect.DeepEqual(orphan, archiveTree(t, &Store{Root: root}, false)) {
										t.Fatal("retry silently mutated orphan evidence")
									}
									_ = root.Close()
									owner, err := PermissionLock(stages[0])
									if err != nil || owner == nil {
										t.Fatal("missing orphan ownership", err)
									}

									if phase == "validated" && os.Geteuid() != 0 {
										ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
										defer cancel()
										recovery := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCreationCleanupFailureHelper$")
										recovery.Env = append(os.Environ(), "FULLA_CREATION_RECOVERY_DENY="+stages[0])
										output, err := recovery.CombinedOutput()
										if err != nil {
											t.Fatalf("restore cleanup failure helper: %v %s", err, output)
										}
										if err := os.Chmod(stages[0], 0700); err != nil {
											t.Fatal(err)
										}
										again, err := PermissionLock(stages[0])
										if err != nil || again == nil || again.Token == owner.Token || again.RestoreID != owner.RestoreID || again.RestoreTarget != target || again.RestoreStoreID != source.Meta.StoreID || again.Alive {
											t.Fatal("lost replacement restore ownership", err)
										}
										if _, err := RecoverCreation(stages[0], owner.Token); err == nil {
											t.Fatal("accepted stale restore token")
										}
										owner = again
									}
									targetRoot, err := os.OpenRoot(target)
									if err != nil {
										t.Fatal(err)
									}
									targetStore := &Store{Dir: target, Root: targetRoot}
									targetBefore := archiveTree(t, targetStore, false)
									result, err := RecoverCreation(stages[0], owner.Token)
									if err != nil || result["restore_applied"] != false {
										t.Fatal("unpublished recovery failed", result, err)
									}
									if _, err := os.Lstat(stages[0]); !os.IsNotExist(err) {
										t.Fatal("owned orphan retained", err)
									}
									if !reflect.DeepEqual(targetBefore, archiveTree(t, targetStore, false)) {
										t.Fatal("stage recovery changed independently restored target")
									}
									targetRoot.Close()
								}
							}
							if !reflect.DeepEqual(sourceStable, archiveTree(t, source, false)) {
								t.Fatal("restore mutated archive source")
							}
						})
					}
				}
			}
		})
	}
}
