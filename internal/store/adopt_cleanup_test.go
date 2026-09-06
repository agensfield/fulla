package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestAdoptionCleanupAndAppliedEvidence(t *testing.T) {
	for _, git := range []bool{false, true} {
		for _, scenario := range []string{"handled", "stage-denied", "lock-before", "lock-after", "changed-owner", "reused"} {
			t.Run(fmt.Sprintf("git=%v/%s", git, scenario), func(t *testing.T) {
				if os.Geteuid() == 0 && (scenario == "stage-denied" || strings.HasPrefix(scenario, "lock-")) {
					t.Skip("requires filesystem permission denial")
				}
				s := fixture(t, git)
				if _, err := s.Write("sample", []byte{0, 255, 10}, false); err != nil {
					t.Fatal(err)
				}
				if err := s.Root.RemoveAll(metadata); err != nil {
					t.Fatal(err)
				}
				before := archiveTree(t, s, false)
				stage := ""
				t.Cleanup(func() {
					if stage != "" {
						_ = s.Root.Chmod(stage, 0700)
					}
					_ = s.Root.Chmod("lock", 0700)
				})
				cause := fault.New("input.cancelled", "private callback sentinel")
				cause.Status = 130
				_, err := adopt(s.Dir, false, nil, func(phase string) error {
					if phase == "locked" && scenario == "lock-before" {
						if err := s.Root.Chmod("lock", 0500); err != nil {
							t.Fatal(err)
						}
						return cause
					}
					if phase == "staged" {
						stages, err := filepath.Glob(filepath.Join(s.Dir, ".fulla-adopt-*"))
						if err != nil || len(stages) != 1 {
							t.Fatal("missing stage", err)
						}
						stage = filepath.Base(stages[0])
						switch scenario {
						case "stage-denied":
							if err := s.Root.Chmod(stage, 0500); err != nil {
								t.Fatal(err)
							}
						case "changed-owner":
							if err := securefs.Replace(s.Root, "lock/owner", []byte("replacement-owner\n")); err != nil {
								t.Fatal(err)
							}
						}
						if scenario != "reused" && scenario != "lock-after" {
							return cause
						}
					}
					if phase == "published" {
						if scenario == "lock-after" {
							if err := s.Root.Chmod("lock", 0500); err != nil {
								t.Fatal(err)
							}
						} else {
							if err := s.Root.Mkdir(stage, 0700); err != nil {
								t.Fatal(err)
							}
							if err := securefs.WriteNew(s.Root, stage+"/new-occupant", []byte("preserved")); err != nil {
								t.Fatal(err)
							}
						}
						return cause
					}
					return nil
				})
				var failure *fault.Error
				if !errors.As(err, &failure) {
					t.Fatal("missing typed refusal", err)
				}
				published := scenario == "reused" || scenario == "lock-after"
				cleanupFailure := scenario != "handled" && scenario != "reused"
				if cleanupFailure {
					wantStatus := 130
					if published {
						wantStatus = 3
					}
					if failure.Code != "store.cleanup_failed" || failure.Status != wantStatus || failure.Details["applied"] != published || failure.Details["cleanup_required"] != true {
						t.Fatal("wrong cleanup evidence", failure)
					}
					wantsLock := strings.HasPrefix(scenario, "lock-") || scenario == "changed-owner" || scenario == "stage-denied"
					if (failure.Details["lock_cleanup_required"] == true) != wantsLock {
						t.Fatal("wrong lock cleanup evidence")
					}
					wantsStage := scenario == "stage-denied" || scenario == "changed-owner"
					if wantsStage {
						if failure.Details["staging_path"] != filepath.Join(s.Dir, stage) {
							t.Fatal("wrong stage path")
						}
						if _, err := s.Root.Stat(stage + "/store.json"); err != nil {
							t.Fatal("lost retained metadata", err)
						}
					} else if _, exists := failure.Details["staging_path"]; exists {
						t.Fatal("claimed unrelated staging")
					}
				} else if scenario == "handled" && err != cause {
					t.Fatal("lost original cancellation", err)
				} else if published && (failure.Status != 3 || failure.Details["applied"] != true) {
					t.Fatal("lost applied state", err)
				}
				encoded, marshalErr := json.Marshal(failure)
				if marshalErr != nil || strings.Contains(string(encoded), "sentinel") && scenario != "handled" {
					t.Fatal("leaked callback diagnostic", marshalErr)
				}
				if published {
					if _, err := s.Root.Stat(metadata + "/store.json"); err != nil {
						t.Fatal("missing published metadata", err)
					}
				} else if _, err := s.Root.Lstat(metadata); !os.IsNotExist(err) {
					t.Fatal("published refused adoption", err)
				}
				if scenario == "reused" {
					value, err := securefs.Read(s.Root, stage+"/new-occupant", 100)
					if err != nil || string(value) != "preserved" {
						t.Fatal("deleted reused stage name", err)
					}
				}
				if scenario == "changed-owner" {
					owner, err := securefs.Read(s.Root, "lock/owner", 100)
					if err != nil || string(owner) != "replacement-owner\n" {
						t.Fatal("changed replacement lock", err)
					}
				}
				if scenario == "handled" || scenario == "reused" {
					if _, err := s.Root.Lstat("lock"); !os.IsNotExist(err) {
						t.Fatal("retained releasable lock", err)
					}
				}
				after := archiveTree(t, s, false)
				for name := range after {
					if name == metadata || strings.HasPrefix(name, metadata+"/") || strings.HasPrefix(name, ".fulla-adopt-") || name == "lock" || strings.HasPrefix(name, "lock/") {
						delete(after, name)
					}
				}
				if !reflect.DeepEqual(before, after) {
					t.Fatal("changed live pa material or history")
				}
			})
		}
	}
}
