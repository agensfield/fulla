package remote

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/store"
)

type sshConnection struct {
	input   io.WriteCloser
	output  io.ReadCloser
	command *exec.Cmd
	cancel  context.CancelFunc
	closed  bool
}

func (c *sshConnection) Read(p []byte) (int, error)  { return c.output.Read(p) }
func (c *sshConnection) Write(p []byte) (int, error) { return c.input.Write(p) }
func (c *sshConnection) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	_ = c.input.Close()
	_ = c.output.Close()
	defer c.cancel()
	if err := c.command.Wait(); err != nil {
		return fault.New("peer.transport_failed", "SSH transport did not complete successfully")
	}
	return nil
}

func quote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func Dial(ctx context.Context, peer store.Peer) (Connection, error) {
	if err := store.ValidatePeer(peer); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	binary := peer.Binary
	if binary == "" {
		binary = "fulla"
	}
	remoteCommand := "exec " + quote(binary)
	if peer.Store != "" {
		remoteCommand += " --store " + quote(peer.Store)
	}
	remoteCommand += " --non-interactive remote serve"
	args := []string{"-o", "BatchMode=yes"}
	for _, option := range peer.SSHOptions {
		args = append(args, "-o", option)
	}
	args = append(args, "--", peer.Host, remoteCommand)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stderr = io.Discard
	input, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		input.Close()
		output.Close()
		cancel()
		return nil, fault.New("peer.transport_failed", "could not start OpenSSH")
	}
	return &sshConnection{input: input, output: output, command: cmd, cancel: cancel}, nil
}
