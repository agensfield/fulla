package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestHumanResultPreservesMetadataAndEscapesControls(t *testing.T) {
	data := struct {
		Applied bool           `json:"applied"`
		Count   uint64         `json:"count"`
		Names   []string       `json:"names"`
		Details map[string]any `json:"details"`
		Secret  string         `json:"-"`
	}{
		Count:   18446744073709551615,
		Names:   []string{"normal/şifre", "line\n\x1b[31m\u202ehidden", ""},
		Details: map[string]any{"empty": []string{}, "missing": nil, "line\nkey": true},
		Secret:  "excluded-private-material",
	}
	var out bytes.Buffer
	if err := writeHumanResult(&out, "add", data); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"fulla add\n", "applied: no\n", "count: 18446744073709551615\n",
		`- "normal/şifre"`, `- "line\n\x1b[31m\u202ehidden"`, `- ""`,
		"empty:\n      (none)\n", "missing: (not set)\n", `line\nkey: yes`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %q", want, out.String())
		}
	}
	for _, forbidden := range []string{data.Secret, "\x1b", "\u202e", "line\nkey"} {
		if strings.Contains(out.String(), forbidden) {
			t.Errorf("unsafe output contains %q", forbidden)
		}
	}
}

func TestSuccessSelectsHumanOrMachineOutput(t *testing.T) {
	for _, machine := range []bool{false, true} {
		var out, diagnostic bytes.Buffer
		a := App{Out: &out, Err: &diagnostic}
		result := commandResult{data: map[string]any{"names": []string{}}, warnings: []any{"fixture warning"}}
		if code := a.success("list", result, machine); code != 0 {
			t.Fatal(code)
		}
		if machine {
			var envelope struct {
				Schema   string
				OK       bool
				Command  string
				Data     struct{ Names []string }
				Warnings []string
			}
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Schema != "fulla.cli/v1" || !envelope.OK || envelope.Command != "list" || envelope.Data.Names == nil || len(envelope.Warnings) != 1 || diagnostic.Len() != 0 {
				t.Fatalf("machine contract changed: %s %s", &out, &diagnostic)
			}
		} else if out.String() != "fulla list\n  names:\n    (none)\n" || diagnostic.String() != "fixture warning\n" {
			t.Fatalf("unexpected human result: %q %q", &out, &diagnostic)
		}
	}
}

type failedHumanWriter struct{}

func (failedHumanWriter) Write([]byte) (int, error) { return 0, errors.New("fixture output failure") }

func TestHumanOutputFailureIsNonzero(t *testing.T) {
	a := App{Out: failedHumanWriter{}, Err: &bytes.Buffer{}}
	if code := a.success("list", map[string]any{"names": []string{"entry"}}, false); code != 3 {
		t.Fatalf("output failure returned %d", code)
	}
}
