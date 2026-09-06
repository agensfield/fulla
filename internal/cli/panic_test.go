package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

const panicSentinel = "synthetic-private-panic-value-2ebfe4"

func TestPanicProcessHelper(t *testing.T) {
	if os.Getenv("FULLA_PANIC_TEST_HELPER") != "1" {
		return
	}
	// Even the deliberate debug crash must not create an OS core file.
	if err := syscall.Setrlimit(syscall.RLIMIT_CORE, &syscall.Rlimit{}); err != nil {
		t.Fatal(err)
	}
	args := []string{"list"}
	if os.Getenv("FULLA_PANIC_TEST_JSON") == "1" {
		args = append(args, "--json")
	}
	app := App{Getenv: func(string) string {
		if os.Getenv("FULLA_PANIC_TEST_ERROR") == "1" {
			panic(errors.New(panicSentinel))
		}
		panic(panicSentinel)
	}}
	os.Exit(app.Main(args))
}

func testPanicProcessPolicy(t *testing.T, expectDebug bool) {
	for _, machine := range []bool{false, true} {
		for _, errorValue := range []bool{false, true} {
			name := "human-string"
			if machine {
				name = "json-string"
			}
			if errorValue {
				name += "-error"
			}
			t.Run(name, func(t *testing.T) {
				dir := t.TempDir()
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPanicProcessHelper$")
				cmd.Dir = dir
				cmd.Env = []string{"HOME=" + dir, "TMPDIR=" + dir, "GOTRACEBACK=single", "FULLA_PANIC_TEST_HELPER=1"}
				if machine {
					cmd.Env = append(cmd.Env, "FULLA_PANIC_TEST_JSON=1")
				}
				if errorValue {
					cmd.Env = append(cmd.Env, "FULLA_PANIC_TEST_ERROR=1")
				}
				var stdout, stderr bytes.Buffer
				cmd.Stdout, cmd.Stderr = &stdout, &stderr
				err := cmd.Run()
				var exit *exec.ExitError
				if ctx.Err() != nil || !errors.As(err, &exit) {
					t.Fatalf("expected bounded process failure: %v", err)
				}
				if expectDebug {
					// testing prints its own failure banner to stdout when the
					// deliberate panic escapes App.Main. Go prints the stack to stderr.
					if exit.ExitCode() != 2 || !bytes.Contains(stderr.Bytes(), []byte(panicSentinel)) || !bytes.Contains(stderr.Bytes(), []byte("cli.go:")) {
						t.Fatal("debug build did not retain panic value and original CLI stack")
					}
				} else {
					if exit.ExitCode() != 1 {
						t.Fatalf("redacted failure exit = %d", exit.ExitCode())
					}
					if machine {
						var result envelope
						if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
							t.Fatal("expected exactly one JSON envelope", err)
						}
						if result.Schema != "fulla.cli/v1" || result.OK || result.Command != "list" || result.Data != nil || result.Error == nil || result.Error.Code != "internal.failure" || result.Error.Message != "unexpected internal failure" || len(result.Error.Details) != 0 || stderr.Len() != 0 {
							t.Fatal("unexpected machine panic output")
						}
					} else if stdout.Len() != 0 || stderr.String() != "internal.failure: unexpected internal failure\n" {
						t.Fatal("unexpected human panic output")
					}
					for _, forbidden := range []string{panicSentinel, "goroutine ", "cli.go:", dir} {
						if bytes.Contains(stdout.Bytes(), []byte(forbidden)) || bytes.Contains(stderr.Bytes(), []byte(forbidden)) {
							t.Fatal("private value, stack, or path escaped redaction")
						}
					}
				}
				entries, err := filepath.Glob(filepath.Join(dir, "*"))
				if err != nil || len(entries) != 0 {
					t.Fatalf("panic created filesystem artifacts: %v, %v", entries, err)
				}
			})
		}
	}
}
