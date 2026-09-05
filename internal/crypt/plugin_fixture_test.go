package crypt

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	"filippo.io/age/plugin"
	"github.com/agensfield/fulla/internal/fault"
)

// The test executable doubles as a real plugin subprocess. Native age performs
// all key wrapping; this fixture adds only the external plugin protocol boundary.
func TestMain(m *testing.M) {
	if filepath.Base(os.Args[0]) != "age-plugin-fullafixture" {
		os.Exit(m.Run())
	}
	p, err := plugin.New("fullafixture")
	if err != nil {
		os.Exit(1)
	}
	p.HandleRecipient(func(data []byte) (age.Recipient, error) {
		if os.Getenv("FULLA_PLUGIN_FIXTURE_REQUEST") == "confirm" {
			yes, err := p.Confirm("fixture confirmation sentinel", "allow", "deny")
			if err != nil {
				return nil, err
			}
			if !yes {
				return nil, errors.New("fixture private error sentinel")
			}
		}
		return age.ParseX25519Recipient(string(data))
	})
	p.HandleIdentity(func(data []byte) (age.Identity, error) {
		if os.Getenv("FULLA_PLUGIN_FIXTURE_REQUEST") == "1" {
			pin, err := p.RequestValue("fixture private prompt sentinel", true)
			if err != nil {
				return nil, err
			}
			if pin != "fixture-pin" {
				return nil, errors.New("fixture private error sentinel")
			}
		}
		return age.ParseX25519Identity(string(data))
	})
	os.Exit(p.Main())
}

func TestRealPluginRoundTripAndInteractionRefusal(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(executable, filepath.Join(dir, "age-plugin-fullafixture")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("AGEDEBUG", "")
	t.Setenv("FULLA_PLUGIN_FIXTURE_REQUEST", "")
	native, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	private := []byte(plugin.EncodeIdentity("fullafixture", []byte(native.String())))
	public := []byte(plugin.EncodeRecipient("fullafixture", []byte(native.Recipient().String())))
	ids, err := Identities(private, nil)
	if err != nil {
		t.Fatal(err)
	}
	rs, err := Recipients(public, nil)
	if err != nil {
		t.Fatal(err)
	}
	value := []byte{0, 255, 10}
	ciphertext, err := Encrypt(value, rs)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Decrypt(ciphertext, ids)
	if err != nil || !bytes.Equal(plain, value) {
		t.Fatal("real plugin lost exact bytes", err)
	}
	if err := VerifyRecipient(rs, ids); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FULLA_PLUGIN_FIXTURE_REQUEST", "1")
	for _, operation := range []func() error{
		func() error { _, err := Decrypt(ciphertext, ids); return err },
		func() error { return VerifyRecipient(rs, ids) },
	} {
		var problem *fault.Error
		if err := operation(); !errors.As(err, &problem) || problem.Code != "interaction.required" {
			t.Fatal("plugin input request lost typed refusal", err)
		}
	}
	t.Setenv("FULLA_PLUGIN_FIXTURE_REQUEST", "confirm")
	var problem *fault.Error
	if _, err := Encrypt(value, rs); !errors.As(err, &problem) || problem.Code != "interaction.required" {
		t.Fatal("plugin confirmation lost typed refusal", err)
	}
	t.Setenv("FULLA_PLUGIN_FIXTURE_REQUEST", "")
	if err := VerifyRecipient(rs, ids); err != nil {
		t.Fatal("interaction refusal poisoned later operations", err)
	}
	t.Setenv("FULLA_PLUGIN_FIXTURE_REQUEST", "1")
	ui := &plugin.ClientUI{RequestValue: func(name, prompt string, secret bool) (string, error) {
		if name != "fullafixture" || !secret {
			t.Fatal("incorrect plugin request metadata")
		}
		return "fixture-pin", nil
	}}
	interactive, err := Identities(private, ui)
	if err != nil {
		t.Fatal(err)
	}
	plain, err = Decrypt(ciphertext, interactive)
	if err != nil || !bytes.Equal(plain, value) {
		t.Fatal("explicit plugin input failed", err)
	}
}

func TestPluginUIFailureRedactsCallbackError(t *testing.T) {
	ui, state := pluginUI(&plugin.ClientUI{RequestValue: func(string, string, bool) (string, error) {
		return "", errors.New("fixture private callback sentinel")
	}})
	_, _ = ui.RequestValue("fixture", "private prompt", true)
	if state.failure == nil || strings.Contains(state.failure.Error(), "sentinel") {
		t.Fatal("callback error was not redacted")
	}
}
