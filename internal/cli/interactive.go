package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"unicode/utf8"

	"github.com/agensfield/fulla/internal/config"
	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func (a *App) interactiveInput(p invocation, c *config.Resolved, original []byte) ([]byte, error) {
	return withTerminal("a controlling terminal is required; use --stdin, --from-fd N, or --generate", func(ctx context.Context, tty *os.File) (value []byte, err error) {
		if p.Command == "edit" {
			value, err = a.editValue(ctx, tty, c, original)
		} else {
			var choice []byte
			choice, err = terminalLine(ctx, tty, "input: [g]enerate, [t]ype without echo, [e]ditor: ")
			if err == nil {
				switch strings.ToLower(strings.TrimSpace(string(choice))) {
				case "g", "generate":
					p.Flags["generate"] = []string{"true"}
					value, err = a.input(p, c)
				case "t", "type":
					value, err = terminalLine(ctx, tty, "secret (enter ends input, no newline is added): ")
				case "e", "editor":
					value, err = a.editValue(ctx, tty, c, nil)
				default:
					err = fault.Usage("choose generate, type, or editor")
				}
			}
		}
		return value, err
	})
}

func withTerminal(required string, run func(context.Context, *os.File) ([]byte, error)) (value []byte, err error) {
	fd, err := unix.Open("/dev/tty", unix.O_RDWR|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, fault.Interaction(required)
	}
	tty := os.NewFile(uintptr(fd), "fulla-terminal")
	defer tty.Close()
	state, err := term.GetState(fd)
	if err != nil {
		return nil, fault.Interaction("cannot inspect controlling terminal")
	}
	defer func() {
		if e := term.Restore(fd, state); e != nil && err == nil {
			value = nil
			err = fault.New("input.terminal_restore_failed", "could not restore terminal state")
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	var interrupted atomic.Int32
	go func() {
		select {
		case sig := <-signals:
			interrupted.Store(int32(sig.(syscall.Signal)))
			cancel()
		case <-ctx.Done():
		}
	}()
	value, err = run(ctx, tty)
	var failure *fault.Error
	if errors.As(err, &failure) && failure.Code == "editor.cleanup_failed" {
		return nil, err
	}
	if sig := interrupted.Load(); sig != 0 {
		return nil, childExit(128 + sig)
	}
	return value, err
}

// Polling keeps terminal reads cancellable without a blocked reader restoring
// terminal state after the descriptor has been closed or reused.
func terminalLine(ctx context.Context, tty *os.File, prompt string) (result []byte, resultErr error) {
	fd := int(tty.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, fault.Interaction("cannot configure controlling terminal")
	}
	defer func() { _ = term.Restore(fd, state); _, _ = fmt.Fprintln(tty) }()
	if _, err := fmt.Fprint(tty, prompt); err != nil {
		return nil, err
	}
	value := []byte{}
	defer func() {
		if resultErr != nil {
			clear(value)
		}
	}()
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Darwin cannot poll /dev/tty. select supports the controlling
		// terminal on both supported operating systems.
		if fd >= unix.FD_SETSIZE {
			return nil, fault.New("input.failed", "terminal descriptor exceeds select capacity")
		}
		var ready unix.FdSet
		ready.Set(fd)
		timeout := unix.Timeval{Usec: 100000}
		n, err := unix.Select(fd+1, &ready, nil, nil, &timeout)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return nil, fault.New("input.failed", "terminal read failed")
		}
		if n == 0 {
			continue
		}
		var b [1]byte
		n, err = unix.Read(fd, b[:])
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil || n == 0 {
			return nil, fault.New("input.failed", "terminal read failed")
		}
		switch b[0] {
		case '\r', '\n':
			return value, nil
		case 3:
			return nil, childExit(130)
		case 28:
			return nil, childExit(131)
		case 4:
			return nil, fault.New("input.cancelled", "input cancelled")
		case 127, 8:
			if len(value) > 0 {
				_, size := utf8.DecodeLastRune(value)
				clear(value[len(value)-size:])
				value = value[:len(value)-size]
			}
		default:
			if len(value) >= crypt.MaxEntryBytes {
				return nil, fault.New("entry.too_large", "entry exceeds the supported size limit")
			}
			value = append(value, b[0])
		}
	}
}

func (a *App) editValue(ctx context.Context, tty *os.File, c *config.Resolved, original []byte) (value []byte, err error) {
	args := append([]string(nil), c.Editor...)
	if len(args) == 0 {
		editor := a.Getenv("VISUAL")
		if editor == "" {
			editor = a.Getenv("EDITOR")
		}
		if editor == "" {
			args = []string{"vi"}
		} else {
			// Standard editor variables are trusted shell commands. TOML editor is
			// an argv array, and never passes through a shell.
			args = []string{"/bin/sh", "-c", "exec " + editor + " \"$@\"", "fulla-editor"}
		}
	}
	executable, err := exec.LookPath(args[0])
	if err != nil {
		return nil, fault.New("editor.unavailable", "configured editor is unavailable")
	}
	base := a.Getenv("TMPDIR")
	if base == "" && runtime.GOOS == "linux" {
		if info, e := os.Stat("/dev/shm"); e == nil && info.IsDir() {
			base = "/dev/shm"
		}
	}
	dir, err := os.MkdirTemp(base, "fulla-edit-")
	if err != nil {
		return nil, fault.New("editor.temp_failed", "could not create private editor directory")
	}
	cleanupDir := dir
	defer func() {
		if e := os.RemoveAll(cleanupDir); e != nil {
			value = nil
			err = fault.New("editor.cleanup_failed", "could not remove private editor material")
		}
	}()
	// macOS temporary paths may have a system-owned /var symlink. Resolve the
	// newly created private directory before applying the private-root checks.
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	root, err := securefs.Open(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := securefs.WriteNew(root, "value", original); err != nil {
		return nil, err
	}
	if _, err = fmt.Fprintln(tty, "editor is trusted plaintext code; its external backups, plugins, and telemetry are outside Fulla's control."); err != nil {
		return nil, err
	}
	if err := unix.SetNonblock(int(tty.Fd()), false); err != nil {
		return nil, err
	}
	defer unix.SetNonblock(int(tty.Fd()), true)
	command := exec.CommandContext(ctx, executable, append(args[1:], filepath.Join(dir, "value"))...)
	command.Stdin, command.Stdout, command.Stderr = tty, tty, tty
	if err := command.Run(); err != nil {
		return nil, fault.New("editor.failed", "editor did not complete successfully; entry unchanged")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	value, err = securefs.Read(root, "value", crypt.MaxEntryBytes)
	if err != nil {
		return nil, fault.New("editor.invalid_output", "edited file must remain a private regular file within the entry size limit")
	}
	return value, nil
}
