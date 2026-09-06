package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/agensfield/fulla/internal/store"
)

// Reconstructed detached ownership verifies CLI routing; the store's separate
// subprocess matrix kills actual release writers at publication/removal seams.
func TestDoctorDetachedCleanupAndExportInputBoundary(t *testing.T) {
	for _, noGit := range []bool{false, true} {
		t.Run(fmt.Sprint("no-git=", noGit), func(t *testing.T) {
			home, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(home, "store")
			if _, err := store.Init(dir, noGit, false); err != nil {
				t.Fatal(err)
			}
			s, err := store.Open(dir, true, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			lock, err := s.Lock("cleanup-fixture")
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
			name := fmt.Sprintf(".fulla-lock-release-v1-%x-%d-%s", sha256.Sum256([]byte(host)), child.ProcessState.Pid(), lock.Token)
			oldPath := filepath.Join(dir, name)
			if err := os.Rename(filepath.Join(dir, "lock"), oldPath); err != nil {
				t.Fatal(err)
			}
			var out, diagnostic bytes.Buffer
			invoke := func(args ...string) int {
				out.Reset()
				diagnostic.Reset()
				app := App{In: &unselectedInput{}, Out: &out, Err: &diagnostic, Getenv: func(key string) string {
					if key == "HOME" {
						return home
					}
					return ""
				}}
				return app.Main(append([]string{"--store", dir, "--json"}, args...))
			}
			before := machineFiles(t, dir)
			if code := invoke("doctor"); code != 1 || !bytes.Contains(out.Bytes(), []byte(lock.Token)) {
				t.Fatal("doctor omitted cleanup ownership", code, out.String())
			}
			if !reflect.DeepEqual(before, machineFiles(t, dir)) {
				t.Fatal("inspection changed store")
			}
			reportPath := filepath.Join(home, "report.json")
			if code := invoke("doctor", "--report", reportPath); code != 1 {
				t.Fatal("wrong diagnostic report status", code)
			}
			report, err := os.ReadFile(reportPath)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(report, []byte(lock.Token)) || bytes.Contains(report, []byte(oldPath)) {
				t.Fatal("shareable report leaked cleanup ownership")
			}
			passphrase := filepath.Join(home, "passphrase")
			if err := os.WriteFile(passphrase, []byte("synthetic recovery value"), 0600); err != nil {
				t.Fatal(err)
			}
			input, err := os.Open(passphrase)
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			code := invoke("backup", "export", "--full", "--output", filepath.Join(home, "archive.age"), "--passphrase-fd", strconv.Itoa(int(input.Fd())))
			position, seekErr := input.Seek(0, 1)
			if seekErr != nil || position != 0 {
				t.Fatal("cleanup refusal consumed recovery input", seekErr, position)
			}
			var envelope struct{ Error struct{ Code string } }
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil || code != 1 || envelope.Error.Code != "store.lock_cleanup_pending" {
				t.Fatal("wrong export refusal", err, out.String())
			}
			if !reflect.DeepEqual(before, machineFiles(t, dir)) {
				t.Fatal("refused export changed store")
			}
			newer, err := s.Lock("newer-writer")
			if err != nil {
				t.Fatal(err)
			}
			before = machineFiles(t, dir)
			if code := invoke("doctor", "--recover-lock", lock.Token); code != 0 {
				t.Fatal("public cleanup failed", code, out.String())
			}
			for name := range before {
				if name == oldPath || strings.HasPrefix(name, oldPath+string(os.PathSeparator)) {
					delete(before, name)
				}
			}
			if !reflect.DeepEqual(before, machineFiles(t, dir)) {
				t.Fatal("cleanup changed newer writer or live store")
			}
			if err := newer.Release(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
