package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/agensfield/fulla/internal/fault"
)

func TestGitOutputBufferEnforcesCopyLimit(t *testing.T) {
	for _, size := range []int{31, 32, 33, 4096} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			canceled := false
			output := &gitOutputBuffer{limit: 32, cancel: func() { canceled = true }}
			_, err := io.Copy(output, io.LimitReader(bytes.NewReader(bytes.Repeat([]byte{'x'}, size)), int64(size)))
			if size <= 32 {
				if err != nil || canceled || len(output.Bytes()) != size {
					t.Fatal("bounded output rejected", err)
				}
			} else if !errors.Is(err, io.ErrShortBuffer) || !canceled || !output.exceeded || len(output.Bytes()) > 32 {
				t.Fatal("copy bypassed output bound", err)
			}
		})
	}
}

func TestGitOutputProcessHelper(t *testing.T) {
	marker := os.Getenv("FULLA_GIT_OUTPUT_MARKER")
	if marker == "" {
		return
	}
	if err := os.WriteFile(marker, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		t.Fatal(err)
	}
	signal.Ignore(syscall.SIGPIPE)
	block := bytes.Repeat([]byte{'x'}, 64<<10)
	for range maxMetadata/(64<<10) + 1 {
		// Ignore write failures to force the parent to terminate us, rather than
		// relying on a cooperative exit after its output pipe is closed.
		_, _ = os.Stdout.Write(block)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestInternalGitOutputLimitTerminatesAndReapsChild(t *testing.T) {
	if os.Getenv("FULLA_GIT_OUTPUT_PARENT") == "1" {
		s, err := Open(os.Getenv("FULLA_GIT_OUTPUT_STORE"), true, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		before := archiveTree(t, s, false)
		output, err := s.Git("status")
		var failure *fault.Error
		if !errors.As(err, &failure) || failure.Code != "git.output_limit" || output != nil {
			t.Fatal("missing typed output refusal", err)
		}
		data, err := os.ReadFile(os.Getenv("FULLA_GIT_OUTPUT_MARKER"))
		if err != nil {
			t.Fatal(err)
		}
		pid, err := strconv.Atoi(string(data))
		if err != nil {
			t.Fatal(err)
		}
		if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
			t.Fatal("output producer not reaped", err)
		}
		if !reflect.DeepEqual(before, archiveTree(t, s, false)) {
			t.Fatal("output refusal changed store")
		}
		return
	}
	s := fixture(t, false)
	bin := t.TempDir()
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
	wrapper := "#!/bin/sh\nexec " + quote(os.Args[0]) + " -test.run=^TestGitOutputProcessHelper$\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(wrapper), 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(bin, "pid")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInternalGitOutputLimitTerminatesAndReapsChild$")
	cmd.Env = append(os.Environ(), "FULLA_GIT_OUTPUT_PARENT=1", "FULLA_GIT_OUTPUT_STORE="+s.Dir, "FULLA_GIT_OUTPUT_MARKER="+marker, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	defer func() {
		// Fixture descendants are confined to their own group so a failed negative
		// control can be cleaned without touching any unrelated process.
		if cmd.Process != nil && (cmd.ProcessState == nil || !cmd.ProcessState.Success()) {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bounded Git subprocess failed: %v %s", err, output)
	}
}

func TestNativeGitOutputBoundary(t *testing.T) {
	s := fixture(t, true)
	want, err := s.Head()
	if err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{len(want), len(want) + 1} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			output, err := s.gitOutput(limit, "rev-parse", "--verify", "HEAD")
			if limit == len(want) {
				var failure *fault.Error
				if !errors.As(err, &failure) || failure.Code != "git.output_limit" || output != nil {
					t.Fatal("accepted oversized native output", err)
				}
			} else if err != nil || string(output) != want+"\n" {
				t.Fatal("exact-bound native output changed", err)
			}
		})
	}
}

func TestHistoryRestoreUsesCiphertextOutputLimit(t *testing.T) {
	s := fixture(t, true)
	value := bytes.Repeat([]byte{0, 255, 10}, maxMetadata/3+1024)
	if _, err := s.Write("large", value, false); err != nil {
		t.Fatal(err)
	}
	ref, err := s.Head()
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := s.Ciphertext("large")
	if err != nil || len(ciphertext) <= maxMetadata {
		t.Fatal("fixture does not exceed metadata limit", err)
	}
	if _, err := s.Remove("large", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.HistoryRestore(ref, "large"); err != nil {
		t.Fatal("supported historical value rejected", err)
	}
	got, err := s.Read("large")
	if err != nil || !bytes.Equal(got, value) {
		t.Fatal("historical bytes changed", err)
	}
}
