package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/agensfield/fulla/internal/securefs"
	"github.com/agensfield/fulla/internal/store"
)

func TestDoctorRecoversBoundUnadoptedCandidate(t *testing.T) {
	for _, bound := range []bool{false, true} {
		t.Run(fmt.Sprintf("bound=%v", bound), func(t *testing.T) {
			home, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(home, "store")
			if _, err := store.Init(dir, true, false); err != nil {
				t.Fatal(err)
			}
			s, err := store.Open(dir, true, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if err := s.Root.RemoveAll(".fulla"); err != nil {
				t.Fatal(err)
			}
			lock, err := s.Lock("adopt")
			if err != nil {
				t.Fatal(err)
			}
			child := exec.Command("true")
			if err := child.Run(); err != nil {
				t.Fatal(err)
			}
			host, err := os.Hostname()
			if err != nil {
				t.Fatal(err)
			}
			info := fmt.Sprintf("pid=%d host=%s operation=adopt", child.ProcessState.Pid(), host)
			stage := ".fulla-adopt-" + securefs.ID()
			if bound {
				info += " adoption_id=" + stage[len(".fulla-adopt-"):]
				if err := s.Root.Mkdir(stage, 0700); err != nil {
					t.Fatal(err)
				}
				if err := securefs.WriteNew(s.Root, stage+"/partial", []byte("metadata fixture")); err != nil {
					t.Fatal(err)
				}
			}
			if err := securefs.Replace(s.Root, "lock/info", []byte(info+"\n")); err != nil {
				t.Fatal(err)
			}
			var out, diagnostic bytes.Buffer
			app := App{Out: &out, Err: &diagnostic, Getenv: func(key string) string {
				if key == "HOME" {
					return home
				}
				return ""
			}}
			status := app.Main([]string{"--store", dir, "--json", "doctor", "--recover-lock", lock.Token})
			var result struct {
				OK    bool
				Data  map[string]any
				Error struct{ Code string }
			}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if bound {
				if status != 0 || !result.OK || result.Data["adoption_applied"] != false || result.Data["lock_released"] != true {
					t.Fatal("bound candidate not recovered", out.String())
				}
				if _, err := s.Root.Lstat(stage); !os.IsNotExist(err) {
					t.Fatal("staging not cleaned", err)
				}
			} else {
				if status != 1 || result.Error.Code != "store.uninitialized" {
					t.Fatal("unbound candidate accepted", out.String())
				}
				owner, err := s.InspectLock()
				if err != nil || owner == nil || owner.Token != lock.Token {
					t.Fatal("changed unbound lock", err)
				}
			}
			if _, err := s.Root.Lstat(".fulla"); !os.IsNotExist(err) {
				t.Fatal("recovery implicitly adopted", err)
			}
		})
	}
}
