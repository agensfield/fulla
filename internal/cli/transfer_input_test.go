package cli

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/agensfield/fulla/internal/fault"
)

func TestRecoveryPassphraseSourceSelection(t *testing.T) {
	for _, flags := range [][]string{
		{"--passphrase", "--json"},
		{"--passphrase", "--non-interactive"},
		{"--passphrase", "--passphrase-fd", "0"},
		{"--passphrase", "--recipient", "invalid"},
	} {
		p, err := parse(append([]string{"transfer", "export"}, flags...))
		if err != nil {
			t.Fatal(err)
		}
		_, err = exportRecipients(p)
		var problem *fault.Error
		if !errors.As(err, &problem) || (problem.Code != "interaction.required" && problem.Code != "invocation.invalid") {
			t.Fatal("source conflict did not fail before input", err)
		}
	}
	for _, flags := range [][]string{
		{"--passphrase", "--json"},
		{"--passphrase", "--non-interactive"},
		{"--passphrase", "--passphrase-fd", "0"},
		{"--passphrase", "--identity", "/not-read"},
	} {
		p, err := parse(append([]string{"transfer", "verify"}, flags...))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := transferIdentities(p, nil); err == nil {
			t.Fatal("accepted conflicting or machine-mode input")
		}
	}
	_, err := parse([]string{"transfer", "export", "--passphrase=private-sentinel"})
	if err == nil || strings.Contains(err.Error(), "private-sentinel") {
		t.Fatal("accepted or echoed argv passphrase", err)
	}
}

func TestSnapshotRestoreRejectsArchiveProtectionOptions(t *testing.T) {
	for _, flags := range [][]string{{"--identity", "/not-read"}, {"--passphrase"}, {"--passphrase-fd", "0"}} {
		app := App{In: strings.NewReader(""), Out: io.Discard, Err: io.Discard, Getenv: func(string) string { return "" }}
		if code := app.Main(append([]string{"backup", "restore", "snapshot"}, flags...)); code != 2 {
			t.Fatalf("ignored archive-only option: status=%d", code)
		}
	}
}
