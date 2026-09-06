package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"filippo.io/age"
	"github.com/agensfield/fulla/internal/securefs"
)

func stagedExportFixture(t *testing.T, git bool) (*Store, string, *LockInfo) {
	t.Helper()
	s := fixture(t, git)
	if _, err := s.Write("entry", []byte{0, 255, 10, 42}, false); err != nil {
		t.Fatal(err)
	}
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(parent, "archive.age")
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestExportCrashHelper$")
	cmd.Env = append(os.Environ(), "FULLA_EXPORT_CRASH_STORE="+s.Dir, "FULLA_EXPORT_CRASH_KIND=logical", "FULLA_EXPORT_CRASH_PHASE=staged", "FULLA_EXPORT_CRASH_OUTPUT="+output, "FULLA_EXPORT_CRASH_RECIPIENT="+identity.Recipient().String(), "FULLA_EXPORT_CRASH_HANDLED=true")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("staging child: %v %s", err, output)
	}
	owner, err := s.InspectLock()
	if err != nil || owner == nil || owner.Alive || owner.ExportReceipt == "" {
		t.Fatal("missing export ownership", err)
	}
	return s, output, owner
}

func TestExportRecoveryRejectsChangedEvidence(t *testing.T) {
	for _, git := range []bool{false, true} {
		for _, attack := range []string{"output-mismatch", "stage-symlink", "stage-hardlink", "future-version", "inside-store", "wrong-id", "competing-binding"} {
			t.Run(fmt.Sprintf("git=%t/%s", git, attack), func(t *testing.T) {
				s, output, owner := stagedExportFixture(t, git)
				stage := filepath.Join(filepath.Dir(output), exportStage(owner.ExportReceipt))
				switch attack {
				case "output-mismatch":
					if err := os.WriteFile(output, []byte("unrelated output"), 0600); err != nil {
						t.Fatal(err)
					}
				case "stage-symlink", "stage-hardlink":
					original := stage + "-original"
					if err := os.Rename(stage, original); err != nil {
						t.Fatal(err)
					}
					var err error
					if attack == "stage-symlink" {
						err = os.Symlink(original, stage)
					} else {
						err = os.Link(original, stage)
					}
					if err != nil {
						t.Fatal(err)
					}
				case "competing-binding":
					info, err := securefs.Read(s.Root, "lock/info", 4096)
					if err != nil {
						t.Fatal(err)
					}
					if err := securefs.Replace(s.Root, "lock/info", append(bytes.TrimSpace(info), []byte(" peer_receipt="+securefs.ID()+"\n")...)); err != nil {
						t.Fatal(err)
					}
				default:
					data, err := securefs.Read(s.Root, exportReceiptPath(owner.ExportReceipt), maxMetadata)
					if err != nil {
						t.Fatal(err)
					}
					var receipt exportReceipt
					if err := json.Unmarshal(data, &receipt); err != nil {
						t.Fatal(err)
					}
					switch attack {
					case "future-version":
						receipt.Export.Version = 99
					case "inside-store":
						receipt.Export.Output = filepath.Join(s.Dir, "identities")
					case "wrong-id":
						receipt.Export.ID = securefs.ID()
					}
					data, err = json.Marshal(receipt)
					if err != nil {
						t.Fatal(err)
					}
					if err := securefs.Replace(s.Root, exportReceiptPath(owner.ExportReceipt), data); err != nil {
						t.Fatal(err)
					}
				}
				before := archiveTree(t, s, false)
				stageInfo, err := os.Lstat(stage)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := s.Recover(owner.Token); err == nil {
					t.Fatal("accepted changed export evidence")
				}
				if !reflect.DeepEqual(before, archiveTree(t, s, false)) {
					t.Fatal("refusal changed store or owner")
				}
				afterInfo, err := os.Lstat(stage)
				if err != nil || !os.SameFile(stageInfo, afterInfo) {
					t.Fatal("refusal replaced staging", err)
				}
			})
		}
	}
}

func TestExportRecoveryCleansPartialBoundStage(t *testing.T) {
	for _, git := range []bool{false, true} {
		t.Run(fmt.Sprintf("git=%t", git), func(t *testing.T) {
			s, output, owner := stagedExportFixture(t, git)
			stage := filepath.Join(filepath.Dir(output), exportStage(owner.ExportReceipt))
			if err := os.Truncate(stage, 17); err != nil {
				t.Fatal(err)
			} // Reconstructed interrupted write, not a kill during Write.
			result, err := s.Recover(owner.Token)
			if err != nil || result["applied"] != false || result["phase"] != "aborted" {
				t.Fatal("partial stage recovery", result, err)
			}
			entries, err := os.ReadDir(filepath.Dir(output))
			if err != nil || len(entries) != 0 {
				t.Fatal("staging residue", err)
			}
			identity, err := age.GenerateX25519Identity()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.ExportLogical(nil, []age.Recipient{identity.Recipient()}, output, nil); err != nil {
				t.Fatal("could not retry aborted export", err)
			}
		})
	}
}

func TestExportRecoveryRetainsBindingAfterCleanupDenial(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires permission enforcement")
	}
	for _, git := range []bool{false, true} {
		t.Run(fmt.Sprintf("git=%t", git), func(t *testing.T) {
			s, output, owner := stagedExportFixture(t, git)
			parent := filepath.Dir(output)
			t.Cleanup(func() { _ = os.Chmod(parent, 0700) })
			cmd := exec.Command(os.Args[0], "-test.run=^TestExportCrashHelper$")
			cmd.Env = append(os.Environ(), "FULLA_EXPORT_CRASH_STORE="+s.Dir, "FULLA_EXPORT_CRASH_OUTPUT="+output, "FULLA_EXPORT_RECOVERY_DENIAL="+owner.Token)
			response, err := cmd.CombinedOutput()
			if err != nil || !bytes.Contains(response, []byte("export-recovery-retained")) {
				t.Fatalf("recovery fixture failed: %v %s", err, response)
			}
			replacement, err := s.InspectLock()
			if err != nil || replacement == nil || replacement.Alive || replacement.Token == owner.Token || replacement.ExportReceipt != owner.ExportReceipt {
				t.Fatal("lost replacement export ownership", err)
			}
			if err := os.Chmod(parent, 0700); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Recover(owner.Token); err == nil {
				t.Fatal("accepted superseded token")
			}
			result, err := s.Recover(replacement.Token)
			if err != nil || result["phase"] != "aborted" || result["lock_released"] != true {
				t.Fatal("replacement-token retry failed", result, err)
			}
			entries, err := os.ReadDir(parent)
			if err != nil || len(entries) != 0 {
				t.Fatal("retained external stage", err)
			}
		})
	}
}

func TestExportPreviousBinaryRefusesNewRecovery(t *testing.T) {
	binary := os.Getenv("FULLA_EXPORT_PREVIOUS_BINARY")
	if binary == "" {
		t.Skip("explicit historical binary acceptance")
	}
	for _, git := range []bool{false, true} {
		t.Run(fmt.Sprintf("git=%t", git), func(t *testing.T) {
			s, output, owner := stagedExportFixture(t, git)
			before := archiveTree(t, s, false)
			stage := filepath.Join(filepath.Dir(output), exportStage(owner.ExportReceipt))
			staged, err := os.ReadFile(stage)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(binary, "--store", s.Dir, "doctor", "--recover-lock", owner.Token, "--json")
			response, err := cmd.CombinedOutput()
			if err == nil || !bytes.Contains(response, []byte("store.lock_unknown")) {
				t.Fatalf("older reader failed to refuse: %v %s", err, response)
			}
			if !reflect.DeepEqual(before, archiveTree(t, s, false)) {
				t.Fatal("older reader changed ownership or store")
			}
			after, err := os.ReadFile(stage)
			if err != nil || !bytes.Equal(after, staged) {
				t.Fatal("older reader changed bound stage", err)
			}
			if _, err := s.Recover(owner.Token); err != nil {
				t.Fatal("current recovery failed after older refusal", err)
			}
		})
	}
}
