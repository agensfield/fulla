package store

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agensfield/fulla/internal/securefs"
)

// Reconstructed private fixtures cover bindings that authenticated publication
// must never produce, and opaque journal bytes extracted before authentication.
func TestRestoreRecoveryBindingRefusals(t *testing.T) {
	for _, mode := range []string{"unpublished-journal", "published-journal", "missing-store-id", "wrong-store-id"} {
		t.Run(mode, func(t *testing.T) {
			s := fixture(t, false)
			id, token := securefs.ID(), securefs.ID()
			target := s.Dir
			if mode == "unpublished-journal" {
				stage := filepath.Join(filepath.Dir(s.Dir), ".fulla-restore-"+id)
				if err := os.Rename(s.Dir, stage); err != nil {
					t.Fatal(err)
				}
				s.Root.Close()
				root, err := os.OpenRoot(stage)
				if err != nil {
					t.Fatal(err)
				}
				s.Root = root
				s.Dir = stage
			}
			child := exec.Command("true")
			if err := child.Run(); err != nil {
				t.Fatal(err)
			}
			host, err := os.Hostname()
			if err != nil {
				t.Fatal(err)
			}
			info := fmt.Sprintf("pid=%d host=%s operation=restore restore_id=%s restore_target=%s", child.ProcessState.Pid(), host, id, base64.RawURLEncoding.EncodeToString([]byte(target)))
			if mode != "missing-store-id" {
				storeID := s.Meta.StoreID
				if mode == "wrong-store-id" {
					storeID = securefs.ID()
				}
				info += " restore_store_id=" + storeID
			}
			if err := s.Root.Mkdir("lock", 0700); err != nil {
				t.Fatal(err)
			}
			for name, data := range map[string]string{"lock/info": info + "\n", "lock/owner": token + "\n"} {
				if err := securefs.WriteNew(s.Root, name, []byte(data)); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "unpublished-journal" || mode == "published-journal" {
				if err := securefs.WriteNew(s.Root, metadata+"/pending.json", []byte("not an authenticated journal")); err != nil {
					t.Fatal(err)
				}
			}
			before := archiveTree(t, s, false)
			result, err := RecoverCreation(s.Dir, token)
			if mode == "unpublished-journal" {
				if err != nil || result["restore_applied"] != false {
					t.Fatal("opaque stage cleanup failed", result, err)
				}
				if _, err := os.Lstat(s.Dir); !os.IsNotExist(err) {
					t.Fatal("stage retained", err)
				}
			} else {
				if err == nil {
					t.Fatal("invalid publication accepted")
				}
				// A conflicting live journal is detected under the recovery guard. The
				// guard may be created, but ownership and every store byte stay unchanged.
				after := archiveTree(t, s, false)
				if mode == "published-journal" {
					delete(after, "lock/recovery")
				}
				if !reflect.DeepEqual(before, after) {
					t.Fatal("refusal changed store or ownership")
				}
			}
		})
	}
}
