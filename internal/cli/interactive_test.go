package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agensfield/fulla/internal/config"
)

func TestEditorFixtureHelper(t *testing.T) {
	mode := os.Getenv("FULLA_EDITOR_FIXTURE")
	if mode == "" {
		return
	}
	file := os.Args[len(os.Args)-1]
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0600 {
		os.Exit(71)
	}
	info, err = os.Stat(filepath.Dir(file))
	if err != nil || info.Mode().Perm() != 0700 {
		os.Exit(72)
	}
	data, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(data, []byte{0, 255, 10, 10}) {
		os.Exit(73)
	}
	if err := os.WriteFile(os.Getenv("FULLA_EDITOR_RECEIPT"), []byte(file), 0600); err != nil {
		os.Exit(74)
	}
	if err := os.WriteFile(file+".backup", data, 0600); err != nil {
		os.Exit(75)
	}
	switch mode {
	case "fail":
		os.Exit(23)
	case "wait":
		time.Sleep(time.Minute)
	case "symlink":
		if os.Remove(file) != nil || os.Symlink(file+".backup", file) != nil {
			os.Exit(76)
		}
	case "hardlink":
		if os.Remove(file) != nil || os.Link(file+".backup", file) != nil {
			os.Exit(76)
		}
	case "mode":
		if os.Chmod(file, 0644) != nil {
			os.Exit(76)
		}
	default:
		if os.WriteFile(file, []byte{255, 0, 10}, 0600) != nil {
			os.Exit(77)
		}
	}
	os.Exit(0)
}

func TestEditorPrivateBytesCleanupAndRejection(t *testing.T) {
	for _, mode := range []string{"success", "fail", "symlink", "hardlink", "mode", "wait"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			receipt := filepath.Join(dir, "receipt")
			t.Setenv("FULLA_EDITOR_FIXTURE", mode)
			t.Setenv("FULLA_EDITOR_RECEIPT", receipt)
			tty, err := os.CreateTemp(dir, "terminal")
			if err != nil {
				t.Fatal(err)
			}
			defer tty.Close()
			a := App{Getenv: func(k string) string {
				if k == "TMPDIR" {
					return dir
				}
				return ""
			}}
			c := &config.Resolved{File: config.File{Editor: []string{os.Args[0], "-test.run=^TestEditorFixtureHelper$", "--"}}}
			ctx := context.Background()
			if mode == "wait" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 2*time.Second)
				defer cancel()
			}
			value, err := a.editValue(ctx, tty, c, []byte{0, 255, 10, 10})
			if mode == "success" {
				if err != nil || !bytes.Equal(value, []byte{255, 0, 10}) {
					t.Fatal("editor changed bytes", err)
				}
			} else if err == nil {
				t.Fatal("accepted invalid or interrupted editor")
			}
			data, e := os.ReadFile(receipt)
			if e != nil {
				t.Fatal("editor did not reach receipt", e)
			}
			if _, e := os.Stat(filepath.Dir(string(data))); !os.IsNotExist(e) {
				t.Fatal("editor directory retained", e)
			}
		})
	}
}
