package store

import (
	"encoding/hex"
	"strings"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
)

type HistoryEntry struct {
	Commit  string `json:"commit"`
	Time    string `json:"time"`
	Subject string `json:"subject"`
}

func (s *Store) History(name string) ([]HistoryEntry, error) {
	if err := s.Unlocked(); err != nil {
		return nil, err
	}
	enabled, err := s.GitEnabled()
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, fault.New("history.unavailable", "store has no Git history")
	}
	args := []string{"--literal-pathspecs", "log", "--format=%H%x00%cI%x00%s%x00"}
	if name != "" {
		if _, err := EntryPath(name); err != nil {
			return nil, err
		}
		args = append(args, "--", name+".age")
	}
	out, err := s.Git(args...)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(string(out), "\x00")
	entries := []HistoryEntry{}
	for len(parts) >= 3 {
		entries = append(entries, HistoryEntry{Commit: strings.TrimSpace(parts[0]), Time: parts[1], Subject: parts[2]})
		parts = parts[3:]
	}
	return entries, nil
}

func validCommit(ref string) bool {
	if len(ref) != 40 && len(ref) != 64 {
		return false
	}
	_, err := hex.DecodeString(ref)
	return err == nil
}

func (s *Store) HistoryShow(ref string) (map[string]any, error) {
	if !validCommit(ref) {
		return nil, fault.Usage("history requires a full commit hash from history list")
	}
	out, err := s.Git("ls-tree", "-r", "--name-only", "-z", ref)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, p := range strings.Split(string(out), "\x00") {
		if p == "" {
			continue
		}
		if strings.HasSuffix(p, ".age") {
			names = append(names, strings.TrimSuffix(p, ".age"))
		}
	}
	return map[string]any{"commit": ref, "names": names}, nil
}

func (s *Store) HistoryRestore(ref, name string) (MutationResult, error) {
	if !validCommit(ref) {
		return MutationResult{}, fault.Usage("history requires a full commit hash from history list")
	}
	if _, err := EntryPath(name); err != nil {
		return MutationResult{}, err
	}
	c, err := s.Git("show", ref+":"+name+".age")
	if err != nil {
		return MutationResult{}, err
	}
	ids, err := s.RecoveryKeys()
	if err != nil {
		return MutationResult{}, err
	}
	value, err := crypt.Decrypt(c, ids)
	if err != nil {
		return MutationResult{}, err
	}
	exists, err := s.Exists(name)
	if err != nil {
		return MutationResult{}, err
	}
	return s.Write(name, value, exists)
}
