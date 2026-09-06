package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/agensfield/fulla/internal/fault"
)

func TestInitializationStageCleanup(t *testing.T) {
	for _, noGit := range []bool{false, true} {
		for _, scenario := range []string{"handled", "denied", "reused"} {
			t.Run(fmt.Sprintf("no-git=%v/%s", noGit, scenario), func(t *testing.T) {
				if scenario == "denied" && os.Geteuid() == 0 {
					t.Skip("requires filesystem permission denial")
				}
				parent, err := filepath.EvalSymlinks(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(parent, "store")
				stage := ""
				t.Cleanup(func() {
					if stage != "" {
						_ = os.Chmod(stage, 0700)
					}
				})
				cause := fault.New("input.cancelled", "private callback sentinel")
				cause.Status = 130
				_, err = initialize(target, noGit, false, func(phase string) error {
					if phase == "staged" {
						stages, err := filepath.Glob(filepath.Join(parent, ".fulla-init-*"))
						if err != nil || len(stages) != 1 {
							t.Fatal("missing stage", err)
						}
						stage = stages[0]
						if scenario == "denied" {
							if err := os.Chmod(stage, 0500); err != nil {
								t.Fatal(err)
							}
						}
						if scenario != "reused" {
							return cause
						}
					}
					if phase == "published" {
						if err := os.Mkdir(stage, 0700); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(stage, "new-occupant"), []byte("preserved"), 0600); err != nil {
							t.Fatal(err)
						}
						return cause
					}
					return nil
				})
				var failure *fault.Error
				if !errors.As(err, &failure) {
					t.Fatal("missing typed failure", err)
				}
				switch scenario {
				case "handled":
					if err != cause {
						t.Fatal("lost original cancellation", err)
					}
					if _, err := os.Lstat(stage); !os.IsNotExist(err) {
						t.Fatal("stage survived successful cleanup", err)
					}
				case "denied":
					if failure.Code != "store.cleanup_failed" || failure.Status != 130 || failure.Details["applied"] != false || failure.Details["cleanup_required"] != true || failure.Details["staging_path"] != stage || failure.Details["target"] != target || failure.Details["operation_code"] != "input.cancelled" {
						t.Fatal("wrong cleanup evidence", failure)
					}
					if _, err := os.Stat(filepath.Join(stage, "identities")); err != nil {
						t.Fatal("fixture did not retain private identity", err)
					}
				case "reused":
					if failure.Status != 3 || failure.Details["applied"] != true {
						t.Fatal("lost applied state", failure)
					}
					value, err := os.ReadFile(filepath.Join(stage, "new-occupant"))
					if err != nil || string(value) != "preserved" {
						t.Fatal("deleted reused stage name", err)
					}
					s, err := Open(target, true, nil)
					if err != nil {
						t.Fatal(err)
					}
					defer s.Close()
					if err := s.DeepVerify(); err != nil {
						t.Fatal(err)
					}
				}
				if scenario != "reused" {
					if _, err := os.Lstat(target); !os.IsNotExist(err) {
						t.Fatal("unpublished initialization created target", err)
					}
				}
			})
		}
	}
}
