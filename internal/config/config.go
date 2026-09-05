// Package config resolves the deliberately small, non-secret user configuration.
package config

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
	"github.com/pelletier/go-toml/v2"
)

type File struct {
	Store      string     `toml:"store"`
	Editor     []string   `toml:"editor"`
	Generation Generation `toml:"generation"`
	Clipboard  Clipboard  `toml:"clipboard"`
	Run        Run        `toml:"run"`
}
type Generation struct {
	Length   int    `toml:"length"`
	Alphabet string `toml:"alphabet"`
}
type Clipboard struct {
	ClearAfter string `toml:"clear_after"`
}
type Run struct {
	Inherit []string `toml:"inherit"`
}
type Flags struct{ Config, Store string }
type Resolved struct {
	File
	ConfigPath string            `json:"config_path"`
	StorePath  string            `json:"store_path"`
	Sources    map[string]string `json:"sources"`
}

func Resolve(flags Flags, getenv func(string) string) (*Resolved, error) {
	home := getenv("HOME")
	if home == "" {
		return nil, fault.New("config.invalid", "HOME must be set")
	}
	c := &Resolved{File: File{Generation: Generation{Length: 32, Alphabet: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"}, Clipboard: Clipboard{ClearAfter: "45s"}}, Sources: map[string]string{}}
	base := getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(home, ".config")
	}
	c.ConfigPath = filepath.Join(base, "fulla", "config.toml")
	c.Sources["config"] = "default"
	if getenv("XDG_CONFIG_HOME") != "" {
		c.Sources["config"] = "XDG_CONFIG_HOME"
	}
	for _, key := range []string{"editor", "generation.length", "generation.alphabet", "clipboard.clear_after", "run.inherit"} {
		c.Sources[key] = "default"
	}
	if p := getenv("FULLA_CONFIG"); p != "" {
		c.ConfigPath = p
		c.Sources["config"] = "FULLA_CONFIG"
	}
	if flags.Config != "" {
		c.ConfigPath = flags.Config
		c.Sources["config"] = "flag"
	}
	var err error
	c.ConfigPath, err = filepath.Abs(c.ConfigPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(c.ConfigPath)
	if err == nil {
		if _, err := securefs.Canonical(c.ConfigPath, false); err != nil {
			return nil, fault.New("config.unsafe", err.Error())
		}
		if err := securefs.ValidateInfo(c.ConfigPath, info, false); err != nil {
			return nil, fault.New("config.unsafe", err.Error())
		}
		if !info.Mode().IsRegular() || info.Size() > 1<<20 {
			return nil, fault.New("config.invalid", "config must be a regular file no larger than 1 MiB")
		}
		data, err := os.ReadFile(c.ConfigPath)
		if err != nil {
			return nil, fault.New("config.unreadable", "cannot read configuration")
		}
		decoder := toml.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&c.File); err != nil {
			// TOML errors may contain untrusted values. Locations and unknown key
			// paths are safe; input excerpts are deliberately never echoed.
			e := fault.New("config.invalid", "invalid TOML key, duplicate definition, type, or syntax")
			var strict *toml.StrictMissingError
			if errors.As(err, &strict) {
				keys := []string{}
				for _, item := range strict.Errors {
					keys = append(keys, joinKey(item.Key()))
				}
				e.Details["keys"] = keys
			}
			var decode *toml.DecodeError
			if errors.As(err, &decode) {
				row, col := decode.Position()
				e.Details["line"] = row
				e.Details["column"] = col
			}
			return nil, e
		}
		var declared map[string]any
		if err := toml.Unmarshal(data, &declared); err != nil {
			return nil, fault.New("config.invalid", "cannot inspect configuration settings")
		}
		for _, key := range []string{"editor", "generation.length", "generation.alphabet", "clipboard.clear_after", "run.inherit"} {
			if declaredSetting(declared, key) {
				c.Sources[key] = "config"
			}
		}
	} else if !errors.Is(err, fs.ErrNotExist) || c.Sources["config"] == "flag" || c.Sources["config"] == "FULLA_CONFIG" {
		return nil, fault.New("config.unreadable", "selected configuration file is missing or unreadable")
	}
	if c.Generation.Length < 1 || c.Generation.Length > 65536 {
		return nil, invalid("generation.length", "integer between 1 and 65536")
	}
	if len(c.Generation.Alphabet) < 2 || len(c.Generation.Alphabet) > 256 {
		return nil, invalid("generation.alphabet", "2 to 256 distinct printable ASCII characters")
	}
	seen := map[byte]bool{}
	for _, b := range []byte(c.Generation.Alphabet) {
		if b < 33 || b > 126 || seen[b] {
			return nil, invalid("generation.alphabet", "distinct printable ASCII characters")
		}
		seen[b] = true
	}
	if d, err := time.ParseDuration(c.Clipboard.ClearAfter); err != nil || d < 0 || d > 24*time.Hour {
		return nil, invalid("clipboard.clear_after", "duration from 0s through 24h")
	}
	for _, name := range c.Run.Inherit {
		if !EnvName(name) {
			return nil, invalid("run.inherit", "valid environment variable names")
		}
	}
	for _, word := range c.Editor {
		if word == "" {
			return nil, invalid("editor", "nonempty executable and argument strings")
		}
	}
	if len(c.Editor) == 0 {
		c.Sources["editor"] = "default"
		if getenv("EDITOR") != "" {
			c.Sources["editor"] = "EDITOR"
		}
		if getenv("VISUAL") != "" {
			c.Sources["editor"] = "VISUAL"
		}
	}
	dataHome := getenv("XDG_DATA_HOME")
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	c.StorePath = filepath.Join(dataHome, "fulla")
	c.Sources["store"] = "default"
	if getenv("XDG_DATA_HOME") != "" {
		c.Sources["store"] = "XDG_DATA_HOME"
	}
	for _, candidate := range []struct{ value, source string }{{getenv("PA_DIR"), "PA_DIR"}, {c.Store, "config"}, {getenv("FULLA_DIR"), "FULLA_DIR"}, {flags.Store, "flag"}} {
		if candidate.value != "" {
			c.StorePath = candidate.value
			c.Sources["store"] = candidate.source
		}
	}
	c.StorePath, err = filepath.Abs(c.StorePath)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func invalid(key, expected string) *fault.Error {
	e := fault.New("config.invalid", "invalid configuration setting")
	e.Details["key"] = key
	e.Details["expected"] = expected
	return e
}
func joinKey(parts []string) string {
	s := ""
	for _, p := range parts {
		if s != "" {
			s += "."
		}
		s += p
	}
	return s
}
func EnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, c := range name {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_' || (i > 0 && c >= '0' && c <= '9') {
			continue
		}
		return false
	}
	return true
}

func declaredSetting(values map[string]any, key string) bool {
	parts := strings.Split(key, ".")
	for index, part := range parts {
		value, ok := values[part]
		if !ok {
			return false
		}
		if index == len(parts)-1 {
			return true
		}
		values, ok = value.(map[string]any)
		if !ok {
			return false
		}
	}
	return false
}
