package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/agensfield/fulla/internal/fault"
)

func TestRestoreReportsFailedPrivateStageCleanup(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission denial requires non-root user")
	}
	source, recovery := fixture(t, false), fixture(t, false)
	ids, rs, err := recovery.Keys()
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(filepath.Dir(source.Dir), "cleanup.age")
	if _, err := source.ExportFull(rs, output); err != nil {
		t.Fatal(err)
	}
	ciphertext, err := ReadArtifact(output, MaxBundleBytes)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"error", "refusal", "signal"} {
		t.Run(kind, func(t *testing.T) {
			parent, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(parent, "restored")
			stage := ""
			t.Cleanup(func() {
				if stage != "" {
					_ = os.Chmod(stage, 0700)
				}
			})
			_, err = RestoreFullConfirmed(ciphertext, ids, target, nil, func(ArchiveResult) error {
				stages, err := filepath.Glob(filepath.Join(parent, ".fulla-restore-*"))
				if err != nil || len(stages) != 1 {
					t.Fatal("missing owned stage", err)
				}
				stage = stages[0]
				// Exercise a real filesystem denial, not a callback that pretends the
				// removal failed. Root-mode denial prevents unlinking its private files.
				if err := os.Chmod(stage, 0500); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "refusal":
					return fault.Interaction("fixture private callback sentinel")
				case "signal":
					cancelled := fault.New("input.cancelled", "fixture private callback sentinel")
					cancelled.Status = 130
					return cancelled
				default:
					return errors.New("fixture private callback sentinel")
				}
			})
			var failure *fault.Error
			if !errors.As(err, &failure) || failure.Code != "recovery.cleanup_failed" {
				t.Fatalf("cleanup failure not reported: %v", err)
			}
			expectedStatus := 1
			if kind == "signal" {
				expectedStatus = 130
			}
			if failure.Status != expectedStatus || failure.Details["applied"] != false || failure.Details["cleanup_required"] != true || failure.Details["staging_path"] != stage || failure.Details["target"] != target {
				t.Fatal("wrong cleanup metadata")
			}
			if kind == "refusal" && failure.Details["operation_code"] != "interaction.required" {
				t.Fatal("lost original operation code")
			}
			if kind == "signal" && failure.Details["operation_code"] != "input.cancelled" {
				t.Fatal("lost signal cause")
			}
			encoded, err := json.Marshal(failure)
			if err != nil || bytes.Contains(encoded, []byte("sentinel")) {
				t.Fatal("cleanup diagnostic leaked callback error", err)
			}
			if _, err := os.Lstat(target); !os.IsNotExist(err) {
				t.Fatal("failed cleanup published target", err)
			}
			retained, openErr := os.OpenRoot(stage)
			if openErr != nil {
				t.Fatal(openErr)
			}
			owner, inspectErr := (&Store{Root: retained}).InspectLock()
			retained.Close()
			if inspectErr != nil || owner == nil || owner.RestoreID == "" || owner.RestoreStoreID != source.Meta.StoreID {
				t.Fatal("cleanup lost restore ownership", inspectErr)
			}
			if _, err := os.Stat(filepath.Join(stage, "identities")); err != nil {
				t.Fatal("fixture did not retain private identity material", err)
			}
		})
	}
}

func TestPublishedRestoreDoesNotDeleteReusedStageName(t *testing.T) {
	source, recovery := fixture(t, false), fixture(t, false)
	ids, rs, err := recovery.Keys()
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(filepath.Dir(source.Dir), "reuse.age")
	if _, err := source.ExportFull(rs, output); err != nil {
		t.Fatal(err)
	}
	ciphertext, err := ReadArtifact(output, MaxBundleBytes)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "restored")
	stage := ""
	_, err = restoreFullConfirmed(ciphertext, ids, target, nil, nil, func(phase string) error {
		if phase == "validated" {
			stages, err := filepath.Glob(filepath.Join(parent, ".fulla-restore-*"))
			if err != nil || len(stages) != 1 {
				t.Fatal("missing stage", err)
			}
			stage = stages[0]
		}
		if phase == "published" {
			if err := os.Mkdir(stage, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(stage, "new-occupant"), []byte("preserved"), 0600); err != nil {
				t.Fatal(err)
			}
			return errors.New("fixture post-publication failure")
		}
		return nil
	})
	var failure *fault.Error
	if !errors.As(err, &failure) || failure.Status != 3 {
		t.Fatal("lost applied-state failure", err)
	}
	got, err := os.ReadFile(filepath.Join(stage, "new-occupant"))
	if err != nil || string(got) != "preserved" {
		t.Fatal("deleted unrelated replacement at old stage name", err)
	}
	restored, err := Open(target, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.DeepVerify(); err != nil {
		t.Fatal(err)
	}
}
