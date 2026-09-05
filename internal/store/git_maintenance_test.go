package store

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestInternalGitDoesNotLaunchAutomaticMaintenance(t *testing.T) {
	s := fixture(t, true)
	directory := t.TempDir()
	control := filepath.Join(directory, "control.json")
	t.Setenv("GIT_TRACE2_EVENT", control)
	// Positive control proves this Git exposes the automatic child through trace2.
	// Keep this disposable control in the foreground so its child has finished
	// before the actual Fulla path is inspected.
	command := exec.Command("git", "-C", s.PasswordDirectory(), "-c", "user.name=Fixture", "-c", "user.email=fixture@localhost", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "-c", "maintenance.auto=true", "-c", "maintenance.autoDetach=false", "-c", "gc.autoDetach=false", "commit", "--allow-empty", "-m", "Fixture control")
	if err := command.Run(); err != nil {
		t.Fatal("control Git commit failed", err)
	}
	if !traceHasMaintenance(t, control) {
		t.Fatal("positive control did not observe automatic maintenance")
	}
	traced := filepath.Join(directory, "fulla.json")
	t.Setenv("GIT_TRACE2_EVENT", traced)
	// Include initialization and transactional writes: maintenance must not
	// outlive either command and race filesystem inspection or snapshots.
	actual := fixture(t, true)
	if _, err := actual.Write("entry", []byte("fixture"), false); err != nil {
		t.Fatal(err)
	}
	if traceHasMaintenance(t, traced) {
		t.Fatal("internal Git launched automatic maintenance outside Fulla transaction ownership")
	}
}

func traceHasMaintenance(t *testing.T, name string) bool {
	t.Helper()
	file, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event struct {
			Event string   `json:"event"`
			Argv  []string `json:"argv"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatal("invalid fixture trace", err)
		}
		if event.Event != "child_start" {
			continue
		}
		for _, arg := range event.Argv {
			if arg == "maintenance" || arg == "gc" {
				return true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return false
}
