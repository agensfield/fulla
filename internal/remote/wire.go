// Package remote implements mutually authenticated, session-bound Fulla SSH RPC.
package remote

import (
	"encoding/binary"
	"encoding/json"
	"io"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/store"
)

const CurrentProtocol = 2
const PreviousProtocol = 1
const maxFrame = 1 << 20

type message struct {
	Operation   string                `json:"operation"`
	Version     int                   `json:"version,omitempty"`
	Recipient   string                `json:"recipient,omitempty"`
	Fingerprint string                `json:"fingerprint,omitempty"`
	Expected    string                `json:"expected,omitempty"`
	Challenge   []byte                `json:"challenge,omitempty"`
	Proof       []byte                `json:"proof,omitempty"`
	Names       []string              `json:"names,omitempty"`
	Length      uint64                `json:"length,omitempty"`
	DryRun      bool                  `json:"dry_run,omitempty"`
	Result      *store.TransferResult `json:"result,omitempty"`
	Error       *fault.Error          `json:"error,omitempty"`
	ExitStatus  int                   `json:"status,omitempty"`
}

func writeMessage(w io.Writer, m message) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if len(data) > maxFrame {
		return fault.New("protocol.limit", "control frame too large")
	}
	if err := binary.Write(w, binary.BigEndian, uint32(len(data))); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func readMessage(r io.Reader) (message, error) {
	var m message
	var n uint32
	if err := binary.Read(r, binary.BigEndian, &n); err != nil {
		return m, err
	}
	if n == 0 || n > maxFrame {
		return m, fault.New("protocol.limit", "control frame exceeds limit")
	}
	data := make([]byte, n)
	if _, err := io.ReadFull(r, data); err != nil {
		return m, err
	}
	if err := store.StrictJSON(data, &m); err != nil {
		return m, err
	}
	if m.Error != nil {
		if m.ExitStatus < 1 || m.ExitStatus > 4 {
			m.ExitStatus = 1
		}
		m.Error.Status = m.ExitStatus
		return m, m.Error
	}
	return m, nil
}

func writeFailure(w io.Writer, err error) error {
	// Protocol failures contain only Fulla-owned redacted diagnostics.
	e := fault.New("peer.operation_failed", "remote operation failed")
	if f, ok := err.(*fault.Error); ok {
		e = f
	}
	return writeMessage(w, message{Operation: "error", Error: e, ExitStatus: e.Status})
}

func readBundle(r io.Reader, length uint64) ([]byte, error) {
	if length > store.MaxBundleBytes {
		return nil, fault.New("protocol.limit", "encrypted bundle exceeds limit")
	}
	data := make([]byte, int(length))
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	return data, nil
}
