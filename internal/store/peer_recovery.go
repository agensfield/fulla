package store

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

// A lock binds the single prepared peer receipt before pin publication.
// Recovery never scans historical receipts or infers which operation owned it.
func (s *Store) reconcilePeerReceipt(id string) error {
	if !validID(id) {
		return fault.New("peer.recovery_invalid", "invalid peer receipt binding")
	}
	if err := s.RequireDomain("peers"); err != nil {
		return err
	}
	name := metadata + "/receipts/" + id + ".json"
	data, err := securefs.Read(s.Root, name, maxMetadata)
	if err != nil {
		return err
	}
	var receipt struct {
		Version     int    `json:"version"`
		Command     string `json:"command"`
		Previous    Peer   `json:"previous"`
		Replacement *Peer  `json:"replacement,omitempty"`
		Phase       string `json:"phase"`
	}
	if err := StrictJSON(data, &receipt); err != nil {
		return err
	}
	if receipt.Version != 1 || (receipt.Command != "peer rotate" && receipt.Command != "peer remove") {
		return fault.New("peer.recovery_invalid", "unsupported peer receipt")
	}
	if err := ValidatePeer(receipt.Previous); err != nil {
		return err
	}
	if receipt.Command == "peer rotate" {
		if receipt.Replacement == nil || receipt.Previous.Name != receipt.Replacement.Name {
			return fault.New("peer.recovery_invalid", "invalid rotation replacement")
		}
		if err := ValidatePeer(*receipt.Replacement); err != nil {
			return err
		}
	} else if receipt.Replacement != nil {
		return fault.New("peer.recovery_invalid", "removal receipt includes a replacement")
	}
	registry, err := s.Root.Lstat(metadata + "/peers")
	if err != nil || !registry.IsDir() {
		return fault.New("peer.recovery_invalid", "peer registry unavailable for reconciliation")
	}
	current, err := s.Peer(receipt.Previous.Name)
	var problem *fault.Error
	missing := errors.As(err, &problem) && problem.Code == "peer.not_found"
	if err != nil && !(missing && receipt.Command == "peer remove") {
		return err
	}
	phase := ""
	if missing || (receipt.Replacement != nil && reflect.DeepEqual(current, *receipt.Replacement)) {
		phase = "applied"
	} else if reflect.DeepEqual(current, receipt.Previous) {
		phase = "aborted"
	} else {
		return fault.New("peer.recovery_mismatch", "live peer does not match the recorded operation")
	}
	if receipt.Phase != "prepared" && receipt.Phase != phase {
		return fault.New("peer.recovery_mismatch", "peer receipt contradicts the live peer")
	}
	if receipt.Phase == phase {
		return nil
	}
	receipt.Phase = phase
	data, err = json.Marshal(receipt)
	if err != nil {
		return err
	}
	published, err := securefs.ReplacePublished(s.Root, name, data)
	if err != nil && published {
		return fault.Applied("peer receipt published but synchronization failed", id)
	}
	return err
}

// Called with the shared lock held, after preparing the receipt and before
// changing the pin. The ID survives recovery ownership changes.
func (s *Store) bindPeerReceipt(id string) error {
	if !validID(id) {
		return fault.New("peer.recovery_invalid", "invalid peer receipt binding")
	}
	info, err := securefs.Read(s.Root, "lock/info", 4096)
	if err != nil {
		return err
	}
	binding := strings.TrimSpace(string(info)) + " peer_receipt=" + id + "\n"
	return securefs.Replace(s.Root, "lock/info", []byte(binding))
}
