package cli

import (
	"errors"
	"os/exec"
	"syscall"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/store"
)

// childExit preserves the expert passthrough's status without adding Fulla
// prose to streams already owned by Git.
type childExit int

func (e childExit) Error() string { return "Git child exited" }

func (a *App) git(p invocation, s *store.Store) (err error) {
	if len(p.Args) != 0 || len(p.Tail) == 0 {
		return fault.Usage("git requires arguments after --")
	}
	enabled, err := s.GitEnabled()
	if err != nil {
		return err
	}
	if !enabled {
		return fault.New("git.disabled", "this store does not have Git history")
	}
	lock, err := s.Lock("git")
	if err != nil {
		return err
	}
	started := false
	defer func() {
		if release := lock.Release(); release != nil {
			if started {
				err = fault.Applied("Git exited but store lock release failed; inspect repository state", "")
			} else {
				err = release
			}
		}
	}()
	// This is an explicit expert escape hatch, including the user's Git config
	// and arguments. Ordinary Fulla Git operations use the restricted helper.
	command := exec.Command("git", append([]string{"-C", s.PasswordDirectory()}, p.Tail...)...)
	command.Stdin, command.Stdout, command.Stderr = a.In, a.Out, a.Err
	if err := command.Start(); err != nil {
		return fault.New("git.start_failed", "could not start Git")
	}
	started = true
	if err := command.Wait(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				return childExit(128 + int(status.Signal()))
			}
			return childExit(exit.ExitCode())
		}
		return fault.Applied("could not observe Git completion; inspect repository state", "")
	}
	return nil
}
