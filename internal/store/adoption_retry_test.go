package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestAdoptionRecoveryDenialHelper(t *testing.T) {
	dir := os.Getenv("FULLA_ADOPTION_RETRY_DIR")
	if dir == "" {
		return
	}
	s, err := Open(dir, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.recover(os.Getenv("FULLA_ADOPTION_RETRY_TOKEN"), func() error {
		if err := s.Validate(); err != nil {
			return err
		}
		// Inject an actual filesystem denial after ordinary recovery validation.
		// This tests cleanup failure, not unsafe-mode preflight rejection.
		return s.Root.Chmod(os.Getenv("FULLA_ADOPTION_RETRY_DENY"), 0500)
	})
	var failure *fault.Error
	if !errors.As(err, &failure) {
		t.Fatal("missing typed recovery error", err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"status": failure.Status, "error": failure}); err != nil {
		t.Fatal(err)
	}
	os.Exit(1)
}

func TestAdoptionRecoveryFailureRetainsBindingForRetry(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires real filesystem permission denial")
	}
	for _, git := range []bool{false, true} {
		t.Run(fmt.Sprintf("git=%v", git), func(t *testing.T) {
			s := fixture(t, git)
			if _, err := s.Write("sample", []byte{0, 255, 10}, false); err != nil {
				t.Fatal(err)
			}
			if err := s.Root.RemoveAll(metadata); err != nil {
				t.Fatal(err)
			}
			before := archiveTree(t, s, false)
			lock, err := s.Lock("adopt")
			if err != nil {
				t.Fatal(err)
			}
			meta := newMetadata()
			stage := ".fulla-adopt-" + meta.StoreID
			if err := s.Root.Mkdir(stage, 0700); err != nil {
				t.Fatal(err)
			}
			if err := s.bindAdoption(lock, meta.StoreID); err != nil {
				t.Fatal(err)
			}
			if err := writeMetadataContents(s.Root, stage, meta, "init"); err != nil {
				t.Fatal(err)
			}

			// Reconstruct the initial dead-owner fixture using an actually reaped PID.
			// The failed recovery runs in a real subprocess; retry uses ordinary Recover.
			dead := exec.Command("true")
			if err := dead.Run(); err != nil {
				t.Fatal(err)
			}
			host, err := os.Hostname()
			if err != nil {
				t.Fatal(err)
			}
			info := fmt.Sprintf("pid=%d host=%s operation=adopt adoption_id=%s\n", dead.ProcessState.Pid(), host, meta.StoreID)
			if err := securefs.Replace(s.Root, "lock/info", []byte(info)); err != nil {
				t.Fatal(err)
			}
			denied := stage
			t.Cleanup(func() { _ = s.Root.Chmod(denied, 0700) })
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAdoptionRecoveryDenialHelper$")
			cmd.Env = append(os.Environ(), "FULLA_ADOPTION_RETRY_DIR="+s.Dir, "FULLA_ADOPTION_RETRY_TOKEN="+lock.Token, "FULLA_ADOPTION_RETRY_DENY="+denied)
			output, err := cmd.Output()
			exit, ok := err.(*exec.ExitError)
			if !ok || exit.ExitCode() != 1 {
				t.Fatal("recovery helper did not report failure", err)
			}
			var report struct {
				Status int
				Error  fault.Error
			}
			if err := json.Unmarshal(output, &report); err != nil {
				t.Fatal("invalid recovery error", err)
			}
			wantStatus := 1
			if report.Status != wantStatus || report.Error.Code != "store.cleanup_failed" || report.Error.Details["applied"] != false || report.Error.Details["cleanup_required"] != true || report.Error.Details["lock_cleanup_required"] != true || report.Error.Details["staging_path"] != filepath.Join(s.Dir, stage) {
				t.Fatal("wrong applied-state evidence", string(output))
			}
			replacement, err := s.InspectLock()
			if err != nil || replacement == nil || replacement.Alive || replacement.Token == lock.Token || replacement.AdoptionID != meta.StoreID {
				t.Fatal("lost binding after recovery takeover", err)
			}
			if _, err := s.Root.Lstat("lock/recovery"); err != nil {
				t.Fatal("missing retained recovery guard", err)
			}
			// Restore only the deliberately changed fixture mode, then use ordinary
			// validated recovery with the newly inspected dead owner's token.
			if err := s.Root.Chmod(denied, 0700); err != nil {
				t.Fatal(err)
			}
			current, err := Open(s.Dir, false, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer current.Close()
			snapshot := archiveTree(t, s, false)
			if _, err := current.Recover(lock.Token); err == nil {
				t.Fatal("accepted stale pre-takeover token")
			}
			if !reflect.DeepEqual(snapshot, archiveTree(t, s, false)) {
				t.Fatal("stale-token refusal changed evidence")
			}
			result, err := current.Recover(replacement.Token)
			if err != nil || result["adoption_applied"] != false || result["lock_released"] != true {
				t.Fatal("recovery retry failed", result, err)
			}
			after := archiveTree(t, s, false)

			if !reflect.DeepEqual(before, after) {
				t.Fatal("retry changed live material or retained staging/lock")
			}
			if _, err := Adopt(s.Dir, false, nil); err != nil {
				t.Fatal("explicit adoption retry failed", err)
			}
		})
	}
}
