// Package cli owns Fulla's versioned machine envelope and command dispatch.
package cli

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/agensfield/fulla/internal/config"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/remote"
	"github.com/agensfield/fulla/internal/store"
)

var Version = "0.1.0-dev"

type App struct {
	In       io.Reader
	Out, Err io.Writer
	Getenv   func(string) string
}
type envelope struct {
	Schema   string       `json:"schema"`
	OK       bool         `json:"ok"`
	Command  string       `json:"command"`
	Data     any          `json:"data,omitempty"`
	Warnings []any        `json:"warnings,omitempty"`
	Error    *fault.Error `json:"error,omitempty"`
}

func (a *App) Main(args []string) (status int) {
	if a.In == nil {
		a.In = os.Stdin
	}
	if a.Out == nil {
		a.Out = os.Stdout
	}
	if a.Err == nil {
		a.Err = os.Stderr
	}
	if a.Getenv == nil {
		a.Getenv = os.Getenv
	}
	p, err := parse(args)
	jsonMode := p.has("json")
	// Even a parsing failure before --json must preserve the requested envelope.
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--json" {
			jsonMode = true
		}
	}
	defer func() {
		if value := recover(); value != nil {
			if debugPanics {
				panic(value)
			}
			status = a.failure(p.Command, jsonMode, fault.New("internal.failure", "unexpected internal failure"))
		}
	}()
	if err != nil {
		return a.failure(p.Command, jsonMode, err)
	}
	switch p.Command {
	case "history", "backup", "peer", "identity", "transfer", "remote":
		if len(p.Args) > 0 {
			sub := p.Args[0]
			switch sub {
			case "ls":
				sub = "list"
			case "rm":
				sub = "remove"
			case "info":
				sub = "show"
			case "recover":
				sub = "restore"
			}
			p.Command += " " + sub
			p.Args = p.Args[1:]
		}
	}
	if p.has("version") || p.Command == "version" {
		if jsonMode {
			return a.success("version", map[string]any{"version": Version}, true)
		}
		fmt.Fprintln(a.Out, Version)
		return 0
	}
	if p.has("help") || p.Command == "" {
		if jsonMode {
			return a.success(p.Command, map[string]any{"help": help}, true)
		}
		fmt.Fprintln(a.Out, help)
		return 0
	}
	data, raw, err := a.dispatch(p)
	if err != nil {
		return a.failure(p.Command, jsonMode, err)
	}
	if raw {
		return 0
	}
	return a.success(p.Command, data, jsonMode)
}

func (a *App) failure(command string, jsonMode bool, err error) int {
	var child childExit
	if errors.As(err, &child) {
		return int(child)
	}
	var e *fault.Error
	if !errors.As(err, &e) {
		e = fault.New("operation.failed", "operation could not complete")
	}
	if jsonMode {
		_ = json.NewEncoder(a.Out).Encode(envelope{Schema: "fulla.cli/v1", OK: false, Command: command, Error: e})
	} else {
		fmt.Fprintln(a.Err, e.Error())
		if e.Details["cleanup_required"] == true {
			if staging, ok := e.Details["staging_path"].(string); ok {
				fmt.Fprintf(a.Err, "  staging cleanup to verify: %q\n", staging)
			}
			if e.Details["lock_cleanup_required"] == true {
				fmt.Fprintln(a.Err, "  inspect the shared lock with fulla doctor before recovery")
			}
		}
		if report, ok := e.Details["report"].(store.DoctorResult); ok {
			for _, issue := range report.Issues {
				fmt.Fprintf(a.Err, "  issue: %q\n", issue)
			}
			for _, staging := range report.Staging {
				fmt.Fprintf(a.Err, "  staging to inspect (not deletion authority): %q\n", staging)
			}
			if report.Lock != nil {
				fmt.Fprintf(a.Err, "  lock owner: pid=%d local=%t alive=%t token=%q\n", report.Lock.PID, report.Lock.Local, report.Lock.Alive, report.Lock.Token)
			}
		}
	}
	return e.Status
}

type commandResult struct {
	data     any
	warnings []any
}

func (a *App) success(command string, data any, jsonMode bool) int {
	warnings := []any{}
	if result, ok := data.(commandResult); ok {
		data = result.data
		warnings = result.warnings
	}
	if jsonMode {
		// Success warnings are always present, including an empty array.
		err := json.NewEncoder(a.Out).Encode(map[string]any{"schema": "fulla.cli/v1", "ok": true, "command": command, "data": data, "warnings": warnings})
		if err != nil {
			return 3
		}
		return 0
	}
	for _, warning := range warnings {
		fmt.Fprintln(a.Err, warning)
	}
	if err := writeHumanResult(a.Out, command, data); err != nil {
		return 3
	}
	return 0
}

func (a *App) dispatch(p invocation) (any, bool, error) {
	if len(p.Tail) > 0 && p.Command != "run" && p.Command != "git" {
		return nil, false, fault.Usage("unexpected passthrough arguments")
	}
	switch p.Command {
	case "copy":
		if err := p.allow("clear-after", "no-clear"); err != nil {
			return nil, false, err
		}
	case "completion":
		return nil, true, a.completion(p)
	case "git":
		if err := p.allow(); err != nil {
			return nil, false, err
		}
		if p.has("json") {
			return nil, false, fault.Usage("git owns child streams and does not support --json")
		}
	case "remote serve":
		if err := p.allow(); err != nil {
			return nil, false, err
		}
		if p.has("json") {
			return nil, false, fault.Usage("remote serve owns protocol streams and does not support --json")
		}
	case "peer add", "peer rotate":
		if err := p.allow("host", "remote-store", "remote-binary", "expect-fingerprint", "ssh-option"); err != nil {
			return nil, false, err
		}
	case "peer list", "peer show", "peer remove":
		if err := p.allow(); err != nil {
			return nil, false, err
		}
	case "sync":
		if err := p.allow("dry-run", "fail-on-skip"); err != nil {
			return nil, false, err
		}
	case "identity show":
		if err := p.allow(); err != nil {
			return nil, false, err
		}
	case "identity rotate":
		if err := p.allow("destroy-retired-key", "acknowledge", "compromise"); err != nil {
			return nil, false, err
		}
	case "backup export":
		if err := p.allow("full", "recipient", "passphrase-fd", "passphrase", "output"); err != nil {
			return nil, false, err
		}
	case "transfer export":
		if err := p.allow("recipient", "output", "manifest", "passphrase-fd", "passphrase"); err != nil {
			return nil, false, err
		}
		if p.value("output") == "-" && p.has("json") {
			return nil, false, fault.Usage("binary export to stdout does not support --json")
		}
	case "transfer verify", "transfer import":
		if err := p.allow("identity", "passphrase-fd", "passphrase"); err != nil {
			return nil, false, err
		}
	case "history list", "history show", "history restore", "backup list", "backup show":
		if err := p.allow(); err != nil {
			return nil, false, err
		}
	case "backup prune":
		if err := p.allow("keep", "older-than", "dry-run"); err != nil {
			return nil, false, err
		}
	case "backup restore":
		if err := p.allow("phase", "full", "identity", "passphrase-fd", "passphrase"); err != nil {
			return nil, false, err
		}
		if !p.has("full") && (p.has("identity") || p.has("passphrase") || p.has("passphrase-fd")) {
			return nil, false, fault.Usage("recovery identity and passphrase options require --full")
		}
	case "doctor":
		if err := p.allow("deep", "recover-lock", "report", "fix-permissions"); err != nil {
			return nil, false, err
		}
	case "run":
		if err := p.allow("env", "clean-env", "inherit"); err != nil {
			return nil, false, err
		}
		if p.has("json") {
			return nil, false, fault.Usage("run owns child streams and does not support --json")
		}
	case "init":
		if err := p.allow("no-git", "adopt", "dry-run"); err != nil {
			return nil, false, err
		}
	case "add", "edit":
		if err := p.allow("stdin", "from-fd", "generate", "length", "alphabet"); err != nil {
			return nil, false, err
		}
	case "show", "list", "move", "status":
		if err := p.allow(); err != nil {
			return nil, false, err
		}
	case "remove":
		if err := p.allow("permanent-delete"); err != nil {
			return nil, false, err
		}
	default:
		return nil, false, fault.Usage("unknown or not yet implemented command")
	}
	c, err := config.Resolve(config.Flags{Store: p.value("store"), Config: p.value("config")}, a.Getenv)
	if err != nil {
		return nil, false, err
	}
	if p.Command == "transfer verify" {
		r, e := a.transfer(p, nil)
		return r, false, e
	}
	if p.Command == "backup restore" && p.has("full") {
		if len(p.Args) != 1 || p.has("phase") {
			return nil, false, fault.Usage("full restore requires one archive path and no snapshot phase")
		}
		if !p.has("yes") && (p.has("json") || p.has("non-interactive")) {
			return nil, false, fault.Interaction("full restore requires --yes and an empty target; it clones identity and peer authority")
		}
		ids, e := transferIdentities(p, nil)
		if e != nil {
			return nil, false, e
		}
		data, e := store.ReadArtifact(p.Args[0], store.MaxBundleBytes)
		if e != nil {
			return nil, false, e
		}
		r, e := store.RestoreFullConfirmed(data, ids, c.StorePath, commandIdentityUI(p), func(plan store.ArchiveResult) error {
			return a.confirm(p, fmt.Sprintf("Restore %d files (%d bytes) into %q.\nThis clones the original identity and peer authority. Use it to replace a lost machine, not to enroll another live peer.\nPublish restored store? [y/N]: ", plan.Files, plan.Bytes, plan.Path))
		})
		return r, false, e
	}
	if p.Command == "init" {
		r, e := a.initialize(p, c)
		return r, false, e
	}
	if p.Command == "doctor" {
		r, e := a.doctor(p, c)
		return r, false, e
	}
	s, err := store.Open(c.StorePath, true, commandIdentityUI(p))
	if err != nil {
		return nil, false, err
	}
	defer s.Close()
	if err := s.Unlocked(); err != nil {
		return nil, false, err
	}
	if p.Command == "copy" {
		r, e := a.copy(p, c, s)
		return r, false, e
	}
	if p.Command == "git" {
		return nil, true, a.git(p, s)
	}
	if p.Command == "remote serve" {
		if len(p.Args) != 0 {
			return nil, false, fault.Usage("remote serve takes no positional arguments")
		}
		return nil, true, remote.Serve(s, a.In, a.Out)
	}
	if p.Command == "sync" || p.Command == "peer add" || p.Command == "peer rotate" || p.Command == "peer list" || p.Command == "peer show" || p.Command == "peer remove" {
		r, e := a.peers(p, s)
		return r, false, e
	}
	if p.Command == "identity show" || p.Command == "identity rotate" {
		if len(p.Args) != 0 {
			return nil, false, fault.Usage(p.Command + " takes no positional arguments")
		}
		if p.Command == "identity show" {
			r, e := s.IdentityShow()
			return r, false, e
		}
		if !p.has("yes") {
			return nil, false, fault.Interaction("identity rotation requires --yes; peers will require explicit trust rotation")
		}
		r, e := s.Rotate(p.has("destroy-retired-key"), p.value("acknowledge"), p.has("compromise"))
		return r, false, e
	}
	if p.Command == "backup export" {
		if !p.has("full") || len(p.Args) != 0 || p.value("output") == "" || p.value("output") == "-" {
			return nil, false, fault.Usage("backup export requires --full and --output PATH")
		}
		if err := s.CheckArtifactPath(p.value("output")); err != nil {
			return nil, false, err
		}
		if err := s.CheckLockCleanup(); err != nil {
			return nil, false, err
		}
		rs, e := exportRecipients(p)
		if e != nil {
			return nil, false, e
		}
		r, e := s.ExportFull(rs, p.value("output"))
		return r, false, e
	}
	if p.Command == "transfer export" || p.Command == "transfer import" {
		r, e := a.transfer(p, s)
		return r, p.Command == "transfer export" && p.value("output") == "-", e
	}
	if p.Command == "history list" || p.Command == "history show" || p.Command == "history restore" || p.Command == "backup list" || p.Command == "backup show" || p.Command == "backup restore" || p.Command == "backup prune" {
		r, e := a.recovery(p, s)
		return r, false, e
	}
	if p.Command == "run" {
		return nil, true, a.run(p, c, s)
	}
	switch p.Command {
	case "list", "status":
		if len(p.Args) != 0 {
			return nil, false, fault.Usage(p.Command + " takes no positional arguments")
		}
		names, err := s.Names()
		if err != nil {
			return nil, false, err
		}
		if p.Command == "list" {
			return map[string]any{"names": names}, false, nil
		}
		backups, err := s.BackupSummary()
		if err != nil {
			return nil, false, err
		}
		return map[string]any{"backups": backups, "store_path": c.StorePath, "config_path": c.ConfigPath, "sources": c.Sources, "profile": "pa-v1", "entries": len(names)}, false, nil
	case "show":
		if len(p.Args) != 1 {
			return nil, false, fault.Usage("show requires one entry name")
		}
		value, err := s.Read(p.Args[0])
		if err != nil {
			return nil, false, err
		}
		if p.has("json") {
			return map[string]any{"name": p.Args[0], "encoding": "base64", "value": base64.StdEncoding.EncodeToString(value)}, false, nil
		}
		_, err = a.Out.Write(value)
		return nil, true, err
	case "add", "edit":
		if len(p.Args) != 1 {
			return nil, false, fault.Usage(p.Command + " requires one entry name")
		}
		if err := s.CheckWrite(p.Args[0], p.Command == "edit"); err != nil {
			return nil, false, err
		}
		if !p.has("stdin") && !p.has("from-fd") && !p.has("generate") {
			if p.has("json") || p.has("non-interactive") {
				return nil, false, fault.Interaction("provide --stdin, --from-fd N, or --generate")
			}
			if p.has("length") || p.has("alphabet") {
				return nil, false, fault.Usage("generation settings require --generate")
			}
			r, err := s.WriteInteractive(p.Args[0], p.Command == "edit", func(original []byte) ([]byte, error) {
				return a.interactiveInput(p, c, original)
			})
			return r, false, err
		}
		value, err := a.input(p, c)
		if err != nil {
			return nil, false, err
		}
		r, err := s.Write(p.Args[0], value, p.Command == "edit")
		return r, false, err
	case "remove":
		if len(p.Args) != 1 {
			return nil, false, fault.Usage("remove requires one entry name")
		}
		r, err := s.Remove(p.Args[0], p.has("permanent-delete"))
		return r, false, err
	case "move":
		if len(p.Args) != 2 {
			return nil, false, fault.Usage("move requires source and destination names")
		}
		r, err := s.Move(p.Args[0], p.Args[1])
		return r, false, err
	}
	return nil, false, fault.Usage("unknown command")
}
