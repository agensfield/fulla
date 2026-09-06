package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestUnpublishedOperationReportsFailedStageCleanup(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("real permission denial requires non-root user")
	}
	for _, git := range []bool{false, true} {
		for _, rotation := range []bool{false, true} {
			for _, kind := range []string{"error", "refusal", "signal"} {
				t.Run(fmt.Sprintf("git=%t/rotation=%t/%s", git, rotation, kind), func(t *testing.T) {
					s := fixture(t, git)
					if _, err := s.Write("entry", []byte("original"), false); err != nil {
						t.Fatal(err)
					}
					before := transactionFiles(t, s)
					stage := ""
					t.Cleanup(func() {
						if stage != "" {
							_ = os.Chmod(filepath.Join(s.Dir, stage), 0700)
							_ = os.Chmod(filepath.Join(s.Dir, stage, "after"), 0700)
						}
					})
					hook := func(phase string) error {
						if phase != "staged" {
							return nil
						}
						entries, err := fs.ReadDir(s.Root.FS(), metadata+"/transactions")
						if err != nil || len(entries) != 1 {
							t.Fatal("missing sole owned stage", err)
						}
						stage = metadata + "/transactions/" + entries[0].Name()
						for _, dir := range []string{stage, stage + "/after"} {
							if err := os.Chmod(filepath.Join(s.Dir, dir), 0500); err != nil {
								t.Fatal(err)
							}
						}
						switch kind {
						case "refusal":
							return fault.Interaction("private-cleanup-fixture-sentinel")
						case "signal":
							err := fault.New("input.cancelled", "private-cleanup-fixture-sentinel")
							err.Status = 130
							return err
						default:
							return errors.New("private-cleanup-fixture-sentinel")
						}
					}
					var err error
					if rotation {
						_, err = s.rotate(false, "", false, hook)
					} else {
						_, rs, e := s.Keys()
						if e != nil {
							t.Fatal(e)
						}
						ciphertext, e := crypt.Encrypt([]byte("replacement"), rs)
						if e != nil {
							t.Fatal(e)
						}
						lock, e := s.Lock("fixture edit")
						if e != nil {
							t.Fatal(e)
						}
						_, err = s.mutate(lock, "fixture edit", map[string][]byte{"entry": ciphertext}, hook)
					}
					var failure *fault.Error
					if !errors.As(err, &failure) || failure.Code != "transaction.cleanup_failed" || failure.Details["applied"] != false || failure.Details["cleanup_required"] != true || failure.Details["staging_cleanup_required"] != true || failure.Details["staging_path"] != filepath.Join(s.Dir, stage) {
						t.Fatal("lost private staging cleanup failure", err)
					}
					expectedStatus := 1
					if kind == "signal" {
						expectedStatus = 130
					}
					if failure.Status != expectedStatus {
						t.Fatal("lost signal status", failure.Status)
					}
					encoded, err := json.Marshal(failure)
					if err != nil || bytes.Contains(encoded, []byte("sentinel")) {
						t.Fatal("raw callback error leaked", err)
					}
					after := transactionFiles(t, s)
					for name, hash := range before {
						if after[name] != hash {
							t.Fatal("failed staging changed existing store file", name)
						}
					}
					for name := range after {
						if _, existed := before[name]; !existed && !strings.HasPrefix(name, stage+"/") && !strings.HasPrefix(name, "lock/") {
							t.Fatal("failure created file outside retained staging", name)
						}
					}
					if rotation {
						if _, err := s.Root.Lstat(stage + "/after/identities"); err != nil {
							t.Fatal("fixture failed to retain staged private identity", err)
						}
					}
					if owner, err := s.InspectLock(); err != nil || owner == nil || owner.StageID != filepath.Base(stage) || failure.Details["lock_retained"] != true || failure.Details["recovery_required"] != true {
						t.Fatal("failed bound cleanup did not retain recovery ownership", err)
					}
					if _, err := s.Read("entry"); err == nil {
						t.Fatal("ordinary read ignored retained recovery lock")
					}
				})
			}
		}
	}
}

func TestUnpublishedCleanupReportsChangedLockWithoutRemovingIt(t *testing.T) {
	s := fixture(t, false)
	lock, err := s.Lock("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if err := securefs.Replace(s.Root, "lock/owner", []byte("replacement-owner\n")); err != nil {
		t.Fatal(err)
	}
	before := transactionFiles(t, s)
	err = s.finishUnpublished(lock, "", errors.New("private-original-error"))
	var failure *fault.Error
	if !errors.As(err, &failure) || failure.Code != "transaction.cleanup_failed" || failure.Details["lock_cleanup_required"] != true || failure.Details["staging_path"] != nil {
		t.Fatal("lost changed-owner cleanup failure", err)
	}
	if !reflect.DeepEqual(before, transactionFiles(t, s)) {
		t.Fatal("cleanup changed another owner's lock")
	}
}
