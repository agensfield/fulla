package remote

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
	"github.com/agensfield/fulla/internal/store"
)

type SyncResult struct {
	Peer            string   `json:"peer"`
	DryRun          bool     `json:"dry_run"`
	Push            []string `json:"push"`
	Pull            []string `json:"pull"`
	Skipped         []string `json:"skipped"`
	Pushed          bool     `json:"pushed"`
	Pulled          bool     `json:"pulled"`
	RemoteUncertain bool     `json:"remote_uncertain"`
	Activated       bool     `json:"activated"`
	Receipt         string   `json:"receipt,omitempty"`
}

func Plan(local, remote []string) (push, pull, shared []string) {
	push = []string{}
	pull = []string{}
	shared = []string{}
	l := map[string]bool{}
	r := map[string]bool{}
	for _, name := range local {
		l[name] = true
	}
	for _, name := range remote {
		r[name] = true
	}
	for name := range l {
		if r[name] {
			shared = append(shared, name)
		} else {
			push = append(push, name)
		}
	}
	for name := range r {
		if !l[name] {
			pull = append(pull, name)
		}
	}
	sort.Strings(push)
	sort.Strings(pull)
	sort.Strings(shared)
	return
}

func Sync(local *store.Store, peer store.Peer, client *Client, dryRun, strict bool) (result SyncResult, err error) {
	result = SyncResult{Peer: peer.Name, DryRun: dryRun, Push: []string{}, Pull: []string{}, Skipped: []string{}}
	receiptAttempted := false
	defer func() {
		var problem *fault.Error
		if receiptAttempted || !errors.As(err, &problem) || problem.Code != "sync.partial" {
			return
		}
		if receiptErr := syncReceipt(local, &result, "partial"); receiptErr != nil {
			problem.Details["receipt_error"] = "sync.receipt_failed"
		}
		problem.Details["state"] = result
	}()

	if err := local.RequireDomain("sync"); err != nil {
		return result, err
	}
	if err := client.Authenticate(local, peer); err != nil {
		return result, err
	}
	identity, err := local.IdentityShow()
	if err != nil {
		return result, err
	}
	local.ExpectedFingerprint = identity.Fingerprint
	local.ExpectedPeerName = peer.Name
	local.ExpectedPeerFingerprint = peer.Fingerprint
	defer func() {
		local.ExpectedFingerprint = ""
		local.ExpectedPeerName = ""
		local.ExpectedPeerFingerprint = ""
	}()
	remoteNames, err := client.Inventory()
	if err != nil {
		return result, err
	}
	localNames, err := local.Names()
	if err != nil {
		return result, err
	}
	result.Push, result.Pull, result.Skipped = Plan(localNames, remoteNames)
	if !dryRun && peer.DryRunIdentity != identity.Fingerprint {
		return result, fault.New("sync.dry_run_required", "first authenticated sync --dry-run is mandatory")
	}
	if dryRun {
		if err := client.Mark(true); err != nil {
			return result, err
		}
		if err := local.MarkPeer(peer.Name, peer.Fingerprint, identity.Fingerprint, false); err != nil {
			return result, err
		}
	} else {
		rs, err := crypt.Recipients([]byte(peer.Recipient), local.UI)
		if err != nil {
			return result, err
		}
		if len(result.Push) > 0 {
			var bundle bytes.Buffer
			if _, err := local.ExportLogical(result.Push, rs, "-", &bundle); err != nil {
				return result, err
			}
			result.RemoteUncertain = true
			if _, err := client.Import(bundle.Bytes()); err != nil {
				return result, partial(result, "remote import did not acknowledge completion")
			}
			result.RemoteUncertain = false
			result.Pushed = true
		}
		if len(result.Pull) > 0 {
			bundle, err := client.Export(result.Pull)
			if err != nil {
				if result.Pushed {
					return result, partial(result, "push applied but remote export failed")
				}
				return result, err
			}
			if _, err := local.ImportLogical(bundle, nil); err != nil {
				return result, partial(result, "pull import did not finalize; inspect local recovery state")
			}
			result.Pulled = true
		}
		if err := client.Mark(false); err != nil {
			return result, partial(result, "sync data converged but remote activation was not acknowledged")
		}
		if err := local.MarkPeer(peer.Name, peer.Fingerprint, identity.Fingerprint, true); err != nil {
			var problem *fault.Error
			if errors.As(err, &problem) && problem.Details["applied"] == true {
				result.Activated = true
			}
			return result, partial(result, "sync data converged but local activation did not finalize")
		}
		result.Activated = true
	}
	if err := client.Close(); err != nil {
		return result, partial(result, "sync steps completed but SSH finalization failed")
	}
	receiptAttempted = true
	if err := syncReceipt(local, &result, "completed"); err != nil {
		return result, partial(result, "sync completed but receipt finalization failed")
	}
	if strict && len(result.Skipped) > 0 {
		e := fault.New("sync.skipped", "shared names were skipped without comparing plaintext")
		e.Status = 4
		e.Details["names"] = result.Skipped
		return result, e
	}
	return result, nil
}

// A partial receipt records observed state, including uncertain remote commits.
// Failure to persist it must never hide the original partial outcome.
func syncReceipt(local *store.Store, result *SyncResult, outcome string) error {
	id := securefs.ID()
	receipt := ".fulla/receipts/" + id + ".json"
	recorded := *result
	recorded.Receipt = receipt
	data, err := json.Marshal(map[string]any{"version": 1, "command": "sync", "outcome": outcome, "result": recorded, "at": time.Now().UTC().Format(time.RFC3339Nano), "pa_xfer_retired": result.Activated})
	if err != nil {
		return err
	}
	lock, err := local.Lock("sync receipt")
	if err != nil {
		return err
	}
	err = securefs.PublishNew(local.Root, receipt, data)
	if err == nil {
		result.Receipt = receipt
	}
	releaseErr := lock.Release()
	if err != nil {
		return err
	}
	return releaseErr
}

func partial(result SyncResult, message string) *fault.Error {
	e := fault.New("sync.partial", message+"; rerun safely to converge without overwriting")
	e.Status = 3
	e.Details["state"] = result
	return e
}
