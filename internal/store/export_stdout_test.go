package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/agensfield/fulla/internal/fault"
)

type exportFixtureWriter struct {
	mode     string
	accepted []byte
	calls    int
}

func (w *exportFixtureWriter) Write(data []byte) (int, error) {
	w.calls++
	n := len(data)
	switch w.mode {
	case "zero-error", "zero-short":
		n = 0
	case "partial-error", "partial-short":
		n /= 2
	}
	w.accepted = append(w.accepted, data[:n]...)
	if strings.HasSuffix(w.mode, "error") {
		return n, errors.New("private writer diagnostic sentinel")
	}
	return n, nil
}

func TestExportStdoutAccountsForPartialWrites(t *testing.T) {
	for _, git := range []bool{false, true} {
		for _, mode := range []string{"success", "zero-error", "zero-short", "partial-error", "partial-short", "full-error"} {
			t.Run(fmt.Sprintf("git=%t/%s", git, mode), func(t *testing.T) {
				s := fixture(t, git)
				if _, err := s.Write("entry", []byte{0, 255, 10, 42}, false); err != nil {
					t.Fatal(err)
				}
				before := archiveTree(t, s, false)
				identity, err := age.GenerateX25519Identity()
				if err != nil {
					t.Fatal(err)
				}
				writer := &exportFixtureWriter{mode: mode}
				result, err := s.ExportLogical(nil, []age.Recipient{identity.Recipient()}, "-", writer)
				if writer.calls != 1 {
					t.Fatal("export retried partial stream", writer.calls)
				}
				after := archiveTree(t, s, false)
				if mode == "success" {
					if err != nil || result.Receipt == "" {
						t.Fatal("successful stream failed", err)
					}
					verified, err := VerifyLogical(writer.accepted, []age.Identity{identity})
					if err != nil || !reflect.DeepEqual(verified.Names, []string{"entry"}) {
						t.Fatal("invalid completed stream", verified, err)
					}
					receiptBytes, err := os.ReadFile(filepath.Join(s.Dir, result.Receipt))
					if err != nil {
						t.Fatal(err)
					}
					var receipt struct {
						Command string
						Applied bool
					}
					if err := json.Unmarshal(receiptBytes, &receipt); err != nil || receipt.Command != "transfer export" || !receipt.Applied {
						t.Fatal("invalid success receipt", err)
					}
					if _, exists := before[result.Receipt]; exists {
						t.Fatal("reused receipt")
					}
					delete(after, result.Receipt)
				} else {
					var problem *fault.Error
					if !errors.As(err, &problem) {
						t.Fatal("missing typed stream failure", err)
					}
					wantStatus := 1
					if len(writer.accepted) > 0 {
						wantStatus = 3
					}
					if problem.Status != wantStatus || problem.Details["bytes_written"] != len(writer.accepted) || problem.Details["output_complete"] != (mode == "full-error") {
						t.Fatal("incorrect output accounting", problem)
					}
					if (problem.Details["applied"] == true) != (len(writer.accepted) > 0) {
						t.Fatal("incorrect applied state", problem)
					}
					if strings.Contains(problem.Error(), "sentinel") || result.Receipt != "" {
						t.Fatal("leaked error or invented success receipt", problem)
					}
					if mode == "full-error" {
						if _, err := VerifyLogical(writer.accepted, []age.Identity{identity}); err != nil {
							t.Fatal("full accepted stream invalid", err)
						}
					} else if _, err := VerifyLogical(writer.accepted, []age.Identity{identity}); err == nil {
						t.Fatal("incomplete stream verified")
					}
				}
				if !reflect.DeepEqual(before, after) {
					t.Fatal("stream result changed source store")
				}
			})
		}
	}
}
