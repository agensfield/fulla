package store

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/agensfield/fulla/internal/securefs"
)

func TestRotationCrashHelper(t *testing.T) {
	dir := os.Getenv("FULLA_ROTATION_FIXTURE")
	if dir == "" {
		return
	}
	s, err := Open(dir, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	old, err := s.IdentityShow()
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.rotate(os.Getenv("FULLA_ROTATION_DESTROY") == "true", "destroy-retired-key:"+old.Fingerprint, false, func(phase string) error {
		if phase == os.Getenv("FULLA_ROTATION_PHASE") {
			fmt.Println("rotation-ready")
			time.Sleep(time.Minute)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRotationKilledOwnerRecovery(t *testing.T) {
	for _, destroy := range []bool{false, true} {
		for _, phase := range []string{"published:passwords/entry.age", "published:recipients", "published:identities", "committed", "cleaned"} {
			t.Run(fmt.Sprintf("destroy=%t/%s", destroy, phase), func(t *testing.T) {
				s := fixture(t, true)
				value := []byte{0, 255, 10, 65, 10}
				if _, err := s.Write("entry", value, false); err != nil {
					t.Fatal(err)
				}
				history, err := s.History("entry")
				if err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command(os.Args[0], "-test.run=^TestRotationCrashHelper$")
				cmd.Env = append(os.Environ(), "FULLA_ROTATION_FIXTURE="+s.Dir, "FULLA_ROTATION_PHASE="+phase, fmt.Sprintf("FULLA_ROTATION_DESTROY=%t", destroy))
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
					ready <- scanner.Scan() && scanner.Text() == "rotation-ready"
				}()
				select {
				case ok := <-ready:
					if !ok {
						t.Fatal("rotation exited before crash boundary")
					}
				case <-time.After(20 * time.Second):
					t.Fatal("rotation did not reach crash boundary")
				}
				lock, err := s.InspectLock()
				if err != nil || lock == nil || !lock.Alive {
					t.Fatal("missing live rotation owner", err)
				}
				if _, err := s.Recover(lock.Token); err == nil {
					t.Fatal("recovery stole live rotation")
				}
				if err := cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				_ = cmd.Wait()
				if _, err := s.Read("entry"); err == nil {
					t.Fatal("interrupted rotation allowed ordinary reads")
				}
				if _, err := s.Recover("wrong-token"); err == nil {
					t.Fatal("recovery accepted wrong owner token")
				}
				result, err := s.Recover(lock.Token)
				if err != nil || result["recovered"] != true || result["lock_released"] != true {
					t.Fatal("killed rotation recovery failed", result, err)
				}
				got, err := s.Read("entry")
				if err != nil || !bytes.Equal(got, value) {
					t.Fatal("recovery lost exact live bytes", err)
				}
				if _, err := s.HistoryRestore(history[0].Commit, "entry"); (err != nil) != destroy {
					t.Fatal("recovered retirement violated history policy", err)
				}
				staging, err := fs.ReadDir(s.Root.FS(), metadata+"/transactions")
				if err != nil || len(staging) != 0 {
					t.Fatal("recovery left private identity staging", err)
				}
				id, ok := result["transaction"].(string)
				if !ok || !validID(id) {
					t.Fatal("missing recovered transaction identifier")
				}
				receipt, err := securefs.Read(s.Root, metadata+"/receipts/"+id+".json", maxMetadata)
				if err != nil {
					t.Fatal(err)
				}
				var record map[string]any
				if err := StrictJSON(receipt, &record); err != nil || record["applied"] != true || record["destroyed"] != destroy {
					t.Fatal("incorrect recovered rotation receipt", err)
				}
			})
		}
	}
}
