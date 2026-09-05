package remote

import (
	"io"
	"time"

	"github.com/agensfield/fulla/internal/crypt"
	"github.com/agensfield/fulla/internal/fault"
	"github.com/agensfield/fulla/internal/store"
)

type Connection interface {
	io.Reader
	io.Writer
	io.Closer
}
type Client struct {
	Conn          Connection
	Version       int
	Greeting      store.IdentityInfo
	Authenticated bool
}

func (c *Client) Hello() (store.IdentityInfo, error) {
	if c.Version == 0 {
		c.Version = CurrentProtocol
	}
	if err := writeMessage(c.Conn, message{Operation: "hello", Version: c.Version}); err != nil {
		return c.Greeting, err
	}
	m, err := readMessage(c.Conn)
	if err != nil {
		return c.Greeting, err
	}
	if m.Operation != "hello" || m.Version != c.Version {
		return c.Greeting, fault.New("protocol.invalid", "invalid greeting")
	}
	if err := validateGreeting(m); err != nil {
		return c.Greeting, err
	}
	c.Greeting = store.IdentityInfo{Recipient: m.Recipient, Fingerprint: m.Fingerprint}
	return c.Greeting, nil
}

func (c *Client) Authenticate(local *store.Store, peer store.Peer) error {
	if c.Greeting.Fingerprint == "" {
		if _, err := c.Hello(); err != nil {
			return err
		}
	}
	if c.Greeting.Fingerprint != peer.Fingerprint || c.Greeting.Recipient != peer.Recipient {
		return fault.New("peer.trust_mismatch", "remote recipient changed; explicitly rotate the saved peer after verification")
	}
	identity, err := local.IdentityShow()
	if err != nil {
		return err
	}
	if err := writeMessage(c.Conn, message{Operation: "authenticate", Version: c.Version, Fingerprint: identity.Fingerprint, Expected: peer.Fingerprint}); err != nil {
		return err
	}
	m, err := readMessage(c.Conn)
	if err != nil {
		return err
	}
	if m.Operation != "challenge" {
		return fault.New("protocol.invalid", "expected session challenge")
	}
	ids, _, err := local.Keys()
	if err != nil {
		return err
	}
	proof, err := crypt.Decrypt(m.Challenge, ids)
	if err != nil {
		return err
	}
	if err := checkChallenge(proof, c.Version, peer.Fingerprint, identity.Fingerprint); err != nil {
		return err
	}
	expected, err := newChallenge(c.Version, identity.Fingerprint, peer.Fingerprint)
	if err != nil {
		return err
	}
	rs, err := crypt.Recipients([]byte(peer.Recipient), local.UI)
	if err != nil {
		return err
	}
	encrypted, err := crypt.Encrypt(expected, rs)
	if err != nil {
		return err
	}
	started := time.Now()
	if err := writeMessage(c.Conn, message{Operation: "proof", Proof: proof, Challenge: encrypted}); err != nil {
		return err
	}
	m, err = readMessage(c.Conn)
	if err != nil {
		return err
	}
	if m.Operation != "authenticated" {
		return fault.New("protocol.invalid", "expected mutual authentication proof")
	}
	if err := checkProof(m.Proof, expected, started); err != nil {
		return err
	}
	c.Authenticated = true
	return nil
}

func (c *Client) Inventory() ([]string, error) {
	if !c.Authenticated {
		return nil, fault.New("peer.unauthorized", "session is not authenticated")
	}
	if err := writeMessage(c.Conn, message{Operation: "inventory"}); err != nil {
		return nil, err
	}
	m, err := readMessage(c.Conn)
	if err != nil {
		return nil, err
	}
	if m.Operation != "inventory" {
		return nil, fault.New("protocol.invalid", "expected inventory")
	}
	if m.Names == nil {
		m.Names = []string{}
	}
	seen := map[string]bool{}
	for _, name := range m.Names {
		if _, err := store.EntryPath(name); err != nil {
			return nil, err
		}
		if seen[name] {
			return nil, fault.New("protocol.invalid", "duplicate inventory name")
		}
		seen[name] = true
	}
	return m.Names, nil
}

func (c *Client) Import(data []byte) (store.TransferResult, error) {
	result := store.TransferResult{}
	if !c.Authenticated {
		return result, fault.New("peer.unauthorized", "session is not authenticated")
	}
	if err := writeMessage(c.Conn, message{Operation: "import", Length: uint64(len(data))}); err != nil {
		return result, err
	}
	if _, err := c.Conn.Write(data); err != nil {
		return result, err
	}
	m, err := readMessage(c.Conn)
	if err != nil {
		return result, err
	}
	if m.Operation != "imported" || m.Result == nil {
		return result, fault.New("protocol.invalid", "expected import receipt")
	}
	return *m.Result, nil
}

func (c *Client) Export(names []string) ([]byte, error) {
	if !c.Authenticated {
		return nil, fault.New("peer.unauthorized", "session is not authenticated")
	}
	if err := writeMessage(c.Conn, message{Operation: "export", Names: names}); err != nil {
		return nil, err
	}
	m, err := readMessage(c.Conn)
	if err != nil {
		return nil, err
	}
	if m.Operation != "exported" {
		return nil, fault.New("protocol.invalid", "expected encrypted export")
	}
	return readBundle(c.Conn, m.Length)
}

func (c *Client) Mark(dryRun bool) error {
	if !c.Authenticated {
		return fault.New("peer.unauthorized", "session is not authenticated")
	}
	op := "activate"
	if dryRun {
		op = "dry-run"
	}
	if err := writeMessage(c.Conn, message{Operation: op}); err != nil {
		return err
	}
	m, err := readMessage(c.Conn)
	if err != nil {
		return err
	}
	if m.Operation != "ok" {
		return fault.New("protocol.invalid", "expected operation acknowledgement")
	}
	return nil
}

func (c *Client) Close() error {
	if err := writeMessage(c.Conn, message{Operation: "bye"}); err != nil {
		_ = c.Conn.Close()
		return err
	}
	if c.Authenticated {
		m, err := readMessage(c.Conn)
		if err != nil {
			_ = c.Conn.Close()
			return err
		}
		if m.Operation != "bye" {
			_ = c.Conn.Close()
			return fault.New("protocol.invalid", "expected session close acknowledgement")
		}
	}
	return c.Conn.Close()
}
