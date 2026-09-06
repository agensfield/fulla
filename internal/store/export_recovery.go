package store

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

const exportProtocol = "export-v1"

type exportPlan struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	Output  string `json:"output"`
	Digest  string `json:"digest"`
	Size    int    `json:"size"`
}

type exportReceipt struct {
	Version int        `json:"version"`
	Command string     `json:"command"`
	Names   []string   `json:"names,omitempty"`
	Entries int        `json:"entries,omitempty"`
	Files   int        `json:"files,omitempty"`
	Full    bool       `json:"full,omitempty"`
	At      string     `json:"at"`
	Applied bool       `json:"applied"`
	Phase   string     `json:"phase"`
	Export  exportPlan `json:"export"`
}

func exportStage(id string) string       { return ".fulla-export-" + id }
func exportReceiptPath(id string) string { return metadata + "/receipts/" + id + ".json" }

// The receipt is inert until bound to this lock. No external staging exists
// before the binding is durable. Final receipts retain their recovery provenance.
func (s *Store) prepareExport(lock *Lock, output string, data []byte, receipt exportReceipt) (exportReceipt, error) {
	if err := s.RequireDomain("transactions"); err != nil {
		return receipt, err
	}
	if err := s.CheckArtifactPath(output); err != nil {
		return receipt, err
	}
	abs, err := securefs.Canonical(output, true)
	if err != nil {
		return receipt, err
	}
	receipt.Export = exportPlan{Version: 1, ID: securefs.ID(), Output: abs, Digest: digest(data), Size: len(data)}
	if _, err := os.Lstat(filepath.Join(filepath.Dir(abs), exportStage(receipt.Export.ID))); !errors.Is(err, fs.ErrNotExist) {
		return receipt, fault.New("export.staging_exists", "export staging destination unavailable")
	}
	receipt.Phase = "prepared"
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return receipt, err
	}
	if len(encoded) > maxMetadata {
		return receipt, fault.New("export.too_large", "export recovery metadata exceeds limit")
	}
	if err := securefs.PublishNew(s.Root, exportReceiptPath(receipt.Export.ID), encoded); err != nil {
		return receipt, err
	}
	info, err := securefs.Read(s.Root, "lock/info", 4096)
	if err != nil {
		return receipt, err
	}
	bound, err := securefs.ReplacePublished(s.Root, "lock/info", []byte(strings.TrimSpace(string(info))+" export_receipt="+receipt.Export.ID+" stage_protocol="+exportProtocol+"\n"))
	if bound {
		lock.ExportReceipt = receipt.Export.ID
	}
	return receipt, err
}

func (s *Store) publishExport(receipt exportReceipt, data []byte, hook func(string) error) error {
	if digest(data) != receipt.Export.Digest || len(data) != receipt.Export.Size {
		return fault.New("export.recovery_mismatch", "export bytes differ from prepared receipt")
	}
	parent, err := os.OpenRoot(filepath.Dir(receipt.Export.Output))
	if err != nil {
		return err
	}
	defer parent.Close()
	if hook != nil {
		if err := hook("bound"); err != nil {
			return err
		}
	}
	stage := exportStage(receipt.Export.ID)
	if err := securefs.WriteNew(parent, stage, data); err != nil {
		return err
	}
	if err := securefs.SyncDir(parent, "."); err != nil {
		return err
	}
	if hook != nil {
		if err := hook("staged"); err != nil {
			return err
		}
	}
	if err := securefs.RenameNew(parent, stage, filepath.Base(receipt.Export.Output)); err != nil {
		return err
	}
	if hook != nil {
		if err := hook("renamed"); err != nil {
			problem := fault.Applied("export renamed but directory synchronization incomplete", receipt.Export.ID)
			problem.Details["durability_confirmed"] = false
			return problem
		}
	}

	if err := securefs.SyncDir(parent, "."); err != nil {
		problem := fault.Applied("export published but directory synchronization failed; recover the retained lock", receipt.Export.ID)
		problem.Details["durability_confirmed"] = false
		return problem
	}
	if hook != nil {
		if err := hook("published"); err != nil {
			return fault.Applied("export published but finalization interrupted", receipt.Export.ID)
		}
	}
	receipt.Phase, receipt.Applied = "applied", true
	if err := s.writeExportReceipt(receipt); err != nil {
		return fault.Applied("export published but receipt finalization failed", receipt.Export.ID)
	}
	if hook != nil {
		if err := hook("receipted"); err != nil {
			return fault.Applied("export receipt published but finalization interrupted", receipt.Export.ID)
		}
	}
	return nil
}

func (s *Store) writeExportReceipt(receipt exportReceipt) error {
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	return securefs.Replace(s.Root, exportReceiptPath(receipt.Export.ID), data)
}

// Validate every external path and existing object before recovery takes ownership.
// A digest mismatch is ambiguous evidence, never authority to overwrite or remove.
func (s *Store) inspectExport(id string) (exportReceipt, bool, error) {
	var receipt exportReceipt
	if !validID(id) {
		return receipt, false, fault.New("export.recovery_invalid", "invalid export binding")
	}
	if err := s.RequireDomain("transactions"); err != nil {
		return receipt, false, err
	}
	data, err := securefs.Read(s.Root, exportReceiptPath(id), maxMetadata)
	if err != nil {
		return receipt, false, err
	}
	if err := StrictJSON(data, &receipt); err != nil {
		return receipt, false, err
	}
	plan := receipt.Export
	if _, err := hex.DecodeString(plan.Digest); err != nil {
		return receipt, false, fault.New("export.recovery_invalid", "invalid ciphertext digest")
	}
	if receipt.Version != 1 || plan.Version != 1 || plan.ID != id || plan.Size < 1 || plan.Size > MaxBundleBytes || len(plan.Digest) != 64 || (receipt.Command != "transfer export" && receipt.Command != "backup export") || receipt.Full != (receipt.Command == "backup export") {
		return receipt, false, fault.New("export.recovery_invalid", "unsupported export receipt")
	}
	if (receipt.Phase != "prepared" && receipt.Phase != "applied" && receipt.Phase != "aborted") || receipt.Applied != (receipt.Phase == "applied") {
		return receipt, false, fault.New("export.recovery_invalid", "contradictory export phase")
	}
	abs, err := securefs.Canonical(plan.Output, true)
	if err != nil || abs != plan.Output || !filepath.IsAbs(abs) {
		return receipt, false, fault.New("export.recovery_invalid", "unsafe export target")
	}
	storePath, err := filepath.Abs(s.Dir)
	if err != nil {
		return receipt, false, err
	}
	if abs == storePath || strings.HasPrefix(abs, storePath+string(filepath.Separator)) || filepath.Base(abs) == exportStage(id) {
		return receipt, false, fault.New("export.recovery_invalid", "export target conflicts with owned paths")
	}
	parent, err := os.OpenRoot(filepath.Dir(abs))
	if err != nil {
		return receipt, false, err
	}
	defer parent.Close()
	applied := false
	output, err := securefs.Read(parent, filepath.Base(abs), MaxBundleBytes)
	if err == nil {
		if len(output) != plan.Size || digest(output) != plan.Digest {
			return receipt, false, fault.New("export.recovery_mismatch", "export target differs from prepared ciphertext")
		}
		applied = true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return receipt, false, err
	}
	if (receipt.Phase == "applied" && !applied) || (receipt.Phase == "aborted" && applied) {
		return receipt, false, fault.New("export.recovery_mismatch", "export receipt contradicts target state")
	}
	// A killed staging write may be incomplete. Only its bound private regular
	// file is cleanup material, never a symlink, hardlink, or unrelated sibling.
	if _, err := securefs.Read(parent, exportStage(id), MaxBundleBytes); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return receipt, false, err
	}
	return receipt, applied, nil
}

func (s *Store) recoverExport(id, token string) (result map[string]any, err error) {
	receipt, applied, err := s.inspectExport(id)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			if applied {
				err = fault.Applied("export recovery incomplete; lock retained", id)
			}
			err = exportRecoveryRequired(err, id)
		}
	}()
	parent, err := os.OpenRoot(filepath.Dir(receipt.Export.Output))
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	if err := parent.Remove(exportStage(id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err := securefs.SyncDir(parent, "."); err != nil {
		return nil, err
	}
	receipt.Phase, receipt.Applied = "aborted", applied
	if applied {
		receipt.Phase = "applied"
	}
	if err := s.writeExportReceipt(receipt); err != nil {
		return nil, err
	}
	if err := s.Root.Remove("lock/recovery"); err != nil {
		return nil, err
	}
	lock := &Lock{store: s, Token: token, held: true}
	if err := lock.Release(); err != nil {
		return nil, err
	}
	return map[string]any{"recovered": true, "lock_released": true, "applied": applied, "export_receipt": id, "phase": receipt.Phase}, nil
}

func exportRecoveryRequired(original error, id string) error {
	problem := fault.New("export.incomplete", "export incomplete; lock retained for explicit recovery")
	problem.Details["applied"] = false
	var previous *fault.Error
	if errors.As(original, &previous) && previous.Status == 3 {
		problem = previous
	}
	problem.Details["recovery_required"] = true
	problem.Details["export_receipt"] = id
	return problem
}
