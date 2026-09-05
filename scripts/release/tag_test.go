package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tagFixture(t *testing.T) string {
	t.Helper()
	t.Chdir(t.TempDir())
	for key, value := range map[string]string{"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull, "GIT_AUTHOR_NAME": "Fixture", "GIT_COMMITTER_NAME": "Fixture", "GIT_AUTHOR_EMAIL": "fixture@localhost", "GIT_COMMITTER_EMAIL": "fixture@localhost"} {
		t.Setenv(key, value)
	}
	mustGit(t, "init", "-q", "-b", "main")
	mustGit(t, "config", "core.hooksPath", os.DevNull)
	if err := os.MkdirAll("docs/releases", 0755); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"0.1.0", "1.0.0", "0.1.0-rc.1"} {
		if err := os.WriteFile(filepath.Join("docs/releases", version+".md"), []byte("Fixture release notes\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	mustGit(t, "add", ".")
	mustGit(t, "commit", "-qm", "fixture release")
	sha := mustGit(t, "rev-parse", "HEAD")
	mustGit(t, "update-ref", "refs/remotes/origin/main", sha)
	return sha
}

func mustGit(t *testing.T, args ...string) string {
	t.Helper()
	data, err := git(args...)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}

func TestReleaseTagAcceptsExactMainSource(t *testing.T) {
	for _, version := range []string{"0.1.0", "1.0.0", "0.1.0-rc.1"} {
		for _, annotated := range []bool{false, true} {
			t.Run(version+map[bool]string{false: "/lightweight", true: "/annotated"}[annotated], func(t *testing.T) {
				sha := tagFixture(t)
				tag := "v" + version
				if annotated {
					mustGit(t, "tag", "-a", tag, "-m", "fixture tag")
				} else {
					mustGit(t, "tag", tag)
				}
				event := mustGit(t, "rev-parse", "refs/tags/"+tag)
				source, err := checkTag(version, tag, event)
				if err != nil || source.Commit != sha || source.Version != version || source.Preview != (version != "1.0.0") {
					t.Fatalf("incorrect release source: %+v %v", source, err)
				}
			})
		}
	}
}

func TestReleaseTagRejectsUnsafeOrUnreviewableSource(t *testing.T) {
	for _, kind := range []string{"version-mismatch", "development", "malformed-tag", "numeric-prerelease", "bad-sha", "missing-tag", "wrong-tag-commit", "wrong-event", "outside-main", "dirty", "missing-notes", "empty-notes", "symlink-notes"} {
		t.Run(kind, func(t *testing.T) {
			sha := tagFixture(t)
			version, tag := "0.1.0", "v0.1.0"
			mustGit(t, "tag", tag)
			event := sha
			switch kind {
			case "version-mismatch":
				version = "1.0.0"
			case "development":
				version = "0.1.0-dev.1"
				tag = "v" + version
			case "malformed-tag":
				tag = "--help"
			case "numeric-prerelease":
				version = "0.1.0-01"
				tag = "v" + version
			case "bad-sha":
				event = "--help"
			case "missing-tag":
				mustGit(t, "tag", "-d", tag)
			case "dirty":
				if err := os.WriteFile("untracked", []byte("fixture"), 0644); err != nil {
					t.Fatal(err)
				}
			case "missing-notes", "empty-notes", "symlink-notes":
				if kind == "missing-notes" {
					mustGit(t, "rm", "docs/releases/0.1.0.md")
				} else if kind == "symlink-notes" {
					mustGit(t, "rm", "docs/releases/0.1.0.md")
					if err := os.Symlink("1.0.0.md", "docs/releases/0.1.0.md"); err != nil {
						t.Fatal(err)
					}
					mustGit(t, "add", ".")
				} else {
					if err := os.WriteFile("docs/releases/0.1.0.md", nil, 0644); err != nil {
						t.Fatal(err)
					}
					mustGit(t, "add", ".")
				}
				mustGit(t, "commit", "-qm", "fixture notes change")
				event = mustGit(t, "rev-parse", "HEAD")
				mustGit(t, "tag", "-f", tag)
				mustGit(t, "update-ref", "refs/remotes/origin/main", event)
			default:
				mustGit(t, "commit", "--allow-empty", "-qm", "fixture second commit")
				current := mustGit(t, "rev-parse", "HEAD")
				switch kind {
				case "wrong-tag-commit":
					event = current
					mustGit(t, "update-ref", "refs/remotes/origin/main", current)
				case "wrong-event":
					mustGit(t, "tag", "-f", tag)
					mustGit(t, "update-ref", "refs/remotes/origin/main", current)
				case "outside-main":
					event = current
					mustGit(t, "tag", "-f", tag)
				}
			}
			if _, err := checkTag(version, tag, event); err == nil {
				t.Fatal("accepted invalid release source")
			}
		})
	}
}
