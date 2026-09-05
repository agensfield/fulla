package cli

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/agensfield/fulla/internal/config"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/store"
)

// diagnosticReport deliberately does not serialize DoctorResult wholesale.
// Paths, names, fingerprints, plugin paths, and lock tokens are not needed in
// a shareable diagnostic artifact. Never attach arbitrary error details here.
type diagnosticReport struct {
	Schema          string   `json:"schema"`
	Version         string   `json:"version"`
	Healthy         bool     `json:"healthy"`
	Complete        bool     `json:"complete"`
	Deep            bool     `json:"deep"`
	Entries         int      `json:"entries"`
	Git             bool     `json:"git"`
	Locked          bool     `json:"locked"`
	IdentityValid   bool     `json:"identity_valid"`
	RecipientsValid bool     `json:"recipients_valid"`
	Peers           int      `json:"peers"`
	Issues          []string `json:"issues"`
	Error           string   `json:"error,omitempty"`
}

func (a *App) doctor(p invocation, c *config.Resolved) (any, error) {
	if len(p.Args) != 0 {
		return nil, fault.Usage("doctor takes no positional arguments")
	}
	if p.has("recover-lock") {
		if p.has("report") || p.has("deep") {
			return nil, fault.Usage("lock recovery cannot be combined with diagnostic inspection")
		}
		s, err := store.Open(c.StorePath, true, nil)
		if err != nil {
			return nil, err
		}
		defer s.Close()
		return s.Recover(p.value("recover-lock"))
	}
	// Artifact path validation/publication depends only on the selected store
	// directory, allowing reports even when the store itself cannot be opened.
	artifacts := &store.Store{Dir: c.StorePath}
	if p.has("report") {
		if p.value("report") == "-" {
			return nil, fault.Usage("diagnostic report requires a private file path")
		}
		// Reject occupied or unsafe destinations before deep inspection can invoke
		// an interactive identity or decrypt any entries.
		if err := artifacts.CheckArtifactPath(p.value("report")); err != nil {
			return nil, fault.New("doctor.report_path", "report requires a new file outside the store with an existing nonsymlink parent")
		}
	}
	r := store.DoctorResult{Deep: p.has("deep"), Issues: []string{}}
	s, err := store.Open(c.StorePath, true, nil)
	if err == nil {
		defer s.Close()
		r, err = s.Doctor(p.has("deep"))
	}
	r.StorePath, r.ConfigPath, r.Sources = c.StorePath, c.ConfigPath, c.Sources
	if p.has("report") {
		report := diagnosticReport{Schema: "fulla.diagnostic/v1", Version: Version, Healthy: r.Healthy && err == nil, Complete: err == nil, Deep: r.Deep, Entries: r.Entries, Git: r.Git, Locked: r.Lock != nil, IdentityValid: r.IdentityValid, RecipientsValid: r.RecipientsValid, Peers: r.Peers, Issues: []string{}}
		seen := map[string]bool{}
		for _, issue := range r.Issues {
			code, _, _ := strings.Cut(issue, ":")
			if !seen[code] {
				report.Issues = append(report.Issues, code)
				seen[code] = true
			}
		}
		if err != nil {
			report.Error = "internal.failure"
			var problem *fault.Error
			if errors.As(err, &problem) {
				report.Error = problem.Code
			}
		}
		data, marshalErr := json.MarshalIndent(report, "", "  ")
		if marshalErr != nil {
			return nil, fault.New("doctor.report_failed", "could not encode diagnostic report")
		}
		if writeErr := artifacts.PublishArtifact(p.value("report"), append(data, '\n')); writeErr != nil {
			return nil, fault.New("doctor.report_failed", "could not publish private diagnostic report without replacement")
		}
	}
	if err == nil && !r.Healthy {
		problem := fault.New("doctor.unhealthy", "store requires attention; inspect the diagnostic report")
		problem.Details["report"] = r
		return nil, problem
	}
	return r, err
}
