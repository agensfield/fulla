package crypt

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"filippo.io/age"
	"github.com/agensfield/fulla/internal/fault"
)

type PluginStatus struct {
	Name       string `json:"name"`
	Executable string `json:"executable"`
	Available  bool   `json:"available"`
	Path       string `json:"path,omitempty"`
	Issue      string `json:"issue,omitempty"`
}

// Plugins inspects already parsed identities and recipients without invoking
// any plugin. Encoded identity data is never included in diagnostic results.
func Plugins(identities []age.Identity, recipients []age.Recipient) []PluginStatus {
	names := map[string]bool{}
	for _, identity := range identities {
		if name, ok := pluginName(identity); ok {
			names[name] = true
		}
	}
	for _, recipient := range recipients {
		if name, ok := pluginName(recipient); ok {
			names[name] = true
		}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	result := []PluginStatus{}
	for _, name := range ordered {
		status := PluginStatus{Name: name, Executable: "age-plugin-" + name}
		resolved, err := exec.LookPath(status.Executable)
		if err == nil {
			status.Path, err = filepath.Abs(resolved)
			status.Available = err == nil
		}
		if err != nil {
			status.Issue = "plugin.unavailable"
			if errors.Is(err, exec.ErrNotFound) {
				status.Issue = "plugin.missing"
			}
		}
		if status.Available && os.Getenv("AGEDEBUG") == "plugin" {
			status.Issue = "plugin.debug_forbidden"
		}
		result = append(result, status)
	}
	return result
}

func identityFailure(err error) error {
	var problem *fault.Error
	if errors.As(err, &problem) && (problem.Code == "interaction.required" || strings.HasPrefix(problem.Code, "plugin.") || strings.HasPrefix(problem.Code, "identity.unlock_")) {
		return problem
	}
	var lookup *exec.Error
	if !errors.As(err, &lookup) || !strings.HasPrefix(lookup.Name, "age-plugin-") {
		return nil
	}
	code := "plugin.unavailable"
	if errors.Is(lookup.Err, exec.ErrNotFound) {
		code = "plugin.missing"
	}
	result := fault.New(code, "required age plugin executable is unavailable; install it explicitly before retrying")
	result.Details["plugin"] = strings.TrimPrefix(lookup.Name, "age-plugin-")
	result.Details["executable"] = lookup.Name
	return result
}

func rejectPluginDebug(identities []age.Identity, recipients []age.Recipient) error {
	if os.Getenv("AGEDEBUG") != "plugin" {
		return nil
	}
	present := false
	for _, id := range identities {
		if _, ok := pluginName(id); ok {
			present = true
		}
	}
	for _, recipient := range recipients {
		if _, ok := pluginName(recipient); ok {
			present = true
		}
	}
	if present {
		return fault.New("plugin.debug_forbidden", "AGEDEBUG=plugin exposes key material; unset it before using age plugins")
	}
	return nil
}
