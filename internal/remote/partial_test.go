package remote

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/store"
)

// Drop one complete server control reply, after the production server performed
// the operation. Both selected boundaries precede any exported bundle body.
type dropReply struct {
	output     io.Writer
	operation  string
	header     []byte
	dropped    chan struct{}
	failed     bool
	beforeDrop func()
}

func (w *dropReply) Write(data []byte) (int, error) {
	if w.failed {
		return 0, io.ErrClosedPipe
	}
	if w.header == nil {
		if len(data) != 4 {
			return 0, errors.New("fixture expected frame header")
		}
		w.header = bytes.Clone(data)
		return len(data), nil
	}
	var m message
	if err := json.Unmarshal(data, &m); err != nil {
		return 0, err
	}
	if m.Operation == w.operation {
		w.failed = true
		if w.beforeDrop != nil {
			w.beforeDrop()
		}
		close(w.dropped)
		return 0, io.ErrClosedPipe
	}
	if _, err := w.output.Write(w.header); err != nil {
		return 0, err
	}
	w.header = nil
	return w.output.Write(data)
}

func interruptedSession(t *testing.T, server *store.Store, version int, operation string, beforeDrop func()) (*Client, <-chan struct{}) {
	t.Helper()
	opened, err := store.Open(server.Dir, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	left, right := net.Pipe()
	_ = left.SetDeadline(time.Now().Add(20 * time.Second))
	_ = right.SetDeadline(time.Now().Add(20 * time.Second))
	writer := &dropReply{output: right, operation: operation, dropped: make(chan struct{}), beforeDrop: beforeDrop}
	done := make(chan struct{})
	go func() { defer close(done); defer opened.Close(); defer right.Close(); _ = Serve(opened, right, writer) }()
	t.Cleanup(func() { _ = left.Close(); _ = right.Close(); <-done })
	return &Client{Conn: left, Version: version}, writer.dropped
}

func TestSyncPartialCommitAndRetry(t *testing.T) {
	for _, version := range []int{CurrentProtocol, PreviousProtocol} {
		for _, boundary := range []string{"imported", "exported"} {
			t.Run(strconv.Itoa(version)+"/"+boundary, func(t *testing.T) {
				left, right := paired(t)
				values := map[string][]byte{"left": {0, 255, 10}, "right": {}, "shared-left": []byte("local divergent"), "shared-right": []byte("remote divergent")}
				for _, item := range []struct {
					s         *store.Store
					name, key string
				}{{left, "left", "left"}, {right, "right", "right"}, {left, "shared", "shared-left"}, {right, "shared", "shared-right"}} {
					if _, err := item.s.Write(item.name, values[item.key], false); err != nil {
						t.Fatal(err)
					}
				}
				peer, err := left.Peer("other")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := Sync(left, peer, session(t, right, version), true, false); err != nil {
					t.Fatal(err)
				}
				peer, err = left.Peer("other")
				if err != nil {
					t.Fatal(err)
				}
				client, dropped := interruptedSession(t, right, version, boundary, nil)
				result, err := Sync(left, peer, client, false, false)
				_ = client.Conn.Close()
				var problem *fault.Error
				if !errors.As(err, &problem) || problem.Code != "sync.partial" || problem.Status != 3 {
					t.Fatal("lost partial outcome", err)
				}
				select {
				case <-dropped:
				default:
					t.Fatal("fixture did not reach selected commit boundary")
				}
				if result.Pushed != (boundary == "exported") || result.RemoteUncertain != (boundary == "imported") || result.Pulled || result.Activated {
					t.Fatalf("incorrect applied-state evidence: %+v", result)
				}
				if result.Receipt == "" {
					t.Fatal("missing durable partial receipt")
				}
				evidence, err := os.ReadFile(filepath.Join(left.Dir, result.Receipt))
				if err != nil {
					t.Fatal(err)
				}
				var saved struct {
					Outcome string     `json:"outcome"`
					Result  SyncResult `json:"result"`
				}
				if err := json.Unmarshal(evidence, &saved); err != nil || saved.Outcome != "partial" || saved.Result.RemoteUncertain != result.RemoteUncertain || saved.Result.Pushed != result.Pushed || saved.Result.Activated {
					t.Fatal("partial receipt lost observed state", err)
				}

				if _, err := os.Stat(filepath.Join(left.Dir, "passwords/right.age")); !os.IsNotExist(err) {
					t.Fatal("pull unexpectedly published")
				}
				pushed, err := right.Read("left")
				if err != nil || !bytes.Equal(pushed, values["left"]) {
					t.Fatal("remote did not commit before disconnect", err)
				}
				remoteCipher, err := right.Ciphertext("left")
				if err != nil {
					t.Fatal(err)
				}
				for _, s := range []*store.Store{left, right} {
					if _, err := os.Stat(filepath.Join(s.Dir, "lock")); !os.IsNotExist(err) {
						t.Fatal("partial sync retained lock")
					}
				}
				result, err = Sync(left, peer, session(t, right, version), false, false)
				if err != nil || result.Pushed || !result.Pulled || !result.Activated || result.RemoteUncertain || len(result.Push) != 0 || len(result.Pull) != 1 || len(result.Skipped) != 2 {
					t.Fatalf("retry did not converge: %+v %v", result, err)
				}
				after, err := right.Ciphertext("left")
				if err != nil || !bytes.Equal(remoteCipher, after) {
					t.Fatal("retry rewrote committed remote entry", err)
				}
				for _, item := range []struct {
					s      *store.Store
					shared string
				}{{left, "shared-left"}, {right, "shared-right"}} {
					for _, name := range []string{"left", "right", "shared"} {
						key := name
						if name == "shared" {
							key = item.shared
						}
						got, err := item.s.Read(name)
						if err != nil || !bytes.Equal(got, values[key]) {
							t.Fatal("retry changed exact bytes or shared name", name, err)
						}
					}
				}
				if result.Receipt == "" {
					t.Fatal("missing converged receipt")
				}
				receipt, err := os.ReadFile(filepath.Join(left.Dir, result.Receipt))
				if err != nil {
					t.Fatal(err)
				}
				var recorded struct {
					Result SyncResult `json:"result"`
				}
				if err := json.Unmarshal(receipt, &recorded); err != nil || !recorded.Result.Activated || !recorded.Result.Pulled || recorded.Result.Pushed {
					t.Fatal("receipt disagrees with retry", err)
				}
			})
		}
	}
}

func TestPartialReceiptFailurePreservesUncertainCommit(t *testing.T) {
	left, right := paired(t)
	if _, err := left.Write("entry", []byte("fixture"), false); err != nil {
		t.Fatal(err)
	}
	peer, err := left.Peer("other")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(left, peer, session(t, right, CurrentProtocol), true, false); err != nil {
		t.Fatal(err)
	}
	peer, err = left.Peer("other")
	if err != nil {
		t.Fatal(err)
	}
	type acquired struct {
		lock *store.Lock
		err  error
	}
	held := make(chan acquired, 1)
	client, dropped := interruptedSession(t, right, CurrentProtocol, "imported", func() {
		lock, err := left.Lock("fixture receipt contention")
		held <- acquired{lock, err}
	})
	result, err := Sync(left, peer, client, false, false)
	_ = client.Conn.Close()
	select {
	case <-dropped:
	default:
		t.Fatal("did not reach committed import")
	}
	ownership := <-held
	if ownership.err != nil {
		t.Fatal(ownership.err)
	}
	defer ownership.lock.Release()
	var problem *fault.Error
	if !errors.As(err, &problem) || problem.Code != "sync.partial" || problem.Status != 3 || !result.RemoteUncertain || result.Receipt != "" || problem.Details["receipt_error"] != "sync.receipt_failed" {
		t.Fatalf("receipt failure obscured uncertain commit: %+v %v", result, err)
	}
	got, err := right.Read("entry")
	if err != nil || string(got) != "fixture" {
		t.Fatal("remote commit missing", err)
	}
}
