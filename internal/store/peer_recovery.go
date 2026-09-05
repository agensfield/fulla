package store

import (
	"encoding/json"
	"reflect"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/securefs"
)

// A lock binds the single prepared rotation receipt before pin publication.
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
		Replacement Peer   `json:"replacement"`
		Phase       string `json:"phase"`
	}
	if err := StrictJSON(data, &receipt); err != nil {
		return err
	}
	if receipt.Version != 1 || receipt.Command != "peer rotate" || receipt.Previous.Name != receipt.Replacement.Name {
		return fault.New("peer.recovery_invalid", "unsupported peer rotation receipt")
	}
	if err := ValidatePeer(receipt.Previous); err != nil {
		return err
	}
	if err := ValidatePeer(receipt.Replacement); err != nil {
		return err
	}
	current, err := s.Peer(receipt.Previous.Name)
	if err != nil {
		return err
	}
	phase := ""
	if reflect.DeepEqual(current, receipt.Replacement) {
		phase = "applied"
	} else if reflect.DeepEqual(current, receipt.Previous) {
		phase = "aborted"
	} else {
		return fault.New("peer.recovery_mismatch", "live peer does not match either recorded rotation state")
	}
	if receipt.Phase != "prepared" && receipt.Phase != phase {
		return fault.New("peer.recovery_mismatch", "rotation receipt contradicts the live peer")
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
		return fault.Applied("peer rotation receipt published but synchronization failed", id)
	}
	return err
}
