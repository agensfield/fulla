package main

import (
	"encoding/hex"
	"errors"
	"os"
	"strings"
)

type releaseSource struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Preview bool   `json:"preview"`
	Notes   string `json:"notes"`
}

// checkTag is read-only. Tag creation remains the deliberate publication step
// after the full-spec acceptance audit, not a side effect of this checker.
func checkTag(version, tag, eventSHA string) (releaseSource, error) {
	var result releaseSource
	if !versionPattern.MatchString(version) || tag != "v"+version {
		return result, errors.New("release tag must match the source version")
	}
	if _, suffix, ok := strings.Cut(version, "-"); ok {
		for _, part := range strings.Split(suffix, ".") {
			if part == "dev" || strings.HasPrefix(part, "dev-") {
				return result, errors.New("development versions cannot be published")
			}
			numeric := true
			for _, ch := range part {
				if ch < '0' || ch > '9' {
					numeric = false
				}
			}
			if numeric && len(part) > 1 && part[0] == '0' {
				return result, errors.New("numeric prerelease identifiers cannot have leading zeros")
			}
		}
	}
	if len(eventSHA) != 40 {
		return result, errors.New("release event must identify an exact Git object")
	}
	if _, err := hex.DecodeString(eventSHA); err != nil {
		return result, errors.New("invalid release event SHA")
	}
	read := func(args ...string) (string, error) {
		value, err := git(args...)
		return strings.TrimSpace(string(value)), err
	}
	head, err := read("rev-parse", "HEAD")
	if err != nil {
		return result, err
	}
	event, err := read("rev-parse", eventSHA+"^{commit}")
	if err != nil || event != head {
		return result, errors.New("checkout does not match release event")
	}
	tagged, err := read("rev-parse", "refs/tags/"+tag+"^{commit}")
	if err != nil || tagged != head {
		return result, errors.New("tag does not identify checked-out source")
	}
	if _, err := git("merge-base", "--is-ancestor", head, "refs/remotes/origin/main"); err != nil {
		return result, errors.New("release commit is not in origin/main")
	}
	dirty, err := read("status", "--porcelain", "--untracked-files=normal")
	if err != nil || dirty != "" {
		return result, errors.New("release verification requires a clean checkout")
	}
	notes := "docs/releases/" + version + ".md"
	if _, err := git("ls-files", "--error-unmatch", "--", notes); err != nil {
		return result, errors.New("release requires tracked version-specific notes")
	}
	info, err := os.Lstat(notes)
	if err != nil || !info.Mode().IsRegular() {
		return result, errors.New("release notes must be a regular source file")
	}
	data, err := os.ReadFile(notes)
	if err != nil || len(strings.TrimSpace(string(data))) == 0 {
		return result, errors.New("release notes must not be empty")
	}
	return releaseSource{version, head, strings.HasPrefix(version, "0.") || strings.Contains(version, "-"), notes}, nil
}
