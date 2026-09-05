package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDeterministicArchiveContainsExactMembers(t *testing.T) {
	files := []member{{"fulla", 0755, []byte{0, 255, 10}}, {"LICENSE", 0644, []byte("fixture license")}, {"SOURCE.json", 0644, []byte(`{"commit":"fixture"}`)}}
	first, err := archive(files)
	if err != nil {
		t.Fatal(err)
	}
	second, err := archive(files)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("archive was not deterministic", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if reader.Name != "" || reader.Comment != "" || reader.OS != 255 {
		t.Fatal("host metadata leaked into gzip")
	}
	tarReader := tar.NewReader(reader)
	for _, expected := range files {
		header, err := tarReader.Next()
		if err != nil {
			t.Fatal(err)
		}
		if header.Name != expected.name || header.Mode != expected.mode || header.Typeflag != tar.TypeReg || header.Uid != 0 || header.Gid != 0 || !header.ModTime.Equal(time.Unix(0, 0)) {
			t.Fatal("incorrect archive metadata")
		}
		content, err := io.ReadAll(tarReader)
		if err != nil || !bytes.Equal(content, expected.data) {
			t.Fatal("archive content changed", err)
		}
	}
	if _, err := tarReader.Next(); err != io.EOF {
		t.Fatal("unexpected archive member", err)
	}
}

func TestArtifactPublicationNeverReplacesExistingFile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "artifact")
	if err := writeNew(target, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := writeNew(target, []byte("replacement")); err == nil {
		t.Fatal("replaced existing artifact")
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "first" {
		t.Fatal("existing artifact changed", err)
	}
}

func TestReleaseVersionMustMatchSource(t *testing.T) {
	for _, version := range []string{"v0.1.0", "0.1.0\n", "0.1.0;echo", "999.0.0", "../other"} {
		target := filepath.Join(t.TempDir(), "out")
		if err := run(version, target); err == nil {
			t.Fatal("accepted incorrect release version")
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatal("invalid invocation created output")
		}
	}
}
