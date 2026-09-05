package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrecedenceAndStrictness(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(home, "config.toml")
	env := map[string]string{"HOME": home, "FULLA_CONFIG": name, "PA_DIR": "/compat", "FULLA_DIR": "/env", "PA_PASSWORD": "never-input"}
	get := func(k string) string { return env[k] }
	if err := os.WriteFile(name, []byte("store = '/toml'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Resolve(Flags{Store: "/flag"}, get)
	if err != nil {
		t.Fatal(err)
	}
	if c.StorePath != "/flag" {
		t.Fatal(c.StorePath)
	}
	c, err = Resolve(Flags{}, get)
	if err != nil || c.StorePath != "/env" {
		t.Fatal(c, err)
	}
	delete(env, "FULLA_DIR")
	c, err = Resolve(Flags{}, get)
	if err != nil || c.StorePath != "/toml" {
		t.Fatal(c, err)
	}
	for _, input := range []string{"unknown = 'private-value'", "store = 42", "store='a'\nstore='b'", "[generation]\nlength=0", "[generation]\nalphabet='aa'"} {
		if err := os.WriteFile(name, []byte(input), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := Resolve(Flags{}, get)
		if err == nil {
			t.Fatalf("accepted %s", input)
		}
		if strings.Contains(err.Error(), "private-value") {
			t.Fatal("echoed config value")
		}
	}
	if err := os.WriteFile(name, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(name, 0o666); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(Flags{}, get); err == nil {
		t.Fatal("accepted writable config")
	}
}

func TestSettingProvenanceIncludesExplicitDefaultsAndEditorFallbacks(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "config"), "XDG_DATA_HOME": filepath.Join(home, "data"), "EDITOR": "editor fixture", "VISUAL": "visual fixture"}
	get := func(k string) string { return env[k] }
	c, err := Resolve(Flags{}, get)
	if err != nil {
		t.Fatal("missing default XDG config should be optional", err)
	}
	if c.Sources["config"] != "XDG_CONFIG_HOME" || c.Sources["store"] != "XDG_DATA_HOME" || c.Sources["editor"] != "VISUAL" || c.Sources["generation.length"] != "default" {
		t.Fatal(c.Sources)
	}
	name := filepath.Join(home, "config.toml")
	if err := os.WriteFile(name, []byte("editor=['fixture']\n[generation]\nlength=32\n[clipboard]\nclear_after='45s'\n[run]\ninherit=[]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err = Resolve(Flags{Config: name}, get)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"editor", "generation.length", "clipboard.clear_after", "run.inherit"} {
		if c.Sources[key] != "config" {
			t.Fatal("explicit default lost provenance", key, c.Sources)
		}
	}
	if c.Sources["generation.alphabet"] != "default" || c.Sources["config"] != "flag" {
		t.Fatal(c.Sources)
	}
	delete(env, "VISUAL")
	c, err = Resolve(Flags{}, get)
	if err != nil || c.Sources["editor"] != "EDITOR" {
		t.Fatal(c, err)
	}
	if _, err := Resolve(Flags{Config: filepath.Join(home, "absent")}, get); err == nil {
		t.Fatal("missing explicit config accepted")
	}
}
