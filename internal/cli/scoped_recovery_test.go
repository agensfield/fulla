package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// These names and bytes are synthetic infrastructure examples, never the live
// Agensfield credential inventory or an authorization to revoke credentials.
func TestInfrastructureManifestRecoveryJourney(t *testing.T) {
	for _, git := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-git", true: "git"}[git], func(t *testing.T) {
			home, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			source, recovery, target := filepath.Join(home, "source"), filepath.Join(home, "recovery"), filepath.Join(home, "target")
			invoke := func(dir string, input []byte, expected int, args ...string) map[string]any {
				t.Helper()
				var out, diagnostic bytes.Buffer
				a := App{In: bytes.NewReader(input), Out: &out, Err: &diagnostic, Getenv: func(k string) string {
					if k == "HOME" {
						return home
					}
					return ""
				}}
				code := a.Main(append([]string{"--store", dir, "--json"}, args...))
				if code != expected || diagnostic.Len() != 0 {
					t.Fatalf("%v: status=%d expected=%d", args, code, expected)
				}
				var result struct {
					Schema string
					OK     bool
					Data   map[string]any
					Error  map[string]any
				}
				if err := json.Unmarshal(out.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.Schema != "fulla.cli/v1" || result.OK != (expected == 0) {
					t.Fatal("invalid envelope")
				}
				if expected != 0 {
					return result.Error
				}
				return result.Data
			}
			for _, dir := range []string{source, recovery, target} {
				args := []string{"init", "--yes"}
				if !git {
					args = append(args, "--no-git")
				}
				invoke(dir, nil, 0, args...)
			}
			values := map[string][]byte{"agensfield/host-recovery": {0, 255, 10, 10}, "agensfield/control-plane": []byte("synthetic-offline-recovery"), "personal/excluded": []byte("synthetic-unrelated-secret")}
			selected := []string{"agensfield/control-plane", "agensfield/host-recovery"}
			for name, value := range values {
				invoke(source, value, 0, "add", name, "--stdin")
			}
			recipient := invoke(recovery, nil, 0, "identity", "show")["recipient"].(string)
			active := invoke(source, nil, 0, "identity", "show")["recipient"].(string)
			identity := filepath.Join(recovery, "identities")
			manifest, capsule := filepath.Join(home, "manifest.json"), filepath.Join(home, "scoped.age")
			writeManifest := func(names []string) {
				t.Helper()
				data, err := json.Marshal(names)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(manifest, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			before := recoveryInventory(t, source)
			for _, names := range [][]string{{selected[0], "absent"}, {selected[0], selected[0]}, {"agensfield/*"}, {}} {
				writeManifest(names)
				expected := 1
				if len(names) == 0 || len(names) == 2 && names[0] == names[1] {
					expected = 2
				}
				invoke(source, nil, expected, "transfer", "export", "--manifest", manifest, "--recipient", recipient, "--output", capsule)
				if _, err := os.Lstat(capsule); !os.IsNotExist(err) {
					t.Fatal("bad selector published artifact")
				}
				if !reflect.DeepEqual(before, recoveryInventory(t, source)) {
					t.Fatal("bad selector changed source")
				}
			}
			writeManifest(selected)
			exported := invoke(source, nil, 0, "transfer", "export", "--manifest", manifest, "--recipient", recipient, "--output", capsule)
			checkScope := func(data map[string]any) {
				t.Helper()
				encoded, _ := json.Marshal(data["names"])
				want, _ := json.Marshal(selected)
				if !bytes.Equal(encoded, want) || data["entries"] != float64(len(selected)) {
					t.Fatal("capsule or receipt exceeded manifest")
				}
			}
			checkScope(exported)
			receiptPath := exported["receipt"].(string)
			receipt, err := os.ReadFile(filepath.Join(source, receiptPath))
			if err != nil {
				t.Fatal(err)
			}
			var recorded map[string]any
			if err := json.Unmarshal(receipt, &recorded); err != nil {
				t.Fatal(err)
			}
			checkScope(recorded)
			for _, value := range values {
				if bytes.Contains(receipt, value) {
					t.Fatal("receipt exposed a value")
				}
			}
			afterExport := recoveryInventory(t, source)
			delete(afterExport, receiptPath)
			if !reflect.DeepEqual(before, afterExport) {
				t.Fatal("export changed source beyond its receipt")
			}
			info, err := os.Stat(capsule)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("capsule is not private", err)
			}
			beforeVerify := recoveryInventory(t, source)
			absentStore := filepath.Join(home, "never-created")
			verified := invoke(absentStore, nil, 0, "transfer", "verify", capsule, "--identity", identity)
			checkScope(verified)
			if _, err := os.Lstat(absentStore); !os.IsNotExist(err) {
				t.Fatal("isolated verification created a store")
			}
			if !reflect.DeepEqual(beforeVerify, recoveryInventory(t, source)) {
				t.Fatal("verification changed source")
			}
			invoke(source, nil, 1, "backup", "export", "--full", "--recipient", active, "--output", filepath.Join(home, "circular.age"))
			if !reflect.DeepEqual(beforeVerify, recoveryInventory(t, source)) {
				t.Fatal("circular export changed source")
			}
			if _, err := os.Lstat(filepath.Join(home, "circular.age")); !os.IsNotExist(err) {
				t.Fatal("circular protection published an artifact")
			}
			// A valid whole-store disaster image is a different recovery product,
			// not an acceptable substitute for this logical-entry capsule.
			full := filepath.Join(home, "whole-store.age")
			invoke(source, nil, 0, "backup", "export", "--full", "--recipient", recipient, "--output", full)
			beforeRefusal := recoveryInventory(t, target)
			invoke(absentStore, nil, 1, "transfer", "verify", full, "--identity", identity)
			invoke(target, nil, 1, "transfer", "import", full, "--identity", identity)
			if !reflect.DeepEqual(beforeRefusal, recoveryInventory(t, target)) {
				t.Fatal("full-archive substitution changed logical recovery target")
			}
			invoke(target, nil, 0, "transfer", "import", capsule, "--identity", identity)
			check := invoke(target, nil, 0, "list")
			encoded, _ := json.Marshal(check["names"])
			want, _ := json.Marshal(selected)
			if !bytes.Equal(encoded, want) {
				t.Fatal("isolated recovery imported unselected entries")
			}
			for _, name := range selected {
				var out bytes.Buffer
				a := App{In: bytes.NewReader(nil), Out: &out, Err: &bytes.Buffer{}, Getenv: func(k string) string {
					if k == "HOME" {
						return home
					}
					return ""
				}}
				if a.Main([]string{"--store", target, "show", name}) != 0 || !bytes.Equal(out.Bytes(), values[name]) {
					t.Fatal("isolated recovery lost exact bytes")
				}
			}
		})
	}
}

func recoveryInventory(t *testing.T, root string) map[string]struct {
	Mode fs.FileMode
	Hash [32]byte
} {
	t.Helper()
	result := map[string]struct {
		Mode fs.FileMode
		Hash [32]byte
	}{}
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		var hash [32]byte
		if !entry.IsDir() {
			data, err := os.ReadFile(name)
			if err != nil {
				return err
			}
			hash = sha256.Sum256(data)
		}
		result[rel] = struct {
			Mode fs.FileMode
			Hash [32]byte
		}{info.Mode(), hash}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
