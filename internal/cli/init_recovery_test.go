package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agensfield/fulla/internal/securefs"
	"github.com/agensfield/fulla/internal/store"
)

func TestDoctorInitializationRequiresBoundStage(t *testing.T) {
	for _, binding := range []string{"valid", "absent", "wrong-target", "conflicting"} {
		t.Run(binding, func(t *testing.T) {
			home, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			id, token := securefs.ID(), securefs.ID()
			stage := filepath.Join(home, ".fulla-init-"+id)
			if err := os.MkdirAll(filepath.Join(stage, "lock"), 0700); err != nil {
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
			target := filepath.Join(home, "target")
			info := fmt.Sprintf("pid=%d host=%s operation=init", child.ProcessState.Pid(), host)
			if binding != "absent" {
				boundTarget := target
				if binding == "wrong-target" {
					boundTarget = filepath.Join(home, "elsewhere", "target")
				}
				info += " init_id=" + id + " init_target=" + base64.RawURLEncoding.EncodeToString([]byte(boundTarget))
				if binding == "conflicting" {
					info += " adoption_id=" + securefs.ID()
				}
			}
			for name, data := range map[string]string{"lock/info": info + "\n", "lock/owner": token + "\n", "partial-private-fixture": "synthetic"} {
				if err := os.WriteFile(filepath.Join(stage, name), []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before := machineFiles(t, home)
			var out, diagnostic bytes.Buffer
			app := App{Out: &out, Err: &diagnostic, Getenv: func(key string) string {
				if key == "HOME" {
					return home
				}
				return ""
			}}
			if binding == "valid" {
				inspected := app.Main([]string{"--store", stage, "--json", "doctor"})
				var report struct {
					Error struct {
						Details struct {
							Report struct{ Lock *store.LockInfo }
						}
					}
				}
				if err := json.Unmarshal(out.Bytes(), &report); err != nil {
					t.Fatal(err)
				}
				owner := report.Error.Details.Report.Lock
				if inspected != 1 || owner == nil || owner.Token != token || owner.InitID != id || owner.InitTarget != target {
					t.Fatal("partial-stage inspection lost ownership evidence", out.String())
				}
				if !reflect.DeepEqual(before, machineFiles(t, home)) {
					t.Fatal("inspection changed stage")
				}
				out.Reset()
				diagnostic.Reset()
			}
			exit := app.Main([]string{"--store", stage, "--json", "doctor", "--recover-lock", token})
			var result struct {
				OK   bool
				Data map[string]any
			}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if binding == "valid" {
				if exit != 0 || !result.OK || result.Data["init_applied"] != false {
					t.Fatal("bound stage not recovered", out.String())
				}
				if _, err := os.Stat(stage); !os.IsNotExist(err) {
					t.Fatal("stage retained", err)
				}
				if _, err := os.Stat(target); !os.IsNotExist(err) {
					t.Fatal("implicit publication", err)
				}
			} else {
				if exit == 0 || result.OK {
					t.Fatal("unbound/conflicting stage accepted")
				}
				if !reflect.DeepEqual(before, machineFiles(t, home)) {
					t.Fatal("refusal changed fixture")
				}
			}
		})
	}
}
