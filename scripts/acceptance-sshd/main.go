// acceptance-sshd is a disposable loopback-only SSH fixture. It is not linked
// into or distributed with Fulla. Every accepted exec runs only the configured
// Fulla remote server, regardless of the supplied SSH command string.
package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"golang.org/x/crypto/ssh"
)

func main() {
	syscall.Umask(0o077)
	dir := flag.String("dir", "", "new fixture directory")
	binary := flag.String("binary", "", "absolute Fulla binary")
	store := flag.String("store", "", "absolute isolated store")
	remotePort := flag.Int("remote-port", 40222, "loopback reverse forwarding port")
	flag.Parse()
	if !filepath.IsAbs(*dir) || !filepath.IsAbs(*binary) || !filepath.IsAbs(*store) {
		panic("absolute fixture paths required")
	}
	if err := os.Mkdir(*dir, 0o700); err != nil {
		panic(err)
	}
	_, hostPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	_, clientPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	hostSigner, err := ssh.NewSignerFromKey(hostPrivate)
	if err != nil {
		panic(err)
	}
	clientSigner, err := ssh.NewSignerFromKey(clientPrivate)
	if err != nil {
		panic(err)
	}
	key, err := ssh.MarshalPrivateKey(clientPrivate, "Fulla acceptance fixture")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(*dir, "client-key"), pem.EncodeToMemory(key), 0o600); err != nil {
		panic(err)
	}
	knownHosts := fmt.Sprintf("[127.0.0.1]:%d %s", *remotePort, ssh.MarshalAuthorizedKey(hostSigner.PublicKey()))
	if err := os.WriteFile(filepath.Join(*dir, "known-hosts"), []byte(knownHosts), 0o600); err != nil {
		panic(err)
	}
	config := &ssh.ServerConfig{PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if meta.User() != "fulla-fixture" || !bytes.Equal(key.Marshal(), clientSigner.PublicKey().Marshal()) {
			return nil, fmt.Errorf("fixture authentication rejected")
		}
		return nil, nil
	}}
	config.AddHostKey(hostSigner)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	info := map[string]any{"address": listener.Addr().String(), "remote_port": *remotePort, "user": "fulla-fixture", "client_key": filepath.Join(*dir, "client-key"), "known_hosts": filepath.Join(*dir, "known-hosts")}
	data, _ := json.Marshal(info)
	if err := os.WriteFile(filepath.Join(*dir, "info.json"), data, 0o600); err != nil {
		panic(err)
	}
	fmt.Println(string(data))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go func() { <-ctx.Done(); listener.Close() }()
	for {
		connection, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			panic(err)
		}
		go serve(ctx, connection, config, *binary, *store)
	}
}

func serve(ctx context.Context, connection net.Conn, config *ssh.ServerConfig, binary, store string) {
	defer connection.Close()
	go func() { <-ctx.Done(); connection.Close() }()
	server, channels, requests, err := ssh.NewServerConn(connection, config)
	if err != nil {
		return
	}
	defer server.Close()
	go ssh.DiscardRequests(requests)
	for incoming := range channels {
		if incoming.ChannelType() != "session" {
			_ = incoming.Reject(ssh.UnknownChannelType, "fixture only accepts sessions")
			continue
		}
		channel, requests, err := incoming.Accept()
		if err != nil {
			continue
		}
		go func() {
			defer channel.Close()
			for request := range requests {
				if request.Type != "exec" {
					_ = request.Reply(false, nil)
					continue
				}
				_ = request.Reply(true, nil)
				command := exec.CommandContext(ctx, binary, "--store", store, "--non-interactive", "remote", "serve")
				command.Stdin = channel
				command.Stdout = channel
				command.Stderr = channel.Stderr()
				code := 0
				if err := command.Run(); err != nil {
					code = 1
					if exit, ok := err.(*exec.ExitError); ok {
						code = exit.ExitCode()
					}
				}
				_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(code)}))
				return
			}
		}()
	}
}
