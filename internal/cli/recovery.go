package cli

import (
	"fmt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/store"
	"strconv"
	"time"
)

func (a *App) recovery(p invocation, s *store.Store) (any, error) {
	switch p.Command {
	case "history list":
		if len(p.Args) > 1 {
			return nil, fault.Usage("history list takes at most one entry name")
		}
		name := ""
		if len(p.Args) == 1 {
			name = p.Args[0]
		}
		entries, err := s.History(name)
		return map[string]any{"entries": entries}, err
	case "history show":
		if len(p.Args) != 1 {
			return nil, fault.Usage("history show requires a full commit hash")
		}
		return s.HistoryShow(p.Args[0])
	case "history restore":
		if len(p.Args) != 2 {
			return nil, fault.Usage("history restore requires COMMIT NAME")
		}
		if !p.has("yes") && (p.has("json") || p.has("non-interactive")) {
			return nil, fault.Interaction("history restore requires --yes to restore the selected entry")
		}
		return s.HistoryRestoreConfirmed(p.Args[0], p.Args[1], func(plan store.HistoryRestorePlan) error {
			action := "Recreate the missing entry"
			if plan.Replaces {
				action = "Replace the current entry"
			}
			return a.confirm(p, fmt.Sprintf("%s %q from commit %s.\nThe current state is retained in an encrypted backup. Continue? [y/N]: ", action, plan.Name, plan.Commit))
		})
	case "backup prune":
		if len(p.Args) != 0 {
			return nil, fault.Usage("backup prune takes no positional arguments")
		}
		retention := store.Retention{}
		if p.has("keep") {
			n, e := strconv.Atoi(p.value("keep"))
			if e != nil || n < 0 {
				return nil, fault.Usage("keep must be a nonnegative integer")
			}
			retention.Keep = &n
		}
		if p.has("older-than") {
			d, e := time.ParseDuration(p.value("older-than"))
			if e != nil || d <= 0 {
				return nil, fault.Usage("older-than must be a positive duration such as 720h")
			}
			retention.OlderThan = d
		}
		return s.Prune(retention, p.has("dry-run") || !p.has("yes"))
	case "backup list":
		if len(p.Args) != 0 {
			return nil, fault.Usage("backup list takes no positional arguments")
		}
		backups, err := s.Backups()
		return map[string]any{"backups": backups}, err
	case "backup show":
		if len(p.Args) != 1 {
			return nil, fault.Usage("backup show requires a backup identifier")
		}
		return s.BackupShow(p.Args[0])
	case "backup restore":
		if len(p.Args) != 1 {
			return nil, fault.Usage("backup restore requires a backup identifier")
		}
		if !p.has("yes") && (p.has("json") || p.has("non-interactive")) {
			return nil, fault.Interaction("backup restore requires --yes; the live entry set will match the selected snapshot")
		}
		phase := p.value("phase")
		if phase == "" {
			phase = "before"
		}
		return s.BackupRestoreConfirmed(p.Args[0], phase, func(plan store.BackupRestorePlan) error {
			return a.confirm(p, fmt.Sprintf("Restore snapshot %s (%s).\nAdd: %q\nReplace: %q\nRemove: %q\nThe current entry set is retained in an encrypted backup. Continue? [y/N]: ", plan.ID, plan.Phase, plan.Added, plan.Replaced, plan.Removed))
		})
	}
	return nil, fault.Usage("unknown recovery command")
}
