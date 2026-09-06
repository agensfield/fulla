package remote

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/store"
)

// This reader drops a complete control reply from the real SSH server before
// exposing it to the client. Both selected boundaries precede any bundle body.
type crossHostDrop struct {
	Connection
	operation string
	pending   []byte
	dropped   bool
}

func (c *crossHostDrop) Read(p []byte) (int, error) {
	if c.dropped {
		return 0, io.EOF
	}
	if len(c.pending) == 0 {
		var header [4]byte
		if _, err := io.ReadFull(c.Connection, header[:]); err != nil {
			return 0, err
		}
		n := binary.BigEndian.Uint32(header[:])
		if n == 0 || n > maxFrame {
			return 0, errors.New("invalid fixture control frame")
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(c.Connection, body); err != nil {
			return 0, err
		}
		var m message
		if err := json.Unmarshal(body, &m); err != nil {
			return 0, err
		}
		if m.Operation == c.operation {
			c.dropped = true
			return 0, io.EOF
		}
		if m.Length != 0 {
			return 0, errors.New("unexpected bundle before selected drop")
		}
		c.pending = append(header[:], body...)
	}
	n := copy(p, c.pending)
	c.pending = c.pending[n:]
	return n, nil
}

// Explicit opt-in only. The operator uploads the reviewed binary and selects an
// existing Fulla-only parent. SSH configuration and installed binaries are never
// changed. Pins are seeded as fixture data, not claimed as CLI enrollment proof.
func TestCrossHostPartialRetry(t *testing.T) {
	host := os.Getenv("FULLA_ACCEPTANCE_HOST")
	if host == "" {
		t.Skip("requires explicit Fulla cross-host acceptance target")
	}
	remoteBinary := os.Getenv("FULLA_ACCEPTANCE_BINARY")
	parent := os.Getenv("FULLA_ACCEPTANCE_PARENT")
	if strings.HasPrefix(host, "-") || !filepath.IsAbs(remoteBinary) || !filepath.IsAbs(parent) {
		t.Fatal("explicit SSH host and absolute remote binary/fixture parent required")
	}
	remoteCommand := func(t *testing.T, data []byte, args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		quoted := make([]string, len(args))
		for i, arg := range args {
			quoted[i] = quote(arg)
		}
		cmd := exec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=10", "--", host, strings.Join(quoted, " "))
		cmd.Stdin = bytes.NewReader(data)
		var diagnostic bytes.Buffer
		cmd.Stderr = &diagnostic
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("fixture SSH command failed: %v; %s", err, diagnostic.Bytes())
		}
		return out
	}
	t.Logf("local platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	t.Logf("remote platform: %s", strings.TrimSpace(string(remoteCommand(t, nil, "uname", "-sm"))))
	for _, noGit := range []bool{false, true} {
		mode := "git"
		if noGit {
			mode = "no-git"
		}
		for _, boundary := range []string{"imported", "exported"} {
			t.Run(mode+"/"+boundary, func(t *testing.T) {
				ssh := func(data []byte, args ...string) []byte { return remoteCommand(t, data, args...) }
				localParent, err := filepath.EvalSymlinks(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				dir := filepath.Join(localParent, "store")
				if _, err := store.Init(dir, noGit, false); err != nil {
					t.Fatal(err)
				}
				local, err := store.Open(dir, true, nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = local.Close() })
				root := strings.TrimSpace(string(ssh(nil, "python3", "-c", "import tempfile,sys; print(tempfile.mkdtemp(prefix='fulla-partial-', dir=sys.argv[1]))", parent)))
				if filepath.Dir(root) != parent || !strings.HasPrefix(filepath.Base(root), "fulla-partial-") {
					t.Fatal("remote fixture path outside selected parent")
				}
				t.Cleanup(func() {
					_ = ssh(nil, "python3", "-c", "import shutil,sys; shutil.rmtree(sys.argv[1])", root)
				})
				remoteStore := root + "/store"
				cli := func(data []byte, args ...string) []byte {
					return ssh(data, append([]string{remoteBinary, "--store", remoteStore}, args...)...)
				}
				args := []string{"init", "--yes", "--json"}
				if noGit {
					args = append(args, "--no-git")
				}
				_ = cli(nil, args...)
				var envelope struct {
					Data store.IdentityInfo `json:"data"`
				}
				if err := json.Unmarshal(cli(nil, "identity", "show", "--json"), &envelope); err != nil {
					t.Fatal(err)
				}
				peer := store.Peer{Version: 1, Name: "physical", Host: host, Binary: remoteBinary, Store: remoteStore, Recipient: envelope.Data.Recipient, Fingerprint: envelope.Data.Fingerprint, SSHOptions: []string{"StrictHostKeyChecking=yes", "ConnectTimeout=10"}}
				if err := local.SavePeer(peer, false, ""); err != nil {
					t.Fatal(err)
				}
				identity, err := local.IdentityShow()
				if err != nil {
					t.Fatal(err)
				}
				pin, err := json.Marshal(store.Peer{Version: 1, Name: "physical", Host: "fixture-unused", Recipient: identity.Recipient, Fingerprint: identity.Fingerprint})
				if err != nil {
					t.Fatal(err)
				}
				_ = ssh(pin, "python3", "-c", "import os,sys; os.umask(0o077); f=open(sys.argv[1], 'xb'); f.write(sys.stdin.buffer.read()); f.close()", remoteStore+"/.fulla/peers/physical.json")
				left, right := []byte{0, 255, 10}, []byte{}
				for name, value := range map[string][]byte{"left": left, "shared": []byte("local divergent")} {
					if _, err := local.Write(name, value, false); err != nil {
						t.Fatal(err)
					}
				}
				_ = cli(right, "add", "right", "--stdin")
				_ = cli([]byte("remote divergent"), "add", "shared", "--stdin")
				sync := func(dry bool, drop string) (SyncResult, error, bool) {
					t.Helper()
					peer, err := local.Peer("physical")
					if err != nil {
						t.Fatal(err)
					}
					ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
					conn, err := Dial(ctx, peer)
					if err != nil {
						cancel()
						t.Fatal(err)
					}
					defer func() { cancel(); _ = conn.Close() }()
					client := &Client{Conn: conn, Version: CurrentProtocol}
					intercept := &crossHostDrop{Connection: conn, operation: drop}
					if drop != "" {
						client.Conn = intercept
					}
					result, err := Sync(local, peer, client, dry, false)
					return result, err, intercept.dropped
				}
				if _, err, _ := sync(true, ""); err != nil {
					t.Fatal(err)
				}
				partial, err, dropped := sync(false, boundary)
				var problem *fault.Error
				if !dropped || !errors.As(err, &problem) || problem.Code != "sync.partial" || problem.Status != 3 || partial.Pulled || partial.Activated || partial.Pushed != (boundary == "exported") || partial.RemoteUncertain != (boundary == "imported") {
					t.Fatalf("incorrect partial evidence: %+v %v dropped=%v", partial, err, dropped)
				}
				var receipt struct {
					Outcome string     `json:"outcome"`
					Result  SyncResult `json:"result"`
				}
				data, err := os.ReadFile(filepath.Join(local.Dir, partial.Receipt))
				if err != nil || json.Unmarshal(data, &receipt) != nil || receipt.Outcome != "partial" || receipt.Result.RemoteUncertain != partial.RemoteUncertain || receipt.Result.Pushed != partial.Pushed || receipt.Result.Pulled || receipt.Result.Activated {
					t.Fatal("missing durable partial receipt", err)
				}
				if _, err := os.Stat(filepath.Join(local.Dir, "passwords/right.age")); !os.IsNotExist(err) {
					t.Fatal("pull unexpectedly committed")
				}
				if !bytes.Equal(cli(nil, "show", "left"), left) {
					t.Fatal("remote commit missing")
				}
				cipher := ssh(nil, "cat", remoteStore+"/passwords/left.age")
				result, err, _ := sync(false, "")
				if err != nil || result.Pushed || !result.Pulled || !result.Activated || result.RemoteUncertain || len(result.Push) != 0 || len(result.Pull) != 1 || len(result.Skipped) != 2 {
					t.Fatalf("retry failed: %+v %v", result, err)
				}
				if !bytes.Equal(cipher, ssh(nil, "cat", remoteStore+"/passwords/left.age")) {
					t.Fatal("retry rewrote committed ciphertext")
				}
				for name, expected := range map[string][]byte{"left": left, "right": right, "shared": []byte("local divergent")} {
					value, err := local.Read(name)
					if err != nil || !bytes.Equal(value, expected) {
						t.Fatal("local exact-byte mismatch", name, err)
					}
					if name == "shared" {
						expected = []byte("remote divergent")
					}
					if !bytes.Equal(cli(nil, "show", name), expected) {
						t.Fatal("remote exact-byte mismatch", name)
					}
				}
				if health, err := local.Doctor(true); err != nil || !health.Healthy {
					t.Fatal(err)
				}
				var remoteHealth struct {
					Data store.DoctorResult `json:"data"`
				}
				if err := json.Unmarshal(cli(nil, "doctor", "--deep", "--json"), &remoteHealth); err != nil || !remoteHealth.Data.Healthy {
					t.Fatal("remote deep verification failed", err)
				}
				t.Logf("verified %s/%s: partial status 3, durable receipt, exact bytes, unchanged committed ciphertext, retry activation", mode, boundary)
			})
		}
	}
}
