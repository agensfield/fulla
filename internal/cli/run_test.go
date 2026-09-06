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

func TestRunHelper(t *testing.T) {
	if os.Getenv("FULLA_TEST_RUN_HELPER") != "1" {
		return
	}
	if target := os.Getenv("FULLA_TEST_RUN_TARGET"); target == "1" || target == "signal" {
		if os.Getenv("TOKEN") != "synthetic-test-value\n" {
			os.Exit(70)
		}
		if os.Getenv("UNSELECTED") != "" {
			os.Exit(71)
		}
		fmt.Println(os.Getpid())
		if target == "signal" {
			for {
				time.Sleep(time.Hour)
			}
		}
		os.Exit(23)
	}
	i := 0
	for i < len(os.Args) && os.Args[i] != "--" {
		i++
	}
	os.Exit((&App{}).Main(os.Args[i+1:]))
}

func TestRunTargetTerminatesByNativeSignal(t *testing.T) {
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
	for name, value := range map[string]string{"api": "synthetic-test-value\n", "mode": "signal"} {
		if _, err := s.Write(name, []byte(value), false); err != nil {
			t.Fatal(err)
		}
	}
	before := machineFiles(t, dir)
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
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
			// The target reports readiness only after exec and environment checks.
			// The context kills the owned child if it never reaches this point.
			line, err := bufio.NewReader(stdout).ReadString('\n')
			if err != nil || strings.TrimSpace(line) != strconv.Itoa(command.Process.Pid) {
				t.Fatal("target did not replace Fulla and become ready", err)
			}
			if err := command.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			err = command.Wait()
			exit, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("expected signal termination: %v", err)
			}
			state, ok := exit.Sys().(syscall.WaitStatus)
			if !ok || !state.Signaled() || state.Signal() != sig {
				t.Fatalf("wrong native termination: %v", exit)
			}
			if diagnostic.Len() != 0 || !reflect.DeepEqual(before, machineFiles(t, dir)) {
				t.Fatal("signal termination emitted diagnostics or changed the store")
			}
		})
	}
}

func TestRunReplacesProcessAndPreservesExit(t *testing.T) {
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
	if _, err := s.Write("api", []byte("synthetic-test-value\n"), false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write("marker", []byte("1"), false); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestRunHelper$", "--", "--store", dir,
		"run", "--env", "TOKEN=api", "--env", "FULLA_TEST_RUN_TARGET=marker",
		"--clean-env", "--inherit", "FULLA_TEST_RUN_HELPER", "--", os.Args[0], "-test.run=^TestRunHelper$")
	command.Env = append(os.Environ(), "HOME="+home, "FULLA_TEST_RUN_HELPER=1", "FULLA_TEST_RUN_TARGET=", "UNSELECTED=omit-me")
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	pid := command.Process.Pid
	err = command.Wait()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 23 {
		t.Fatalf("wrong exit: %v, %s", err, output.String())
	}
	if strings.TrimSpace(output.String()) != strconv.Itoa(pid) {
		t.Fatalf("PID changed or output leaked: %q", output.String())
	}
}
