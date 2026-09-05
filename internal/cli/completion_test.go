package cli

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestCompletionWithoutStoreAndShellSyntax(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			var out, diagnostics bytes.Buffer
			a := App{Out: &out, Err: &diagnostics, Getenv: func(string) string { return "/absent/fulla-fixture" }}
			if code := a.Main([]string{"completion", shell}); code != 0 {
				t.Fatalf("generation failed %d: %s", code, diagnostics.String())
			}
			binary, err := exec.LookPath(shell)
			if err != nil {
				t.Skipf("%s unavailable for shell syntax acceptance", shell)
			}
			cmd := exec.Command(binary, "-n")
			cmd.Stdin = bytes.NewReader(out.Bytes())
			if result, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("invalid shell script: %v %s", err, result)
			}
		})
	}
}

func TestBashCompletionFindsGroupedCommandsAfterGlobalOptions(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	var script bytes.Buffer
	a := App{Out: &script}
	if code := a.Main([]string{"completion", "bash"}); code != 0 {
		t.Fatal(code)
	}
	script.WriteString(`
COMP_WORDS=(fulla --store '/fixture with spaces' --json peer r)
COMP_CWORD=5
_fulla
printf '%s\n' "${COMPREPLY[@]}"
COMP_WORDS=(fulla git -- '')
COMP_CWORD=3
_fulla
[ "${#COMPREPLY[@]}" -eq 0 ] || exit 72
`)
	command := exec.Command(bash)
	command.Stdin = &script
	out, err := command.CombinedOutput()
	if err != nil || string(out) != "rotate\nremove\n" {
		t.Fatalf("wrong completion: %v %q", err, out)
	}
}

func TestCompletionRejectsJSONAndUnknownShell(t *testing.T) {
	for _, args := range [][]string{{"completion", "bash", "--json"}, {"completion", "unknown", "--json"}} {
		var out bytes.Buffer
		a := App{Out: &out}
		if code := a.Main(args); code != 2 || !strings.Contains(out.String(), "fulla.cli/v1") || strings.Contains(out.String(), "compdef") {
			t.Fatalf("mixed shell output and JSON: %d %s", code, out.String())
		}
	}
}
