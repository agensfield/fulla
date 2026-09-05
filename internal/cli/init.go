package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/agensfield/fulla/internal/config"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/store"
)

func (a *App) initialize(p invocation, c *config.Resolved) (any, error) {
	if len(p.Args) != 0 {
		return nil, fault.Usage("init takes no positional arguments")
	}
	if p.has("adopt") && p.has("no-git") {
		return nil, fault.Usage("adoption preserves existing Git state")
	}
	apply := func(dry bool) (store.InitResult, error) {
		if p.has("adopt") {
			return store.Adopt(c.StorePath, dry, commandPluginUI(p))
		}
		return store.Init(c.StorePath, p.has("no-git"), dry)
	}
	if p.has("dry-run") {
		return apply(true)
	}
	if p.has("yes") {
		return apply(false)
	}
	if p.has("json") || p.has("non-interactive") {
		return nil, fault.Interaction("initialization preflight requires --yes (Git history is enabled unless --no-git)")
	}
	preflight, err := apply(true)
	if err != nil {
		return nil, err
	}
	action := "Create a new Fulla store"
	if p.has("adopt") {
		action = "Adopt the existing pa store in place, preserving its identity and entries"
	}
	history := "Git history is disabled; transactional backups are still retained."
	if preflight.Git {
		history = "Git retains encrypted history and exposes entry names and change times."
	}
	if err := a.confirm(p, fmt.Sprintf("%s at %q.\n%s\nContinue? [y/N]: ", action, c.StorePath, history)); err != nil {
		return nil, err
	}
	// The apply operation repeats validation; confirmation is not a stale-state
	// exemption and never allows replacing an existing destination.
	return apply(false)
}

// confirm accepts routine authority only. Fingerprints and scoped destructive
// acknowledgements remain explicit command inputs, never implied by --yes.
func (a *App) confirm(p invocation, prompt string) error {
	if p.has("yes") {
		return nil
	}
	if p.has("json") || p.has("non-interactive") {
		return fault.Interaction("routine confirmation requires --yes")
	}
	_, err := withTerminal("routine confirmation requires a controlling terminal; provide --yes for noninteractive operation", func(ctx context.Context, tty *os.File) ([]byte, error) {
		answer, err := terminalLine(ctx, tty, prompt)
		if err != nil {
			return nil, err
		}
		switch strings.ToLower(strings.TrimSpace(string(answer))) {
		case "y", "yes":
			return nil, nil
		case "", "n", "no":
			return nil, fault.New("input.cancelled", "operation cancelled; no changes applied")
		default:
			return nil, fault.Usage("answer yes or no")
		}
	})
	return err
}
