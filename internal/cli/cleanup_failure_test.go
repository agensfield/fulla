package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agensfield/fulla/internal/fault"
)

func TestCleanupFailureProvidesEscapedHumanAndStructuredMachineGuidance(t *testing.T) {
	for _, machine := range []bool{false, true} {
		var stdout, stderr bytes.Buffer
		app := App{Out: &stdout, Err: &stderr}
		failure := fault.New("transaction.cleanup_failed", "could not confirm cleanup of an unpublished operation")
		failure.Details["cleanup_required"] = true
		failure.Details["staging_path"] = "/fixture/new\nline/\x1b[31m"
		failure.Details["lock_cleanup_required"] = true
		if app.failure("identity rotate", machine, failure) != 1 {
			t.Fatal("wrong cleanup status")
		}
		if machine {
			var result envelope
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.Error == nil || result.Error.Details["staging_path"] != failure.Details["staging_path"] || stderr.Len() != 0 {
				t.Fatal("invalid structured cleanup result", err)
			}
		} else {
			if stdout.Len() != 0 || !strings.Contains(stderr.String(), `staging cleanup to verify: "/fixture/new\nline/\x1b[31m"`) || !strings.Contains(stderr.String(), "inspect the shared lock with fulla doctor") || bytes.Contains(stderr.Bytes(), []byte{0x1b}) {
				t.Fatal("missing or unescaped human cleanup guidance")
			}
		}
	}
}
