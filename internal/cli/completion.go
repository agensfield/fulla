package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/agensfield/fulla/internal/fault"
)

var commandGroups = map[string]string{
	"peer": "add list show rotate remove", "identity": "show rotate",
	"transfer": "export verify import", "history": "list show restore",
	"backup": "list show restore prune export", "remote": "serve",
}

const commands = "init add show copy edit list remove move run sync peer identity transfer history backup status doctor git completion version remote"
const globalOptions = "--store --config --json --non-interactive --yes --help --version"

func (a *App) completion(p invocation) error {
	if err := p.allow(); err != nil {
		return err
	}
	if p.has("json") {
		return fault.Usage("completion generates shell code and does not support --json")
	}
	if len(p.Args) != 1 {
		return fault.Usage("completion requires bash, zsh, or fish")
	}
	groups := make([]string, 0, len(commandGroups))
	for group := range commandGroups {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	var script strings.Builder
	switch p.Args[0] {
	case "bash", "zsh":
		zsh := p.Args[0] == "zsh"
		if zsh {
			script.WriteString("#compdef fulla\n_fulla() {\n  local -a args candidates\n  args=(\"${words[@]:1:$((CURRENT-2))}\")\n")
		} else {
			script.WriteString("_fulla() {\n  local -a args\n  COMPREPLY=()\n  args=(\"${COMP_WORDS[@]:1:$((COMP_CWORD-1))}\")\n")
		}
		script.WriteString("  local word first='' second='' skip='' choices=''\n  for word in \"${args[@]}\"; do\n    if [ -n \"$skip\" ]; then skip=''; continue; fi\n    case \"$word\" in\n      --) return ;;\n      --store|--config) skip=1; continue ;;\n      --*) continue ;;\n    esac\n    if [ -z \"$first\" ]; then first=$word; else second=$word; break; fi\n  done\n  [ -n \"$skip\" ] && return\n")
		fmt.Fprintf(&script, "  if [ -z \"$first\" ]; then choices='%s'; elif [ -z \"$second\" ]; then\n    case \"$first\" in\n", commands)
		for _, group := range groups {
			fmt.Fprintf(&script, "      %s) choices='%s' ;;\n", group, commandGroups[group])
		}
		script.WriteString("      completion) choices='bash zsh fish' ;;\n    esac\n  fi\n")
		fmt.Fprintf(&script, "  choices=\"$choices %s\"\n", globalOptions)
		if zsh {
			script.WriteString("  candidates=(${=choices})\n  compadd -- \"${candidates[@]}\"\n}\ncompdef _fulla fulla\n")
		} else {
			script.WriteString("  COMPREPLY=()\n  while IFS= read -r word; do COMPREPLY+=(\"$word\"); done < <(compgen -W \"$choices\" -- \"${COMP_WORDS[COMP_CWORD]}\")\n}\ncomplete -F _fulla fulla\n")
		}
	case "fish":
		script.WriteString(`function __fulla_position
    set -l words (commandline -opc)
    set -e words[1]
    set -l first ''
    set -l skip ''
    for word in $words
        if test -n "$skip"
            set skip ''
            continue
        end
        switch $word
            case --
                return 1
            case --store --config
                set skip 1
                continue
            case '--*'
                continue
        end
        if test -n "$first"
            return 1
        end
        set first $word
    end
    test -z "$skip"; and test "$first" = "$argv[1]"
end
complete -c fulla -f
`)
		fmt.Fprintf(&script, "complete -c fulla -n \"__fulla_position ''\" -a '%s'\n", commands)
		for _, group := range groups {
			fmt.Fprintf(&script, "complete -c fulla -n '__fulla_position %s' -a '%s'\n", group, commandGroups[group])
		}
		script.WriteString("complete -c fulla -n '__fulla_position completion' -a 'bash zsh fish'\n")
		for _, option := range strings.Fields(globalOptions) {
			fmt.Fprintf(&script, "complete -c fulla -l %s\n", strings.TrimPrefix(option, "--"))
		}
	default:
		return fault.Usage("unsupported completion shell; use bash, zsh, or fish")
	}
	_, err := io.WriteString(a.Out, script.String())
	return err
}
