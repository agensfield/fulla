# Trust-change acceptance

`internal/remote/trust_journey_test.go` runs one ordered journey on each
Git/no-Git × current/previous protocol combination, using generated disposable
stores and real in-process protocol connections:

1. Give both stores a unique binary entry and different values under `shared`.
2. Enroll the server only on the client. Sync must return `peer.unauthorized`.
3. Enroll the client on the server, then rotate the server's identity. The old
   client pin must return `peer.trust_mismatch`.
4. Explicitly replace that saved pin, acknowledging its previous fingerprint.
5. Capture a valid client proof from one session. Present it to a fresh session;
   the server must return `peer.authentication_failed` before using its identity.
6. Restore fixture identities, complete mutual authentication and the mandatory
   dry-run, then synchronize. Verify both unique binary values byte-for-byte on
   both sides and retain each side's different `shared` value.

During the three refusal stages, each store has a sole age plugin identity whose
executable records an invocation and exits. This is an execution sentinel, not
a working decryption plugin. Before each use, an explicit local entry read must
invoke it exactly once and fail; the fixture then clears the marker. Failed trust
must leave the marker absent. With no fallback native identity, this catches
premature attempts to unwrap the valid fixture entries/challenges. Refusals also
compare every file digest and every path/mode on both stores.

The sentinel deliberately substitutes synthetic identities while retaining the
original public recipients and ciphertext. It is not a valid production identity
configuration or hardware acceptance. Original generated identities are restored
for successful authentication and transfer. No runtime instrumentation, secret
logging, or test switch is added to the product.

Two local negative-control Go overlays insert a premature `Read("shared")` into
client authentication and server startup, respectively. Each must make the
journey fail at the decryption sentinel. The positive controls and these source
mutations establish that absence of a marker is meaningful, rather than a broken
observer or an unavailable plugin executable.

This is bounded J5/I3 evidence for the named refusals on both supported protocols.
It complements the stricter malformed/expired/same-session replay fixtures in
`auth_test.go`. It does not replace a protocol security review, instrument every
cryptographic parser call, or prove physical two-host transport behavior and
one-sided failure/retry (J4). Linux/macOS CI runs the journey through the normal
remote race-test suite.
