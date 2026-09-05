package cli

import (
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"

	"github.com/agensfield/fulla/internal/config"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/store"
)

func (a *App) run(p invocation, c *config.Resolved, s *store.Store) error {
	if len(p.Args) != 0 || len(p.Tail) == 0 {
		return fault.Usage("run requires mappings followed by -- COMMAND [ARG...]")
	}
	mappings := map[string]string{}
	for _, mapping := range p.Flags["env"] {
		name, entry, ok := strings.Cut(mapping, "=")
		if !ok || !config.EnvName(name) {
			return fault.Usage("invalid environment mapping; expected ENV_NAME=ENTRY")
		}
		if _, err := store.EntryPath(entry); err != nil {
			return err
		}
		if _, exists := mappings[name]; exists {
			return fault.Usage("duplicate environment mapping")
		}
		mappings[name] = entry
	}
	if len(mappings) == 0 {
		return fault.Usage("run requires at least one explicit --env mapping")
	}
	if p.has("inherit") && !p.has("clean-env") {
		return fault.Usage("--inherit requires --clean-env")
	}
	inherit := append(append([]string{}, c.Run.Inherit...), p.Flags["inherit"]...)
	for _, name := range inherit {
		if !config.EnvName(name) {
			return fault.Usage("invalid inherited environment name")
		}
	}
	executable, err := exec.LookPath(p.Tail[0])
	if err != nil {
		return fault.New("run.executable_missing", "target executable could not be resolved")
	}
	env := map[string]string{}
	if p.has("clean-env") {
		for _, name := range inherit {
			if value, ok := os.LookupEnv(name); ok {
				env[name] = value
			}
		}
	} else {
		for _, item := range os.Environ() {
			name, value, ok := strings.Cut(item, "=")
			if ok {
				env[name] = value
			}
		}
	}
	// Validate all names and destinations before reading the first secret.
	for _, entry := range mappings {
		exists, err := s.Exists(entry)
		if err != nil {
			return err
		}
		if !exists {
			return fault.New("entry.not_found", "mapped entry does not exist")
		}
	}
	for name, entry := range mappings {
		value, err := s.Read(entry)
		if err != nil {
			return err
		}
		if strings.IndexByte(string(value), 0) >= 0 {
			return fault.New("run.invalid_value", "mapped entry contains NUL and cannot be represented in an environment")
		}
		env[name] = string(value)
	}
	keys := []string{}
	for name := range env {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	for _, name := range keys {
		values = append(values, name+"="+env[name])
	}
	// Exec replaces this process: native PID, terminal, streams, signals, cwd,
	// process group and exit status are inherited without a supervising parent.
	if err := syscall.Exec(executable, p.Tail, values); err != nil {
		return fault.New("run.exec_failed", "could not replace process with target executable")
	}
	return nil
}
