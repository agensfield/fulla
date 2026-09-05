package store

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"

	"github.com/agensfield/fulla/internal/securefs"
)

func TestPruneRetentionPreviewAndPreservation(t *testing.T) {
	s := fixture(t, false)
	for i := range 3 {
		if _, err := s.Write(fmt.Sprintf("entry%d", i), []byte{byte(i), 0, 255}, false); err != nil {
			t.Fatal(err)
		}
	}
	before, err := s.Backups()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func() map[string]string {
		out := map[string]string{}
		if err := fs.WalkDir(s.Root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			out[name] = info.Mode().String()
			if !entry.IsDir() {
				data, err := s.Root.ReadFile(name)
				if err != nil {
					return err
				}
				out[name] += fmt.Sprintf("%x", sha256.Sum256(data))
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	beforePreview := snapshot()
	plan, err := s.Prune(Retention{Keep: retain(1)}, true)
	if err != nil || len(plan.Candidates) != 2 || plan.Retained != 1 || len(plan.Deleted) != 0 {
		t.Fatal(plan, err)
	}
	if !reflect.DeepEqual(beforePreview, snapshot()) {
		t.Fatal("preview mutated store contents or modes")
	}
	after, err := s.Backups()
	if err != nil || len(after) != 3 {
		t.Fatal("preview changed backups", err)
	}
	if err := s.Unlocked(); err != nil {
		t.Fatal(err)
	}
	plan, err = s.Prune(Retention{Keep: retain(0), OlderThan: time.Hour}, false)
	if err != nil || len(plan.Candidates) != 0 {
		t.Fatal("age criterion ignored", plan, err)
	}
	result, err := s.Prune(Retention{Keep: retain(1)}, false)
	if err != nil || len(result.Deleted) != 2 {
		t.Fatal(result, err)
	}
	after, err = s.Backups()
	if err != nil || len(after) != 1 || after[0].ID != before[0].ID {
		t.Fatal("newest backup was not preserved", after, err)
	}
	for i := range 3 {
		value, err := s.Read(fmt.Sprintf("entry%d", i))
		if err != nil || !bytes.Equal(value, []byte{byte(i), 0, 255}) {
			t.Fatal("prune changed live entry", err)
		}
	}
	if _, err := securefs.Read(s.Root, result.Receipt, maxMetadata); err != nil {
		t.Fatal("missing receipt", err)
	}
	if _, err := s.Prune(Retention{}, false); err == nil {
		t.Fatal("implicit retention accepted")
	}
}

func TestPruneCrashHelper(t *testing.T) {
	dir := os.Getenv("FULLA_PRUNE_FIXTURE")
	if dir == "" {
		return
	}
	s, err := Open(dir, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.prune(Retention{Keep: retain(1)}, false, func(phase string) error {
		if phase == os.Getenv("FULLA_PRUNE_PHASE") {
			fmt.Println("prune-ready")
			time.Sleep(time.Minute)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPruneSurvivesKilledProcessAndPartialSnapshotRemoval(t *testing.T) {
	for _, phase := range []string{"prepared", "removed", "receipted"} {
		t.Run(phase, func(t *testing.T) {
			s := fixture(t, false)
			for i := range 3 {
				if _, err := s.Write(fmt.Sprintf("entry%d", i), []byte{byte(i)}, false); err != nil {
					t.Fatal(err)
				}
			}
			backups, err := s.Backups()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestPruneCrashHelper$")
			cmd.Env = append(os.Environ(), "FULLA_PRUNE_FIXTURE="+s.Dir, "FULLA_PRUNE_PHASE="+phase)
			output, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			ready := make(chan bool, 1)
			go func() {
				scanner := bufio.NewScanner(output)
				ready <- scanner.Scan() && scanner.Text() == "prune-ready"
			}()
			ok := false
			select {
			case ok = <-ready:
			case <-time.After(10 * time.Second):
			}
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			if !ok {
				t.Fatal("prune did not reach crash boundary")
			}
			if err := s.Unlocked(); err == nil {
				t.Fatal("crashed prune was not locked")
			}
			data, err := securefs.Read(s.Root, metadata+"/prune.json", maxMetadata)
			if err != nil {
				t.Fatal(err)
			}
			var journal pruneJournal
			if err := json.Unmarshal(data, &journal); err != nil {
				t.Fatal(err)
			}
			if phase == "prepared" {
				// Reproduce interruption during recursive unlink, after removal of the
				// backup's own journal. Recovery must use the independent prune journal.
				if err := s.Root.Remove(metadata + "/backups/" + journal.Selected[0].ID + "/journal.json"); err != nil {
					t.Fatal(err)
				}
			}
			lock, err := s.InspectLock()
			if err != nil || lock == nil || lock.Alive {
				t.Fatal(lock, err)
			}
			if _, err := s.Recover(lock.Token); err != nil {
				t.Fatal(err)
			}
			remaining, err := s.Backups()
			if err != nil || len(remaining) != 1 || remaining[0].ID != backups[0].ID {
				t.Fatal("wrong recovered selection", remaining, err)
			}
			if err := s.Unlocked(); err != nil {
				t.Fatal(err)
			}
			receipt, err := securefs.Read(s.Root, metadata+"/receipts/"+journal.ID+".json", maxMetadata)
			if err != nil || !bytes.Contains(receipt, []byte("backup prune")) {
				t.Fatal("missing prune receipt", err)
			}
		})
	}
}

func TestBackupOrderingUsesInstantsNotTimestampText(t *testing.T) {
	s := fixture(t, false)
	ids := []string{}
	for i := range 3 {
		r, err := s.Write(fmt.Sprintf("entry%d", i), []byte{byte(i)}, false)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, r.Transaction)
	}
	for i, stamp := range []string{"2026-09-05T12:00:00Z", "2026-09-05T12:00:00.1Z", "2026-09-05T12:00:00.01Z"} {
		j, err := s.BackupShow(ids[i])
		if err != nil {
			t.Fatal(err)
		}
		j.Started = stamp
		data, err := json.Marshal(j)
		if err != nil {
			t.Fatal(err)
		}
		if err := securefs.Replace(s.Root, metadata+"/backups/"+ids[i]+"/journal.json", data); err != nil {
			t.Fatal(err)
		}
	}
	result, err := s.Prune(Retention{Keep: retain(1)}, false)
	if err != nil || len(result.Deleted) != 2 {
		t.Fatal(result, err)
	}
	remaining, err := s.Backups()
	if err != nil || len(remaining) != 1 || remaining[0].ID != ids[1] {
		t.Fatal("incorrect chronological retention", remaining, err)
	}
}

func retain(count int) *int { return &count }

func TestPruneRejectsMalformedJournalBeforeRemovingBackups(t *testing.T) {
	s := fixture(t, false)
	if _, err := s.Write("entry", []byte("fixture"), false); err != nil {
		t.Fatal(err)
	}
	backups, err := s.Backups()
	if err != nil {
		t.Fatal(err)
	}
	base := pruneJournal{Version: 1, ID: securefs.ID(), Started: time.Now().UTC().Format(time.RFC3339Nano), Selected: backups}
	for _, kind := range []string{"duplicate", "traversal", "version", "false-progress"} {
		j := base
		j.Selected = append([]Backup{}, base.Selected...)
		switch kind {
		case "duplicate":
			j.Selected = append(j.Selected, j.Selected[0])
		case "traversal":
			j.Selected = append(j.Selected, Backup{ID: "../passwords"})
		case "version":
			j.Version = 99
		case "false-progress":
			j.Done = 1
		}
		if err := s.finishPrune(&j, nil); err == nil {
			t.Fatal("accepted malformed journal", kind)
		}
		remaining, err := s.Backups()
		if err != nil || len(remaining) != 1 {
			t.Fatal("malformed journal deleted data", kind, err)
		}
	}
}
