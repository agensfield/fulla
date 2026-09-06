package store

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestTransactionCrashHelper(t *testing.T) {
	dir := os.Getenv("FULLA_TRANSACTION_FIXTURE")
	if dir == "" {
		return
	}
	s, err := Open(dir, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if os.Getenv("FULLA_TRANSACTION_RECOVER") == "true" {
		owner, err := s.InspectLock()
		if err != nil || owner == nil {
			t.Fatal("no recovery owner", err)
		}
		if _, err := s.Recover(owner.Token); err != nil {
			t.Fatal(err)
		}
		return
	}
	_, recipients, err := s.Keys()
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := crypt.Encrypt([]byte{0, 255, 10}, recipients)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := s.Lock("fixture move")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.mutate(lock, "fixture move", map[string][]byte{"a": nil, "b": ciphertext}, func(phase string) error {
		if phase == os.Getenv("FULLA_TRANSACTION_PHASE") {
			fmt.Println("transaction-ready")
			time.Sleep(time.Minute)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestKilledTransactionRecoveryAndChangedGitFilter(t *testing.T) {
	for _, git := range []bool{false, true} {
		for _, phase := range []string{"journaled", "published:a", "published:b", "committed", "receipted"} {
			t.Run(fmt.Sprintf("git=%t/%s", git, phase), func(t *testing.T) {
				s := fixture(t, git)
				if _, err := s.Write("a", []byte("original"), false); err != nil {
					t.Fatal(err)
				}
				env := append(os.Environ(), "FULLA_TRANSACTION_FIXTURE="+s.Dir, "FULLA_TRANSACTION_PHASE="+phase)
				cmd := exec.Command(os.Args[0], "-test.run=^TestTransactionCrashHelper$")
				cmd.Env = env
				output, err := cmd.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
				ready := make(chan bool, 1)
				go func() {
					scanner := bufio.NewScanner(output)
					ready <- scanner.Scan() && scanner.Text() == "transaction-ready"
				}()
				select {
				case ok := <-ready:
					if !ok {
						t.Fatal("transaction exited before boundary")
					}
				case <-time.After(20 * time.Second):
					t.Fatal("transaction did not reach boundary")
				}
				owner, err := s.InspectLock()
				if err != nil || owner == nil || !owner.Alive {
					t.Fatal("missing live owner", err)
				}
				if _, err := s.Recover(owner.Token); err == nil {
					t.Fatal("recovered a live transaction")
				}
				if err := cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				_ = cmd.Wait()
				if _, err := s.Read("a"); err == nil {
					t.Fatal("interrupted store allowed reads")
				}
				if git && phase == "published:a" {
					// A separate recovery process must fail without running the new
					// filter, then leave a recoverable owner token when it exits.
					head, err := s.Head()
					if err != nil {
						t.Fatal(err)
					}
					marker := filepath.Join(t.TempDir(), "filter-ran")
					quoted := "'" + strings.ReplaceAll(marker, "'", "'\"'\"'") + "'"
					if _, err := s.Git("config", "filter.fixture.clean", "printf invoked > "+quoted+"; printf corrupted"); err != nil {
						t.Fatal(err)
					}
					if err := securefs.WriteNew(s.Root, "passwords/.git/info/attributes", []byte("b.age filter=fixture\n")); err != nil {
						t.Fatal(err)
					}
					recovery := exec.Command(os.Args[0], "-test.run=^TestTransactionCrashHelper$")
					recovery.Env = append(env, "FULLA_TRANSACTION_RECOVER=true")
					if err := recovery.Run(); err == nil {
						t.Fatal("recovery accepted a changed conversion rule")
					}
					if _, err := os.Stat(marker); !os.IsNotExist(err) {
						t.Fatal("recovery invoked the filter", err)
					}
					if after, err := s.Head(); err != nil || after != head {
						t.Fatal("failed recovery changed Git head", err)
					}
					if err := s.Root.Remove("passwords/.git/info/attributes"); err != nil {
						t.Fatal(err)
					}
				}
				owner, err = s.InspectLock()
				if err != nil || owner == nil || owner.Alive {
					t.Fatal("failed recovery lost stale ownership", err)
				}
				result, err := s.Recover(owner.Token)
				if err != nil || result["recovered"] != true || result["lock_released"] != true {
					t.Fatal("could not recover killed transaction", result, err)
				}
				if exists, err := s.Exists("a"); err != nil || exists {
					t.Fatal("recovered move retained source", err)
				}
				got, err := s.Read("b")
				if err != nil || !bytes.Equal(got, []byte{0, 255, 10}) {
					t.Fatal("recovered move lost exact destination bytes", err)
				}
				if _, err := s.CleanGit(); err != nil {
					t.Fatal("recovered store is not clean", err)
				}
				id, ok := result["transaction"].(string)
				if !ok || !validID(id) {
					t.Fatal("missing recovered transaction")
				}
				backup, err := s.BackupShow(id)
				if err != nil || backup.Phase != "committed" || len(backup.Changes) != 2 {
					t.Fatal("incomplete recovery backup", err)
				}
				if _, err := securefs.Read(s.Root, metadata+"/receipts/"+id+".json", maxMetadata); err != nil {
					t.Fatal("missing recovery receipt", err)
				}
			})
		}
	}
}
