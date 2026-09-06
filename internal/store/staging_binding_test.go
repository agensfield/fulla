package store

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestStagingBindingRejectsConflictingJournalAndMalformedOwner(t *testing.T) {
	for _, kind := range []string{"journal", "duplicate", "invalid", "peer", "owner"} {
		t.Run(kind, func(t *testing.T) {
			s := fixture(t, false)
			lock, err := s.Lock("fixture")
			if err != nil {
				t.Fatal(err)
			}
			id := securefs.ID()
			if err := s.Root.Mkdir(metadata+"/transactions/"+id, 0700); err != nil {
				t.Fatal(err)
			}
			if kind == "owner" {
				if err := securefs.Replace(s.Root, "lock/owner", []byte("another-owner\n")); err != nil {
					t.Fatal(err)
				}
				before := transactionFiles(t, s)
				if err := s.bindStaging(lock, id); err == nil {
					t.Fatal("bound another owner's lock")
				}
				if !reflect.DeepEqual(before, transactionFiles(t, s)) {
					t.Fatal("changed another owner's record")
				}
				return
			}
			if err := s.bindStaging(lock, id); err != nil {
				t.Fatal(err)
			}
			owner, err := s.InspectLock()
			if err != nil || owner.StageID != id || lock.StageID != id {
				t.Fatal("missing durable binding", err)
			}
			if kind == "journal" {
				data, err := json.Marshal(Journal{Version: 1, ID: securefs.ID(), Phase: "prepared"})
				if err != nil {
					t.Fatal(err)
				}
				if err := securefs.WriteNew(s.Root, metadata+"/pending.json", data); err != nil {
					t.Fatal(err)
				}
				before := transactionFiles(t, s)
				var problem *fault.Error
				if err := s.validateStagingBinding(id); !errors.As(err, &problem) || problem.Code != "transaction.conflict" {
					t.Fatal("ignored journal mismatch", err)
				}
				if !reflect.DeepEqual(before, transactionFiles(t, s)) {
					t.Fatal("binding validation mutated evidence")
				}
				return
			}
			data, err := securefs.Read(s.Root, "lock/info", 4096)
			if err != nil {
				t.Fatal(err)
			}
			text := strings.TrimSpace(string(data))
			switch kind {
			case "duplicate":
				text += " stage_id=" + id
			case "invalid":
				text = strings.ReplaceAll(text, "stage_id="+id, "stage_id=../outside")
			case "peer":
				text += " peer_receipt=" + securefs.ID()
			}
			if err := securefs.Replace(s.Root, "lock/info", []byte(text+"\n")); err != nil {
				t.Fatal(err)
			}
			before := transactionFiles(t, s)
			if _, err := s.InspectLock(); err == nil {
				t.Fatal("accepted malformed/conflicting binding")
			}
			if !reflect.DeepEqual(before, transactionFiles(t, s)) {
				t.Fatal("inspection changed invalid binding")
			}
		})
	}
}

func TestUnpublishedCleanupPreservesStageWhenOwnershipChanges(t *testing.T) {
	for _, kind := range []string{"owner", "binding", "missing-owner", "invalid-info"} {
		t.Run(kind, func(t *testing.T) {
			s := fixture(t, false)
			lock, err := s.Lock("fixture")
			if err != nil {
				t.Fatal(err)
			}
			id := securefs.ID()
			dir := metadata + "/transactions/" + id
			if err := s.Root.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := s.bindStaging(lock, id); err != nil {
				t.Fatal(err)
			}
			if err := securefs.WriteNew(s.Root, dir+"/private-fixture", []byte("retained-private-evidence")); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "owner":
				err = securefs.Replace(s.Root, "lock/owner", []byte("replacement-owner\n"))
			case "missing-owner":
				err = s.Root.Remove("lock/owner")
			case "binding":
				var data []byte
				data, err = securefs.Read(s.Root, "lock/info", 4096)
				if err == nil {
					err = securefs.Replace(s.Root, "lock/info", []byte(strings.ReplaceAll(string(data), id, securefs.ID())))
				}
			case "invalid-info":
				err = securefs.Replace(s.Root, "lock/info", []byte("invalid\n"))
			}
			if err != nil {
				t.Fatal(err)
			}
			before := transactionFiles(t, s)
			err = s.finishUnpublished(lock, dir, errors.New("synthetic-private-original-error"))
			if !reflect.DeepEqual(before, transactionFiles(t, s)) {
				t.Fatal("cleanup deleted evidence after ownership changed")
			}
			var failure *fault.Error
			if !errors.As(err, &failure) || failure.Code != "transaction.cleanup_failed" || failure.Details["ownership_changed"] != true || failure.Details["lock_cleanup_required"] != true || failure.Details["staging_cleanup_required"] != true || failure.Details["lock_retained"] != nil {
				t.Fatal("lost changed ownership evidence", err)
			}
		})
	}
}
