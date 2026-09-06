package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"

	"filippo.io/age"
	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/store"
)

func passphraseFD(value string) (string, error) {
	fd, err := strconv.Atoi(value)
	if err != nil || fd < 0 || fd == 1 || fd == 2 {
		return "", fault.Usage("passphrase-fd requires an inherited readable descriptor")
	}
	f := os.NewFile(uintptr(fd), "fulla-passphrase")
	if f == nil {
		return "", fault.Usage("invalid passphrase descriptor")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 4097))
	defer clear(b)
	if err != nil {
		return "", fault.New("input.failed", "could not read passphrase descriptor")
	}
	if len(b) < 20 || len(b) > 4096 {
		return "", fault.New("recovery.weak_passphrase", "recovery passphrase must contain 20 to 4096 bytes")
	}
	return string(b), nil
}

func transferIdentities(p invocation, s *store.Store) ([]age.Identity, error) {
	if p.has("identity") && (p.has("passphrase-fd") || p.has("passphrase")) {
		return nil, fault.Usage("select one recovery identity or passphrase source")
	}
	if p.has("passphrase-fd") || p.has("passphrase") {
		value, err := recoveryPassphrase(p, false)
		if err != nil {
			return nil, err
		}
		id, err := age.NewScryptIdentity(value)
		if err != nil {
			return nil, fault.New("identity.invalid", "could not initialize passphrase identity")
		}
		return []age.Identity{id}, nil
	}
	if p.has("identity") {
		data, err := store.ReadArtifact(p.value("identity"), 4<<20)
		if err != nil {
			return nil, err
		}
		return crypt.Identities(data, commandIdentityUI(p))
	}
	if s != nil {
		ids, _, err := s.Keys()
		return ids, err
	}
	return nil, fault.Interaction("isolated verification requires --identity PATH or --passphrase / --passphrase-fd N")
}

func (a *App) transfer(p invocation, s *store.Store) (any, error) {
	if p.Command == "transfer export" {
		if len(p.Args) != 0 || !p.has("output") {
			return nil, fault.Usage("transfer export requires --output PATH and an explicit recipient or passphrase")
		}
		// Reject impossible artifact publication and malformed selection before
		// consuming a recovery passphrase or opening terminal input.
		if p.value("output") != "-" {
			if err := s.CheckArtifactPath(p.value("output")); err != nil {
				return nil, err
			}
		}
		var names []string
		if p.has("manifest") {
			data, err := store.ReadArtifact(p.value("manifest"), 4<<20)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(data, &names); err != nil || len(names) == 0 {
				return nil, fault.Usage("manifest must be a nonempty JSON array of exact entry names")
			}
		}
		recipients, err := exportRecipients(p)
		if err != nil {
			return nil, err
		}
		return s.ExportLogical(names, recipients, p.value("output"), a.Out)
	}
	if len(p.Args) != 1 {
		return nil, fault.Usage(p.Command + " requires one encrypted bundle path")
	}
	ids, err := transferIdentities(p, s)
	if err != nil {
		return nil, err
	}
	ciphertext, err := store.ReadArtifact(p.Args[0], store.MaxBundleBytes)
	if err != nil {
		return nil, err
	}
	if p.Command == "transfer verify" {
		return store.VerifyLogical(ciphertext, ids)
	}
	return s.ImportLogical(ciphertext, ids)
}

func exportRecipients(p invocation) ([]age.Recipient, error) {
	if p.has("recipient") && (p.has("passphrase-fd") || p.has("passphrase")) {
		return nil, fault.Usage("choose recipient protection or passphrase protection")
	}
	if p.has("passphrase-fd") || p.has("passphrase") {
		value, err := recoveryPassphrase(p, true)
		if err != nil {
			return nil, err
		}
		r, err := age.NewScryptRecipient(value)
		if err != nil {
			return nil, fault.New("recovery.invalid_passphrase", "could not initialize passphrase protection")
		}
		return []age.Recipient{r}, nil
	}
	if !p.has("recipient") {
		return nil, fault.Interaction("export requires --recipient, --passphrase, or --passphrase-fd")
	}
	return crypt.Recipients([]byte(strings.Join(p.Flags["recipient"], "\n")), commandIdentityUI(p))
}

// --passphrase selects terminal input; it never takes a secret argument.
func recoveryPassphrase(p invocation, confirm bool) (string, error) {
	if p.has("passphrase") && p.has("passphrase-fd") {
		return "", fault.Usage("select one passphrase input source")
	}
	if p.has("passphrase-fd") {
		return passphraseFD(p.value("passphrase-fd"))
	}
	if p.has("json") || p.has("non-interactive") {
		return "", fault.Interaction("terminal passphrase input is unavailable in machine mode; use --passphrase-fd")
	}
	value, err := withTerminal("recovery passphrase requires a controlling terminal", func(ctx context.Context, tty *os.File) ([]byte, error) {
		first, err := terminalLine(ctx, tty, "recovery passphrase (20-4096 bytes, hidden): ")
		if err != nil {
			clear(first)
			return nil, err
		}
		if len(first) < 20 || len(first) > 4096 {
			clear(first)
			return nil, fault.New("recovery.weak_passphrase", "recovery passphrase must contain 20 to 4096 bytes")
		}
		if confirm {
			second, err := terminalLine(ctx, tty, "confirm recovery passphrase (hidden): ")
			defer clear(second)
			if err != nil {
				clear(first)
				return nil, err
			}
			if !bytes.Equal(first, second) {
				clear(first)
				return nil, fault.New("recovery.passphrase_mismatch", "recovery passphrases did not match")
			}
		}
		return first, nil
	})
	defer clear(value)
	return string(value), err
}
