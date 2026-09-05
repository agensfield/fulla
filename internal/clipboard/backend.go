// Package clipboard owns text clipboard access and digest-only expiry workers.
package clipboard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
)

type Backend struct {
	Name  string   `json:"name"`
	Read  []string `json:"read"`
	Write []string `json:"write"`
}

func Detect(getenv func(string) string) (Backend, error) {
	var b Backend
	switch {
	case runtime.GOOS == "darwin":
		b = Backend{"macos", []string{"pbpaste", "-Prefer", "txt"}, []string{"pbcopy"}}
	case getenv("WAYLAND_DISPLAY") != "":
		b = Backend{"wayland", []string{"wl-paste", "--no-newline", "--type", "text/plain;charset=utf-8"}, []string{"wl-copy", "--type", "text/plain;charset=utf-8"}}
	case getenv("DISPLAY") != "":
		b = Backend{"x11", []string{"xclip", "-selection", "clipboard", "-out", "-target", "UTF8_STRING"}, []string{"xclip", "-selection", "clipboard", "-in", "-target", "UTF8_STRING"}}
	default:
		return b, fault.New("clipboard.unavailable", "no supported desktop clipboard; use show or run")
	}
	for _, args := range [][]string{b.Read, b.Write} {
		executable, err := exec.LookPath(args[0])
		if err != nil {
			return Backend{}, fault.New("clipboard.unavailable", "required clipboard utility is unavailable")
		}
		absolute, err := filepath.Abs(executable)
		if err != nil {
			return Backend{}, fault.New("clipboard.unavailable", "cannot resolve clipboard utility")
		}
		args[0] = absolute
	}
	return b, nil
}

func (b Backend) ValidateValue(value []byte) error {
	if len(value) > crypt.MaxEntryBytes || !utf8.Valid(value) || bytes.IndexByte(value, 0) >= 0 {
		return fault.New("clipboard.unsupported_value", "clipboard requires UTF-8 text without NUL; use show or run for other values")
	}
	if b.Name == "macos" && (bytes.HasPrefix(value, []byte("{\\rtf")) || bytes.HasPrefix(value, []byte("%!PS"))) {
		return fault.New("clipboard.unsupported_value", "macOS would interpret this value as a document; use show for exact bytes")
	}
	return nil
}

// Backend processes receive operational desktop variables, never arbitrary
// caller credentials or the copied secret through their environment.
func environment() []string {
	names := []string{"PATH", "HOME", "DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "XAUTHORITY", "DBUS_SESSION_BUS_ADDRESS"}
	env := []string{"LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8", "LC_CTYPE=en_US.UTF-8"}
	for _, name := range names {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
}

func command(ctx context.Context, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = environment()
	cmd.Stderr = nil
	cmd.WaitDelay = time.Second
	return cmd
}

func (b Backend) WriteValue(value []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := command(ctx, b.Write)
	cmd.Stdin = bytes.NewReader(value)
	cmd.Stdout = nil
	if err := cmd.Start(); err != nil {
		return fault.New("clipboard.start_failed", "could not start clipboard utility")
	}
	if err := cmd.Wait(); err != nil {
		e := fault.New("clipboard.write_failed", "clipboard write did not complete; clipboard state may have changed")
		e.Status = 3
		e.Details["state_uncertain"] = true
		return e
	}
	return nil
}

type digestWriter struct {
	hash  hash.Hash
	count int
}

func (w *digestWriter) Write(p []byte) (int, error) {
	if len(p) > crypt.MaxEntryBytes-w.count {
		return 0, io.ErrShortWrite
	}
	w.count += len(p)
	return w.hash.Write(p)
}

func (b Backend) Digest() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output := &digestWriter{hash: sha256.New()}
	cmd := command(ctx, b.Read)
	cmd.Stdout = output
	if err := cmd.Run(); err != nil {
		return "", fault.New("clipboard.read_failed", "could not verify clipboard contents")
	}
	return hex.EncodeToString(output.hash.Sum(nil)), nil
}

func Digest(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }

func (b Backend) ClearIfMatching(expected string) (bool, error) {
	actual, err := b.Digest()
	if err != nil || actual != expected {
		return false, err
	}
	if err := b.WriteValue(nil); err != nil {
		return false, err
	}
	return true, nil
}

func (b Backend) valid() bool {
	for _, args := range [][]string{b.Read, b.Write} {
		if len(args) == 0 || len(args) > 16 || !strings.HasPrefix(args[0], "/") {
			return false
		}
		for _, arg := range args {
			if strings.ContainsRune(arg, 0) {
				return false
			}
		}
	}
	return b.Name == "macos" || b.Name == "wayland" || b.Name == "x11"
}
