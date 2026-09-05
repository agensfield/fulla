package cli

import (
	"strings"

	"github.com/agensfield/fulla/internal/fault"
)

var aliases = map[string]string{"setup": "init", "new": "add", "create": "add", "get": "show", "cat": "show", "clip": "copy", "update": "edit", "ls": "list", "rm": "remove", "delete": "remove", "del": "remove", "mv": "move", "rename": "move", "exec": "run", "st": "status", "check": "doctor", "diagnose": "doctor"}

type invocation struct {
	Command string
	Args    []string
	Flags   map[string][]string
	Tail    []string
}

var booleanFlags = map[string]bool{"json": true, "non-interactive": true, "yes": true, "help": true, "version": true, "stdin": true, "generate": true, "no-git": true, "adopt": true, "dry-run": true, "permanent-delete": true, "deep": true, "fix-permissions": true, "clean-env": true, "no-clear": true, "full": true, "fail-on-skip": true, "destroy-retired-key": true, "compromise": true}
var valueFlags = map[string]bool{"store": true, "config": true, "from-fd": true, "length": true, "alphabet": true, "env": true, "inherit": true, "clear-after": true, "recipient": true, "identity": true, "output": true, "manifest": true, "passphrase-fd": true, "expect-fingerprint": true, "host": true, "remote-store": true, "acknowledge": true, "recover-lock": true, "older-than": true, "keep": true, "report": true, "phase": true, "remote-binary": true, "ssh-option": true}

func parse(args []string) (invocation, error) {
	p := invocation{Args: []string{}, Flags: map[string][]string{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			p.Tail = args[i+1:]
			break
		}
		if a == "-h" {
			a = "--help"
		}
		if a == "-v" {
			a = "--version"
		}
		if strings.HasPrefix(a, "-") {
			if !strings.HasPrefix(a, "--") {
				return p, fault.Usage("unknown short option")
			}
			name, value, has := strings.Cut(strings.TrimPrefix(a, "--"), "=")
			if booleanFlags[name] {
				if has {
					return p, fault.Usage("boolean option does not accept a value")
				}
				p.Flags[name] = append(p.Flags[name], "true")
				continue
			}
			if !valueFlags[name] {
				return p, fault.Usage("unknown option: --" + name)
			}
			if !has {
				i++
				if i >= len(args) {
					return p, fault.Usage("missing value for --" + name)
				}
				value = args[i]
			}
			if value == "" {
				return p, fault.Usage("empty value for --" + name)
			}
			p.Flags[name] = append(p.Flags[name], value)
			continue
		}
		p.Args = append(p.Args, a)
	}
	if len(p.Args) > 0 {
		p.Command = p.Args[0]
		p.Args = p.Args[1:]
		if canonical, ok := aliases[p.Command]; ok {
			p.Command = canonical
		}
	}
	for name, values := range p.Flags {
		if len(values) > 1 && name != "env" && name != "inherit" && name != "recipient" && name != "ssh-option" {
			return p, fault.Usage("duplicate option: --" + name)
		}
	}
	return p, nil
}
func (p invocation) has(name string) bool { return len(p.Flags[name]) > 0 }
func (p invocation) value(name string) string {
	v := p.Flags[name]
	if len(v) == 0 {
		return ""
	}
	return v[0]
}

func (p invocation) allow(names ...string) error {
	allowed := map[string]bool{"json": true, "non-interactive": true, "yes": true, "help": true, "version": true, "store": true, "config": true}
	for _, name := range names {
		allowed[name] = true
	}
	for name := range p.Flags {
		if !allowed[name] {
			return fault.Usage("option --" + name + " is not valid for " + p.Command)
		}
	}
	return nil
}
