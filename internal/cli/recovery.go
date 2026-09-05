package cli

import (
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/store"
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
		if !p.has("yes") {
			return nil, fault.Interaction("history restore requires --yes to restore the selected entry")
		}
		return s.HistoryRestore(p.Args[0], p.Args[1])
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
		if !p.has("yes") {
			return nil, fault.Interaction("backup restore requires --yes; the live entry set will match the selected snapshot")
		}
		phase := p.value("phase")
		if phase == "" {
			phase = "before"
		}
		return s.BackupRestore(p.Args[0], phase)
	}
	return nil, fault.Usage("unknown recovery command")
}
