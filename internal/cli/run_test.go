package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/agensfield/fulla/internal/store"
)

func TestRunHelper(t *testing.T) {
	if os.Getenv("FULLA_TEST_RUN_HELPER") != "1" {
		return
	}
	if os.Getenv("FULLA_TEST_RUN_TARGET") == "1" {
		if os.Getenv("TOKEN") != "synthetic-test-value\n" {
			os.Exit(70)
		}
		if os.Getenv("UNSELECTED") != "" {
			os.Exit(71)
		}
		fmt.Println(os.Getpid())
		os.Exit(23)
	}
	i := 0
	for i < len(os.Args) && os.Args[i] != "--" {
		i++
	}
	os.Exit((&App{}).Main(os.Args[i+1:]))
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
