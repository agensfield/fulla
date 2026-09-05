package store

import (
	"bytes"
	"io"
	"path/filepath"
	"testing"
)

func TestScopedRecoveryAndNoOverwrite(t *testing.T) {
	source := fixture(t, false)
	target := fixture(t, false)
	for name, value := range map[string][]byte{"selected": {0, 255, 10}, "excluded": []byte("must-stay-out"), "shared": []byte("source")} {
		if _, err := source.Write(name, value, false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := target.Write("shared", []byte("destination"), false); err != nil {
		t.Fatal(err)
	}
	ids, rs, err := target.Keys()
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(filepath.Dir(source.Dir), "scoped.age")
	if _, err := source.ExportLogical([]string{"selected", "shared"}, rs, output, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := source.ExportLogical([]string{"absent"}, rs, output+"-absent", io.Discard); err == nil {
		t.Fatal("export accepted missing selector")
	}
	if _, err := source.ExportLogical([]string{"selected"}, rs, output, io.Discard); err == nil {
		t.Fatal("export overwrote artifact")
	}
	data, err := ReadArtifact(output, MaxBundleBytes)
	if err != nil {
		t.Fatal(err)
	}
	verification, err := VerifyLogical(data, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(verification.Names) != 2 {
		t.Fatal("scope leaked", verification)
	}
	result, err := target.ImportLogical(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skipped) != 1 || result.Skipped[0] != "shared" {
		t.Fatal(result)
	}
	value, err := target.Read("selected")
	if err != nil || !bytes.Equal(value, []byte{0, 255, 10}) {
		t.Fatal("lost exact bytes", err)
	}
	value, err = target.Read("shared")
	if err != nil || string(value) != "destination" {
		t.Fatal("overwrote shared value", err)
	}
	if exists, err := target.Exists("excluded"); err != nil || exists {
		t.Fatal("unselected export leaked", err)
	}
	data[len(data)-1] ^= 1
	if _, err := VerifyLogical(data, ids); err == nil {
		t.Fatal("accepted corrupt bundle")
	}
	if _, err := target.ImportLogical(data, nil); err == nil {
		t.Fatal("import accepted corrupt bundle")
	}
}
