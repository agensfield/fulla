package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

func TestExportPublicationPreservesAppliedState(t *testing.T) {
	for _, git := range []bool{false, true} {
		for _, published := range []bool{false, true} {
			t.Run(fmt.Sprintf("git=%t/published=%t", git, published), func(t *testing.T) {
				s := fixture(t, git)
				before := archiveTree(t, s, false)
				parent, err := filepath.EvalSymlinks(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				output := filepath.Join(parent, "archive.age")
				data := []byte{0, 255, 10, 42}
				called := false
				err = s.publishArtifact(output, data, func(root *os.Root, name string, value []byte) (bool, error) {
					called = true
					if published {
						ok, err := securefs.PublishNewPublished(root, name, value)
						if err != nil || !ok {
							t.Fatal("fixture publication failed", ok, err)
						}
					}
					// Model the filesystem outcome at the store boundary. The securefs test
					// separately injects failure at its actual post-rename sync callback.
					return published, errors.New("private filesystem diagnostic sentinel")
				})
				var problem *fault.Error
				if !called || !errors.As(err, &problem) {
					t.Fatal("missing typed failure", err)
				}
				if strings.Contains(err.Error(), "sentinel") {
					t.Fatal("leaked underlying error")
				}
				if published {
					if problem.Status != 3 || problem.Details["applied"] != true || problem.Details["durability_confirmed"] != false {
						t.Fatal("lost publication evidence", problem)
					}
					got, err := os.ReadFile(output)
					if err != nil || !bytes.Equal(got, data) {
						t.Fatal("lost published bytes", err)
					}
					info, err := os.Stat(output)
					if err != nil || info.Mode().Perm() != 0600 {
						t.Fatal("unsafe artifact", err)
					}
					if err := s.PublishArtifact(output, []byte("replacement")); err == nil {
						t.Fatal("retry overwrote artifact")
					}
					got, err = os.ReadFile(output)
					if err != nil || !bytes.Equal(got, data) {
						t.Fatal("retry changed artifact", err)
					}
				} else {
					if problem.Status != 1 || problem.Code != "export.publish_failed" || problem.Details["applied"] == true {
						t.Fatal("invented publication", problem)
					}
					if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
						t.Fatal("unpublished artifact exists", err)
					}
				}
				if !reflect.DeepEqual(before, archiveTree(t, s, false)) {
					t.Fatal("artifact publication changed store")
				}
			})
		}
	}
}
