package store

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/agensfield/fulla/internal/securefs"
)

func TestBasicRecoveryRejectsUnboundOrUnknownState(t *testing.T) {
	for _, kind := range []string{"future-protocol", "missing-protocol", "wrong-id", "unknown-command", "malformed-journal", "feature-journal", "stage-file"} {
		t.Run(kind, func(t *testing.T) {
			s := fixture(t, false)
			futureBasicFixture(t, s, true)
			lock, err := s.lockMutation("add")
			if err != nil {
				t.Fatal(err)
			}
			if err := s.prepareBasicProtocol(); err != nil {
				t.Fatal(err)
			}
			id := securefs.ID()
			stage := transactionBase(basicProtocol) + "/" + id
			if err := s.Root.Mkdir(stage, 0700); err != nil {
				t.Fatal(err)
			}
			if err := s.bindStagingProtocol(lock, id, basicProtocol); err != nil {
				t.Fatal(err)
			}
			journal := Journal{Version: 1, ID: id, Command: "add", SnapshotDomain: basicProtocol, Phase: "prepared"}
			if kind == "wrong-id" {
				journal.ID = securefs.ID()
			}
			if kind == "unknown-command" {
				journal.Command = "future-operation"
			}
			data, err := json.Marshal(journal)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "malformed-journal" {
				data = []byte("{broken")
			}
			if err := securefs.WriteNew(s.Root, journalPath(basicProtocol), data); err != nil {
				t.Fatal(err)
			}
			info, err := s.Root.ReadFile("lock/info")
			if err != nil {
				t.Fatal(err)
			}
			text := string(info)
			if kind == "future-protocol" {
				text = strings.Replace(text, "stage_protocol="+basicProtocol, "stage_protocol=basic-v2", 1)
			}
			if kind == "missing-protocol" {
				text = strings.Replace(text, " stage_protocol="+basicProtocol, "", 1)
			}
			if kind == "feature-journal" {
				if err := securefs.WriteNew(s.Root, metadata+"/pending.json", []byte("opaque future journal")); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "stage-file" {
				if err := s.Root.Remove(stage); err != nil {
					t.Fatal(err)
				}
				if err := securefs.WriteNew(s.Root, stage, []byte("preserve")); err != nil {
					t.Fatal(err)
				}
			}
			dead := exec.Command("true")
			if err := dead.Run(); err != nil {
				t.Fatal(err)
			}
			// Malformed ownership fixtures only; actual killed owners have separate tests.
			text = strings.Replace(text, fmt.Sprintf("pid=%d ", os.Getpid()), fmt.Sprintf("pid=%d ", dead.ProcessState.Pid()), 1)
			if err := securefs.Replace(s.Root, "lock/info", []byte(text)); err != nil {
				t.Fatal(err)
			}
			before := archiveTree(t, s, false)
			if _, err := s.Recover(lock.Token); err == nil {
				t.Fatal("accepted ambiguous or unsupported basic recovery")
			}
			if !reflect.DeepEqual(before, archiveTree(t, s, false)) {
				t.Fatal("refusal changed ownership or opaque metadata")
			}
		})
	}
}
