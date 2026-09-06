package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/agensfield/fulla/internal/store"
)

func TestRunTargetStopsAndContinuesWithoutSupervisor(t *testing.T) {
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
	for name, value := range map[string]string{"api": "synthetic-test-value\n", "mode": "resume"} {
		if _, err := s.Write(name, []byte(value), false); err != nil {
			t.Fatal(err)
		}
	}
	before := machineFiles(t, dir)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRunHelper$", "--", "--store", dir,
		"run", "--env", "TOKEN=api", "--env", "FULLA_TEST_RUN_TARGET=mode",
		"--clean-env", "--inherit", "FULLA_TEST_RUN_HELPER", "--", os.Args[0], "-test.run=^TestRunHelper$")
	command.Env = append(os.Environ(), "HOME="+home, "FULLA_TEST_RUN_HELPER=1", "FULLA_TEST_RUN_TARGET=", "UNSELECTED=omit-me")
	var diagnostic bytes.Buffer
	command.Stderr = &diagnostic
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	reader := bufio.NewReader(stdout)
	line, err := reader.ReadString('\n')
	if err != nil || strings.TrimSpace(line) != strconv.Itoa(command.Process.Pid) {
		t.Fatal("target did not replace Fulla", err)
	}
	if err := command.Process.Signal(syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	// Observe the kernel's stopped state, not an acknowledgement from a wrapper.
	// WUNTRACED consumes only this stop notification; Cmd.Wait reaps final exit.
	for {
		var state syscall.WaitStatus
		pid, err := syscall.Wait4(command.Process.Pid, &state, syscall.WUNTRACED|syscall.WNOHANG, nil)
		if err != nil && err != syscall.EINTR {
			t.Fatal(err)
		}
		if pid == command.Process.Pid {
			// Darwin and Linux encode an ordinary stop as signal<<8 | 0x7f.
			// Go 1.26's BSD Stopped helper incorrectly classifies SIGSTOP as
			// continued, so compare this exact requested kernel notification.
			if state != syscall.WaitStatus(uint32(syscall.SIGSTOP)<<8|0x7f) {
				t.Fatal("wrong native stop", state)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("target stop was not observed")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := command.Process.Signal(syscall.SIGCONT); err != nil {
		t.Fatal(err)
	}
	line, err = reader.ReadString('\n')
	if err != nil || strings.TrimSpace(line) != fmt.Sprint("continued ", command.Process.Pid) {
		t.Fatal("same target did not continue", err)
	}
	err = command.Wait()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 23 {
		t.Fatal("wrong resumed target exit", err)
	}
	if diagnostic.Len() != 0 || !reflect.DeepEqual(before, machineFiles(t, dir)) {
		t.Fatal("stop/continue emitted diagnostics or changed store")
	}
}
