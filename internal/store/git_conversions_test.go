package store

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agensfield/fulla/internal/securefs"
)

func TestGitFilterRefusedBeforeExecutionOrPublication(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "existing"}[existing], func(t *testing.T) {
			s := fixture(t, true)
			if existing {
				if _, err := s.Write("entry", []byte("original"), false); err != nil {
					t.Fatal(err)
				}
			}
			head, err := s.Head()
			if err != nil {
				t.Fatal(err)
			}
			backups, err := s.Backups()
			if err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(t.TempDir(), "filter-invoked")
			quoted := "'" + strings.ReplaceAll(marker, "'", "'\"'\"'") + "'"
			if _, err := s.Git("config", "filter.fixture.clean", "printf invoked > "+quoted+"; printf filtered"); err != nil {
				t.Fatal(err)
			}
			if err := securefs.WriteNew(s.Root, "passwords/.git/info/attributes", []byte("entry.age filter=fixture\n")); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Write("entry", []byte("replacement"), existing); err == nil {
				t.Fatal("accepted ciphertext conversion")
			}
			if _, err := os.Stat(marker); !errors.Is(err, fs.ErrNotExist) {
				t.Fatal("Git executed a clean filter", err)
			}
			if after, err := s.Head(); err != nil || after != head {
				t.Fatal("refused conversion changed Git history", err)
			}
			if after, err := s.Backups(); err != nil || len(after) != len(backups) {
				t.Fatal("refused conversion created backups", err)
			}
			if err := s.Unlocked(); err != nil {
				t.Fatal(err)
			}
			if existing {
				value, err := s.Read("entry")
				if err != nil || !bytes.Equal(value, []byte("original")) {
					t.Fatal("refused conversion changed live bytes", err)
				}
			} else if exists, err := s.Exists("entry"); err != nil || exists {
				t.Fatal("refused conversion created an entry", err)
			}
		})
	}
}

func TestGitConversionAttributePolicy(t *testing.T) {
	for _, rule := range []string{"text", "text=auto", "eol=crlf", "ident", "working-tree-encoding=UTF-16", "filter=unset", "filter=unspecified"} {
		t.Run(rule, func(t *testing.T) {
			s := fixture(t, true)
			if err := securefs.WriteNew(s.Root, "passwords/.git/info/attributes", []byte("*.age "+rule+"\n")); err != nil {
				t.Fatal(err)
			}
			if err := s.checkGitConversions([]string{"entry"}); err == nil {
				t.Fatal("accepted converting attribute")
			}
		})
	}
	s := fixture(t, true)
	if err := securefs.WriteNew(s.Root, "passwords/.git/info/attributes", []byte("*.age -text diff=age\n")); err != nil {
		t.Fatal(err)
	}
	if err := s.checkGitConversions([]string{"entry"}); err != nil {
		t.Fatal("rejected non-converting attributes", err)
	}
}
