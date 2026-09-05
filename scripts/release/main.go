// Command release creates deterministic public distribution artifacts.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/agensfield/fulla/internal/cli"
)

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$`)

type member struct {
	name string
	mode int64
	data []byte
}

func main() {
	output := flag.String("output", "dist/release", "new output directory (must not exist)")
	version := flag.String("version", cli.Version, "version, matching the source CLI version")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "release takes no positional arguments")
		os.Exit(2)
	}
	if err := run(*version, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func git(args ...string) ([]byte, error) {
	c := exec.Command("git", args...)
	data, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s failed", args[0])
	}
	return data, nil
}

func run(version, output string) error {
	if !versionPattern.MatchString(version) || version != cli.Version {
		return errors.New("release version must match the semantic version in internal/cli/cli.go")
	}
	if output == "" {
		return errors.New("output directory is required")
	}
	root, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if filepath.Clean(cwd) != strings.TrimSpace(string(root)) {
		return errors.New("run release from the repository root")
	}
	dirty, err := git("status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return err
	}
	if len(dirty) != 0 {
		return errors.New("release requires a clean committed checkout")
	}
	commit, err := git("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	sha := strings.TrimSpace(string(commit))
	if len(sha) != 40 {
		return errors.New("unexpected source commit format")
	}
	if _, err := hex.DecodeString(sha); err != nil {
		return errors.New("invalid source commit")
	}
	source, err := git("archive", "--format=tar", "--prefix=fulla-"+version+"/", sha)
	if err != nil {
		return err
	}
	provenance, err := json.MarshalIndent(map[string]string{"version": version, "commit": sha, "module": "github.com/agensfield/fulla", "go": runtime.Version(), "source": "https://github.com/agensfield/fulla/tree/" + sha}, "", "  ")
	if err != nil {
		return err
	}
	provenance = append(provenance, '\n')
	// Refuse replacement. Failed builds leave inspectable partial artifacts and
	// never produce checksums.txt, which is written only after every build passes.
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	if err := os.Mkdir(output, 0755); err != nil {
		return fmt.Errorf("create fresh release directory: %w", err)
	}
	temp, err := os.MkdirTemp("", "fulla-release-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	if err := extractSource(temp, source); err != nil {
		return err
	}
	snapshot := filepath.Join(temp, "fulla-"+version)
	license, err := os.ReadFile(filepath.Join(snapshot, "LICENSE"))
	if err != nil {
		return err
	}
	notices, err := os.ReadFile(filepath.Join(snapshot, "THIRD_PARTY_NOTICES"))
	if err != nil {
		return err
	}
	readme, err := os.ReadFile(filepath.Join(snapshot, "README.md"))
	if err != nil {
		return err
	}
	checksums := map[string]string{}
	for _, target := range [][2]string{{"darwin", "amd64"}, {"darwin", "arm64"}, {"linux", "amd64"}, {"linux", "arm64"}} {
		executable := filepath.Join(temp, "fulla-"+target[0]+"-"+target[1])
		command := exec.Command("go", "build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-ldflags=-s -w -buildid=", "-o", executable, ".")
		command.Dir = snapshot
		command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+target[0], "GOARCH="+target[1], "GOFLAGS=", "GOWORK=off")
		command.Stdout, command.Stderr = os.Stdout, os.Stderr
		if err := command.Run(); err != nil {
			return fmt.Errorf("build %s/%s: %w", target[0], target[1], err)
		}
		binary, err := os.ReadFile(executable)
		if err != nil {
			return err
		}
		data, err := archive([]member{{"fulla", 0755, binary}, {"LICENSE", 0644, license}, {"THIRD_PARTY_NOTICES", 0644, notices}, {"README.md", 0644, readme}, {"SOURCE.json", 0644, provenance}})
		if err != nil {
			return err
		}
		name := fmt.Sprintf("fulla_%s_%s_%s.tar.gz", version, target[0], target[1])
		if err := writeArtifact(output, name, data, checksums); err != nil {
			return err
		}
	}
	compressed, err := compress(source)
	if err != nil {
		return err
	}
	if err := writeArtifact(output, "fulla_"+version+"_source.tar.gz", compressed, checksums); err != nil {
		return err
	}
	names := make([]string, 0, len(checksums))
	for name := range checksums {
		names = append(names, name)
	}
	sort.Strings(names)
	var sums strings.Builder
	for _, name := range names {
		fmt.Fprintf(&sums, "%s  %s\n", checksums[name], name)
	}
	return writeNew(filepath.Join(output, "checksums.txt"), []byte(sums.String()))
}

func archive(files []member) ([]byte, error) {
	var output bytes.Buffer
	writer := tar.NewWriter(&output)
	for _, file := range files {
		header := &tar.Header{Name: file.name, Mode: file.mode, Size: int64(len(file.data)), ModTime: time.Unix(0, 0).UTC(), Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}
		if err := writer.WriteHeader(header); err != nil {
			return nil, err
		}
		if _, err := writer.Write(file.data); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return compress(output.Bytes())
}

func compress(data []byte) ([]byte, error) {
	var output bytes.Buffer
	writer, err := gzip.NewWriterLevel(&output, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	writer.Header.ModTime = time.Unix(0, 0).UTC()
	writer.Header.OS = 255
	if _, err := writer.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func writeArtifact(directory, name string, data []byte, checksums map[string]string) error {
	if err := writeNew(filepath.Join(directory, name), data); err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	checksums[name] = hex.EncodeToString(sum[:])
	return nil
}

func writeNew(name string, data []byte) (err error) {
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer func() {
		if e := file.Close(); err == nil {
			err = e
		}
	}()
	_, err = file.Write(data)
	return err
}

// Only regular tracked files and directories are accepted into the immutable
// build snapshot. Rooted I/O and local-path validation prevent archive escape.
func extractSource(directory string, data []byte) error {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	reader := tar.NewReader(bytes.NewReader(data))
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		name := filepath.FromSlash(strings.TrimSuffix(header.Name, "/"))
		if !filepath.IsLocal(name) {
			return errors.New("source archive contains nonlocal path")
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(name, 0700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := root.MkdirAll(filepath.Dir(name), 0700); err != nil {
				return err
			}
			file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, os.FileMode(header.Mode)&0755)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, reader)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return errors.New("source archive contains unsupported link or special file")
		}
	}
}
