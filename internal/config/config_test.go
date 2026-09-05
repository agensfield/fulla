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
