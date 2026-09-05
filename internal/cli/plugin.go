package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"filippo.io/age/plugin"
	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
)

// Identity interaction uses the controlling terminal, never command input/output.
// Machine mode and remote protocol handlers cannot acquire an interactive UI.
func commandIdentityUI(p invocation) *crypt.UI {
	if p.has("json") || p.has("non-interactive") || strings.HasPrefix(p.Command, "remote ") {
		return nil
	}
	read := func(prompt string) ([]byte, error) {
		value, err := withTerminal("identity input requires a controlling terminal", func(ctx context.Context, tty *os.File) ([]byte, error) {
			return terminalLine(ctx, tty, prompt)
		})
		var signal childExit
		if errors.As(err, &signal) {
			problem := fault.New("plugin.interaction_cancelled", "age plugin interaction cancelled")
			problem.Status = int(signal)
			return nil, problem
		}
		return value, err
	}
	return &crypt.UI{
		SSHPassphrase: func() ([]byte, error) {
			value, err := read("Unlock encrypted SSH identity.\npassphrase (hidden): ")
			var problem *fault.Error
			if errors.As(err, &problem) && problem.Code == "plugin.interaction_cancelled" {
				cancelled := fault.New("identity.unlock_cancelled", "SSH identity unlocking cancelled")
				cancelled.Status = problem.Status
				return value, cancelled
			}
			return value, err
		},
		ClientUI: plugin.ClientUI{
			DisplayMessage: func(name, message string) error {
				_, err := withTerminal("age plugin messages require a controlling terminal", func(_ context.Context, tty *os.File) ([]byte, error) {
					_, err := fmt.Fprintf(tty, "age plugin %q: %q\n", name, message)
					return nil, err
				})
				var problem *fault.Error
				if errors.As(err, &problem) && problem.Code == "interaction.required" {
					return nil // Informational messages alone do not require a terminal.
				}
				return err
			},
			RequestValue: func(name, prompt string, secret bool) (string, error) {
				kind := "input"
				if secret {
					kind = "secret input"
				}
				value, err := read(fmt.Sprintf("age plugin %q requests %s: %q\nvalue (hidden): ", name, kind, prompt))
				defer clear(value)
				return string(value), err
			},
			Confirm: func(name, prompt, yes, no string) (bool, error) {
				value, err := read(fmt.Sprintf("age plugin %q: %q\nyes: %q; no: %q\nallow? [y/N]: ", name, prompt, yes, no))
				defer clear(value)
				if err != nil {
					return false, err
				}
				switch strings.ToLower(strings.TrimSpace(string(value))) {
				case "y", "yes":
					return true, nil
				case "", "n", "no":
					return false, nil
				default:
					return false, fault.Usage("answer yes or no")
				}
			},
		},
	}
}
