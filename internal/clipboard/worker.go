package clipboard

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/agensfield/fulla/internal/fault"
)

const WorkerFlag = "--internal-clipboard-expire"

type request struct {
	Backend Backend   `json:"backend"`
	Digest  string    `json:"digest"`
	Expires time.Time `json:"expires"`
}

// Schedule starts a fresh process, so it cannot inherit the parent's decrypted
// heap. The request contains a digest, deadline, and transport commands only.
func Schedule(b Backend, digest string, after time.Duration) error {
	if after <= 0 || after > 24*time.Hour || !b.valid() {
		return fault.Usage("invalid clipboard expiry request")
	}
	executable, err := os.Executable()
	if err != nil {
		return fault.New("clipboard.expiry_failed", "cannot locate expiry worker executable")
	}
	data, err := json.Marshal(request{b, digest, time.Now().Add(after)})
	if err != nil {
		return err
	}
	cmd := exec.Command(executable, WorkerFlag)
	cmd.Env = environment()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = bytes.NewReader(data)
	cmd.Stderr = nil
	output, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fault.New("clipboard.expiry_failed", "could not start clipboard expiry worker")
	}
	ready := make(chan bool, 1)
	go func() { var ack [1]byte; _, err := io.ReadFull(output, ack[:]); ready <- err == nil && ack[0] == '1' }()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	ok := false
	select {
	case ok = <-ready:
	case <-timer.C:
	}
	_ = output.Close()
	if !ok {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fault.New("clipboard.expiry_failed", "clipboard expiry worker did not acknowledge scheduling")
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// Worker communicates only a startup acknowledgement. Clipboard data is
// streamed directly into a digest at expiry and never printed or logged.
func Worker() int {
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 16385))
	if err != nil || len(data) > 16384 {
		return 2
	}
	var r request
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&r) != nil || decoder.Decode(new(any)) != io.EOF || !r.Backend.valid() {
		return 2
	}
	digest, err := hex.DecodeString(r.Digest)
	remaining := time.Until(r.Expires)
	if err != nil || len(digest) != 32 || remaining > 24*time.Hour || remaining < -time.Minute {
		return 2
	}
	if _, err := os.Stdout.Write([]byte{'1'}); err != nil {
		return 1
	}
	_ = os.Stdout.Close()
	if remaining > 0 {
		time.Sleep(remaining)
	}
	if _, err := r.Backend.ClearIfMatching(r.Digest); err != nil {
		return 1
	}
	return 0
}
