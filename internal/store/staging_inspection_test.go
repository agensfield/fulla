package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/agensfield/fulla/internal/securefs"
)

func TestDoctorReportsUnjournaledKilledWriterStaging(t *testing.T) {
	for _, git := range []bool{false, true} {
		for _, rotation := range []bool{false, true} {
			t.Run(fmt.Sprintf("git=%t/rotation=%t", git, rotation), func(t *testing.T) {
				s := fixture(t, git)
				for _, name := range []string{"a", "entry"} {
					if _, err := s.Write(name, []byte("unchanged"), false); err != nil {
						t.Fatal(err)
					}
				}
				helper, prefix, readyText := "TestTransactionCrashHelper", "FULLA_TRANSACTION_", "transaction-ready"
				if rotation {
					helper, prefix, readyText = "TestRotationCrashHelper", "FULLA_ROTATION_", "rotation-ready"
				}
				cmd := exec.Command(os.Args[0], "-test.run=^"+helper+"$")
				cmd.Env = append(os.Environ(), prefix+"FIXTURE="+s.Dir, prefix+"PHASE=staged")
				pipe, err := cmd.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
				ready := make(chan bool, 1)
				go func() { scanner := bufio.NewScanner(pipe); ready <- scanner.Scan() && scanner.Text() == readyText }()
				select {
				case ok := <-ready:
					if !ok {
						t.Fatal("writer did not stage")
					}
				case <-time.After(20 * time.Second):
					t.Fatal("writer readiness timed out")
				}
				live, err := s.Doctor(false)
				if err != nil || live.Healthy || len(live.Staging) != 1 || live.Lock == nil || !live.Lock.Alive {
					t.Fatal("live staging not visible", err)
				}
				if _, err := s.Recover(live.Lock.Token); err == nil {
					t.Fatal("recovery stole live writer")
				}
				if err := cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				_ = cmd.Wait()
				result, err := s.Recover(live.Lock.Token)
				if err != nil || result["lock_released"] != true || result["recovered"] != false {
					t.Fatal("unjournaled recovery should only release dead lock", result, err)
				}
				before := transactionFiles(t, s)
				report, err := s.Doctor(false)
				if err != nil || report.Healthy || report.Lock != nil || !reflect.DeepEqual(report.Staging, live.Staging) || !slices.Contains(report.Issues, "transaction.staging_present") {
					t.Fatal("unlocked leftover staging hidden by healthy report", report, err)
				}
				if !reflect.DeepEqual(before, transactionFiles(t, s)) {
					t.Fatal("doctor changed leftover evidence")
				}
				if rotation {
					if _, err := s.Root.Lstat(report.Staging[0] + "/after/identities"); err != nil {
						t.Fatal("rotation fixture has no retained private identity", err)
					}
				}
				for _, name := range []string{"a", "entry"} {
					value, err := s.Read(name)
					if err != nil || string(value) != "unchanged" {
						t.Fatal("unjournaled interruption changed live values", err)
					}
				}
			})
		}
	}
}

func TestStagingInspectionRefusesUnknownDomainAndUnexpectedPaths(t *testing.T) {
	for _, kind := range []string{"domain", "filename", "limit"} {
		t.Run(kind, func(t *testing.T) {
			s := fixture(t, false)
			if kind == "domain" {
				s.Meta.Domains["transactions"] = 2
				data, err := json.Marshal(s.Meta)
				if err != nil {
					t.Fatal(err)
				}
				if err := securefs.Replace(s.Root, metadata+"/store.json", data); err != nil {
					t.Fatal(err)
				}
			} else if kind == "limit" {
				for range 1025 {
					if err := s.Root.Mkdir(metadata+"/transactions/"+securefs.ID(), 0700); err != nil {
						t.Fatal(err)
					}
				}
			} else if err := securefs.WriteNew(s.Root, metadata+"/transactions/unexpected", []byte("private-fixture")); err != nil {
				t.Fatal(err)
			}
			before := transactionFiles(t, s)
			report, err := s.Doctor(false)
			if err != nil || report.Healthy || len(report.Staging) != 0 || !slices.Contains(report.Issues, "transaction.staging_unavailable") {
				t.Fatal("unsupported staging inspection appeared healthy", report, err)
			}
			if !reflect.DeepEqual(before, transactionFiles(t, s)) {
				t.Fatal("inspection changed unsupported evidence")
			}
		})
	}
}
