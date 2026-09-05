package cli

import (
	"time"

	"github.com/agensfield/fulla/internal/clipboard"
	"github.com/agensfield/fulla/internal/config"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/store"
)

func (a *App) copy(p invocation, c *config.Resolved, s *store.Store) (any, error) {
	if len(p.Args) != 1 {
		return nil, fault.Usage("copy requires one entry name")
	}
	if p.has("no-clear") && p.has("clear-after") {
		return nil, fault.Usage("choose --no-clear or --clear-after, not both")
	}
	duration := c.Clipboard.ClearAfter
	if p.has("clear-after") {
		duration = p.value("clear-after")
	}
	after, err := time.ParseDuration(duration)
	if err != nil || after < 0 || after > 24*time.Hour {
		return nil, fault.Usage("clear-after must be a duration from 0s through 24h")
	}
	if p.has("no-clear") {
		after = 0
	}
	backend, err := clipboard.Detect(a.Getenv)
	if err != nil {
		return nil, err
	}
	value, err := s.Read(p.Args[0])
	if err != nil {
		return nil, err
	}
	if err := backend.ValidateValue(value); err != nil {
		return nil, err
	}
	expected := clipboard.Digest(value)
	if err := backend.WriteValue(value); err != nil {
		return nil, err
	}
	actual, err := backend.Digest()
	if err != nil || actual != expected {
		return nil, clipboardApplied("clipboard changed but exact-byte verification failed")
	}
	if after > 0 {
		if err := clipboard.Schedule(backend, expected, after); err != nil {
			return nil, clipboardApplied("value copied but clipboard expiry could not be scheduled")
		}
	}
	return commandResult{data: map[string]any{"name": p.Args[0], "backend": backend.Name, "copied": true, "clear_after": after.String(), "clear_scheduled": after > 0}, warnings: []any{"Clipboard managers may retain copied values outside Fulla’s control."}}, nil
}

func clipboardApplied(message string) *fault.Error {
	e := fault.New("clipboard.incomplete", message)
	e.Status = 3
	e.Details["applied"] = true
	return e
}
