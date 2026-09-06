package store

import (
	"bufio"
	"bytes"
	"context"
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

func TestAdoptionCrashHelper(t *testing.T) {
	dir := os.Getenv("FULLA_ADOPTION_CRASH_DIR")
	if dir == "" {
		return
	}
	_, err := adopt(dir, false, nil, func(phase string) error {
		if phase == os.Getenv("FULLA_ADOPTION_CRASH_PHASE") {
			fmt.Println("adoption-ready")
			time.Sleep(time.Minute)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestKilledAdoptionRecovery(t *testing.T) {
	for _, git := range []bool{false, true} {
		for _, phase := range []string{"bound", "staged", "published"} {
			t.Run(fmt.Sprintf("git=%v/%s", git, phase), func(t *testing.T) {
				s := fixture(t, git)
				value := []byte{0, 255, 10, 65}
				if _, err := s.Write("sample", value, false); err != nil {
					t.Fatal(err)
				}
				if err := s.Root.RemoveAll(metadata); err != nil {
					t.Fatal(err)
				}
				before := archiveTree(t, s, false)
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAdoptionCrashHelper$")
				cmd.Env = append(os.Environ(), "FULLA_ADOPTION_CRASH_DIR="+s.Dir, "FULLA_ADOPTION_CRASH_PHASE="+phase)
				output, err := cmd.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if cmd.ProcessState == nil {
						_ = cmd.Process.Kill()
						_ = cmd.Wait()
					}
				}()
				line, err := bufio.NewReader(output).ReadString('\n')
				if err != nil || line != "adoption-ready\n" {
					t.Fatal("missing crash boundary", err)
				}
				owner, err := s.InspectLock()
				if err != nil || owner == nil || owner.AdoptionID == "" {
					t.Fatal("missing adoption binding", err)
				}
				if _, err := s.Recover(owner.Token); err == nil {
					t.Fatal("stole live adoption")
				}
				if err := cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				_ = cmd.Wait()
				reopened, err := Open(s.Dir, false, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				if _, err := reopened.Recover("wrong-token"); err == nil {
					t.Fatal("accepted wrong token")
				}
				stage := ".fulla-adopt-" + owner.AdoptionID
				if phase == "published" {
					if err := s.Root.Mkdir(stage, 0700); err != nil {
						t.Fatal(err)
					}
					if err := securefs.WriteNew(s.Root, stage+"/new-occupant", []byte("preserved")); err != nil {
						t.Fatal(err)
					}
				}
				result, err := reopened.Recover(owner.Token)
				if err != nil || result["adoption_applied"] != (phase == "published") || result["lock_released"] != true {
					t.Fatal("recovery failed", result, err)
				}
				if phase == "published" {
					got, err := securefs.Read(s.Root, stage+"/new-occupant", 100)
					if err != nil || string(got) != "preserved" {
						t.Fatal("deleted reused stage", err)
					}
					if err := s.Root.RemoveAll(stage); err != nil {
						t.Fatal(err)
					}
				}
				after := archiveTree(t, s, false)
				if phase == "published" {
					for name := range after {
						if name == metadata || strings.HasPrefix(name, metadata+"/") {
							delete(after, name)
						}
					}
				}
				if !reflect.DeepEqual(before, after) {
					t.Fatal("recovery changed live pa material or left staging")
				}
				if _, err := reopened.Recover(owner.Token); err == nil {
					t.Fatal("repeated recovery accepted missing lock")
				}
				if phase != "published" {
					if _, err := Adopt(s.Dir, false, nil); err != nil {
						t.Fatal("explicit adoption retry failed", err)
					}
				}
				current, err := Open(s.Dir, true, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer current.Close()
				got, err := current.Read("sample")
				if err != nil || !bytes.Equal(got, value) {
					t.Fatal("lost exact bytes", err)
				}
				if err := current.DeepVerify(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestAdoptionRecoveryRejectsConflictingBindings(t *testing.T) {
	for _, variant := range []string{"duplicate", "traversal", "transaction", "peer", "different-store", "future-domain"} {
		t.Run(variant, func(t *testing.T) {
			s := fixture(t, false)
			lock, err := s.Lock("adopt")
			if err != nil {
				t.Fatal(err)
			}
			id := s.Meta.StoreID
			info, err := securefs.Read(s.Root, "lock/info", 4096)
			if err != nil {
				t.Fatal(err)
			}
			text := strings.TrimSpace(string(info)) + " adoption_id=" + id
			switch variant {
			case "duplicate":
				text += " adoption_id=" + id
			case "traversal":
				text = strings.ReplaceAll(text, "adoption_id="+id, "adoption_id=../outside")
			case "transaction":
				text += " stage_id=" + securefs.ID()
			case "peer":
				text += " peer_receipt=" + securefs.ID()
			case "different-store":
				id = securefs.ID()
			case "future-domain":
				meta := s.Meta
				meta.Domains["backup"] = 2
				data, err := json.Marshal(meta)
				if err != nil {
					t.Fatal(err)
				}
				if err := securefs.Replace(s.Root, ".fulla/store.json", data); err != nil {
					t.Fatal(err)
				}
			}
			if err := securefs.Replace(s.Root, "lock/info", []byte(text+"\n")); err != nil {
				t.Fatal(err)
			}
			before := transactionFiles(t, s)
			if variant == "different-store" || variant == "future-domain" {
				if _, err := s.adoptionPublished(id); err == nil {
					t.Fatal("accepted conflicting published adoption")
				}
			} else if _, err := s.InspectLock(); err == nil {
				t.Fatal("accepted invalid adoption binding")
			}
			if !reflect.DeepEqual(before, transactionFiles(t, s)) {
				t.Fatal("validation changed evidence")
			}
			_ = lock
		})
	}
}
