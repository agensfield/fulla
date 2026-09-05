package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agensfield/fulla/internal/securefs"
)

func TestKilledTransactionSnapshotDomainRecovery(t *testing.T) {
	for _, git := range []bool{false, true} {
		for _, phase := range []string{"published:a", "committed", "receipted"} {
			t.Run(fmt.Sprintf("git=%v/%s", git, phase), func(t *testing.T) {
				s := fixture(t, git)
				if _, err := s.Write("a", []byte("original"), false); err != nil {
					t.Fatal(err)
				}
				setBackupVersion(t, s, 2)
				before := transactionFiles(t, s)
				cmd := exec.Command(os.Args[0], "-test.run=^TestTransactionCrashHelper$")
				cmd.Env = append(os.Environ(), "FULLA_TRANSACTION_FIXTURE="+s.Dir, "FULLA_TRANSACTION_PHASE="+phase)
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
						t.Fatal("child exited before boundary")
					}
				case <-time.After(20 * time.Second):
					t.Fatal("child did not reach boundary")
				}
				if err := cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				_ = cmd.Wait()
				owner, err := s.InspectLock()
				if err != nil || owner == nil || owner.Alive {
					t.Fatal("missing dead owner", err)
				}
				if _, err := s.Read("a"); err == nil {
					t.Fatal("interrupted operation allowed reads")
				}
				data, err := securefs.Read(s.Root, metadata+"/pending.json", maxMetadata)
				if err != nil {
					t.Fatal(err)
				}
				var j Journal
				if err := StrictJSON(data, &j); err != nil || j.SnapshotDomain != "transactions" {
					t.Fatal("lost snapshot domain binding", err)
				}
				// A changed manifest must not retarget an already journaled snapshot.
				if phase == "committed" {
					setBackupVersion(t, s, 1)
				}
				result, err := s.Recover(owner.Token)
				if err != nil || result["recovered"] != true || result["lock_released"] != true {
					t.Fatal("snapshot recovery failed", result, err)
				}
				after := transactionFiles(t, s)
				for name, hash := range before {
					if strings.HasPrefix(name, metadata+"/backups/") {
						if after[name] != hash {
							t.Fatal("recovery changed backup-domain files", name)
						}
					}
				}
				got, err := s.Read("b")
				if err != nil || !bytes.Equal(got, []byte{0, 255, 10}) {
					t.Fatal("recovery lost exact value", err)
				}
				data, err = securefs.Read(s.Root, snapshotBase("transactions")+"/"+j.ID+"/journal.json", maxMetadata)
				if err != nil {
					t.Fatal(err)
				}
				var snapshot Journal
				if err := json.Unmarshal(data, &snapshot); err != nil || snapshot.SnapshotDomain != "transactions" || snapshot.Phase != "committed" {
					t.Fatal("wrong recovered snapshot", err)
				}
				// Recovery completion is idempotent at the public lock boundary.
				stable := transactionFiles(t, s)
				if _, err := s.Recover(owner.Token); err == nil {
					t.Fatal("accepted nonexistent recovered owner")
				}
				if !reflect.DeepEqual(stable, transactionFiles(t, s)) {
					t.Fatal("retry changed completed state")
				}
			})
		}
	}
}
