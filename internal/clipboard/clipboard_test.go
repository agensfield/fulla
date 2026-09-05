package clipboard

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	syscall.Umask(0077)
	if len(os.Args) == 2 && os.Args[1] == WorkerFlag {
		os.Exit(Worker())
	}
	if len(os.Args) == 4 && os.Args[1] == "clipboard-fixture" {
		if os.Getenv("FULLA_TEST_AMBIENT_SECRET") != "" {
			os.Exit(71)
		}
		file, operation := os.Args[2], os.Args[3]
		if operation == "write" {
			value, err := io.ReadAll(os.Stdin)
			if err != nil || os.WriteFile(file, value, 0600) != nil {
				os.Exit(72)
			}
		} else if operation == "read" {
			// The parent polls .reader concurrently. Publish the complete PID
			// by rename; WriteFile exposes an empty file before its write.
			if os.WriteFile(file+".reader.pending", []byte(strconv.Itoa(os.Getppid())), 0600) != nil ||
				os.Rename(file+".reader.pending", file+".reader") != nil {
				os.Exit(73)
			}
			value, err := os.ReadFile(file)
			if err != nil {
				os.Exit(74)
			}
			if _, err := os.Stdout.Write(value); err != nil {
				os.Exit(75)
			}
		} else {
			os.Exit(23)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func backendFixture(t *testing.T) (Backend, string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "clipboard")
	t.Setenv("FULLA_TEST_AMBIENT_SECRET", "synthetic-ambient-value")
	return Backend{"x11", []string{os.Args[0], "clipboard-fixture", file, "read"}, []string{os.Args[0], "clipboard-fixture", file, "write"}}, file
}

func TestCopyAndConditionalClearExactText(t *testing.T) {
	b, file := backendFixture(t)
	original := []byte("synthetic-秘密\n\n")
	if err := b.ValidateValue(original); err != nil {
		t.Fatal(err)
	}
	if err := b.WriteValue(original); err != nil {
		t.Fatal(err)
	}
	actual, err := b.Digest()
	if err != nil || actual != Digest(original) {
		t.Fatal("lost bytes", err)
	}
	if err := os.WriteFile(file, []byte("newer clipboard"), 0600); err != nil {
		t.Fatal(err)
	}
	if cleared, err := b.ClearIfMatching(Digest(original)); err != nil || cleared {
		t.Fatal("cleared replacement", err)
	}
	current, err := os.ReadFile(file)
	if err != nil || string(current) != "newer clipboard" {
		t.Fatal("replacement changed", err)
	}
	if cleared, err := b.ClearIfMatching(Digest(current)); err != nil || !cleared {
		t.Fatal("did not clear matching value", err)
	}
	if current, err := os.ReadFile(file); err != nil || len(current) != 0 {
		t.Fatal("clipboard retained", err)
	}
}

func TestExpiryWorkerFreshProcessAndDigestOnly(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(strconv.FormatBool(replace), func(t *testing.T) {
			b, file := backendFixture(t)
			original := []byte("synthetic-copy-value")
			if err := os.WriteFile(file, original, 0600); err != nil {
				t.Fatal(err)
			}
			payload, err := json.Marshal(request{b, Digest(original), time.Now().Add(100 * time.Millisecond)})
			if err != nil || bytes.Contains(payload, original) {
				t.Fatal("worker request contains plaintext", err)
			}
			if err := Schedule(b, Digest(original), 100*time.Millisecond); err != nil {
				t.Fatal(err)
			}
			if replace {
				if err := os.WriteFile(file, []byte("replacement"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			deadline := time.Now().Add(8 * time.Second)
			for time.Now().Before(deadline) {
				receipt, err := os.ReadFile(file + ".reader")
				if err == nil {
					pid, e := strconv.Atoi(string(receipt))
					if e != nil || pid == os.Getpid() {
						t.Fatal("expiry did not use a fresh process", e)
					}
					if syscall.Kill(pid, 0) == syscall.ESRCH {
						value, e := os.ReadFile(file)
						if e != nil || (replace && string(value) != "replacement") || (!replace && len(value) != 0) {
							t.Fatal("worker completed with incorrect clipboard state", e)
						}
						return
					}
				}
				time.Sleep(20 * time.Millisecond)
			}
			t.Fatal("expiry worker did not produce expected state")
		})
	}
}

func TestUnsupportedValuesAndMalformedWorkerRequests(t *testing.T) {
	b := Backend{Name: "macos"}
	for _, value := range [][]byte{{0}, {255}, []byte("{\\rtf1 fixture}"), []byte("%!PS-Adobe fixture")} {
		if err := b.ValidateValue(value); err == nil {
			t.Fatal("accepted incompatible clipboard value")
		}
	}
	if err := b.ValidateValue([]byte("unicode ✓\n\n")); err != nil {
		t.Fatal(err)
	}
	b, _ = backendFixture(t)
	if err := Schedule(b, "invalid", time.Second); err == nil {
		t.Fatal("worker accepted invalid digest")
	}
	b.Read[len(b.Read)-1] = "fail"
	if _, err := b.ClearIfMatching(Digest(nil)); err == nil {
		t.Fatal("backend failure ignored")
	}
}
