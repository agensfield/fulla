package store

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
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
		if strings.HasPrefix(phase, "publication-staged:"+metadata+"/retired/") {
			phase = "publication-staged:retired"
		}
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
	for _, git := range []bool{false, true} {
		for _, destroy := range []bool{false, true} {
			for _, phase := range []string{"journaled", "publication-staged:identities", "publication-staged:retired", "published:passwords/entry.age", "published:recipients", "published:identities", "committed", "cleaned"} {
				if !git && !strings.HasPrefix(phase, "publication-staged:") {
					continue
				}
				t.Run(fmt.Sprintf("git=%t/destroy=%t/%s", git, destroy, phase), func(t *testing.T) {
					s := fixture(t, git)
					value := []byte{0, 255, 10, 65, 10}
					if _, err := s.Write("entry", value, false); err != nil {
						t.Fatal(err)
					}
					historyCommit := ""
					if git {
						history, err := s.History("entry")
						if err != nil {
							t.Fatal(err)
						}
						historyCommit = history[0].Commit
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
					if strings.HasPrefix(phase, "publication-staged:") {
						stage := metadata + "/transactions/" + lock.StageID
						paths, err := s.inspectStaging()
						if err != nil || len(paths) != 1 || paths[0] != stage {
							t.Fatal("publication copy escaped its bound transaction", err, paths)
						}
						journalData, err := securefs.Read(s.Root, metadata+"/rotation.json", maxMetadata)
						if err != nil {
							t.Fatal(err)
						}
						var journal Rotation
						if err := StrictJSON(journalData, &journal); err != nil {
							t.Fatal(err)
						}
						target := "identities"
						if phase == "publication-staged:retired" {
							target = journal.Retired
						}
						original, err := securefs.Read(s.Root, stage+"/after/"+target, 65<<20)
						if err != nil {
							t.Fatal("publication consumed retry source", err)
						}
						entries, err := fs.ReadDir(s.Root.FS(), stage)
						if err != nil {
							t.Fatal(err)
						}
						copies := 0
						for _, entry := range entries {
							if strings.HasPrefix(entry.Name(), "publish-") {
								copy, err := securefs.Read(s.Root, stage+"/"+entry.Name(), 65<<20)
								if err != nil || !bytes.Equal(original, copy) {
									t.Fatal("publication copy mismatch", err)
								}
								copies++
							}
						}
						if copies != 1 {
							t.Fatal("expected one owned publication copy", copies)
						}
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
					if git {
						if _, err := s.HistoryRestore(historyCommit, "entry"); (err != nil) != destroy {
							t.Fatal("recovered retirement violated history policy", err)
						}
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
}
