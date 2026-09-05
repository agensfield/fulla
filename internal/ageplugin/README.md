# Maintained age plugin client

This package derives from the official `filippo.io/age v1.3.2` plugin client.
It exists to own bounded shutdown after a plugin operation has completed or
failed. File encryption/decryption and identity/recipient encoding still use
the upstream module. This is a maintained adaptation, not an unmodified upstream
package. The BSD license is preserved here and in packaged THIRD_PARTY_NOTICES.

## Auditable changes

- `client.go`: package/import relocation; identity/recipient encoding delegates
  to upstream plugin functions; `clientConnection.Close` allows one second after
  SIGINT, then kills and waits for the owned child if it has not exited.
- `format/format.go`: byte-identical upstream source, needed because age's
  internal package cannot be imported from another module.
- `format/format_test.go`: upstream deterministic tests with the import relocated.
  The CCTV-dependent fuzz function is omitted to avoid adding a test-only module;
  all other test functions and synthetic redaction constants are retained.
- `lifecycle_test.go`: Fulla regression tests prove graceful cleanup and bounded
  SIGKILL/reaping of a child that ignores SIGINT, with a watchdog that makes the
  original unbounded behavior fail deterministically.

This is a shutdown deadline, not a timeout on protocol exchanges, PIN input, or
hardware touch. It controls the direct plugin child; it does not sandbox trusted
plugins or guarantee cleanup of arbitrary independently spawned descendants.
The existing protocol behavior is deliberately preserved. Fulla's native
malformed-response acceptance exercises the integration in Linux/macOS CI.

## Source provenance

Source: https://github.com/FiloSottile/age/tree/v1.3.2.
Hashes below identify the original files before adaptations:

| Upstream file | SHA-256 |
| --- | --- |
| `plugin/client.go` | `6e8d44a4e3095c9803556520773068f44225f5951ea541d2b3d0b69abcc12cff` |
| `internal/format/format.go` | `74478c5facf49bbca09098815c637afae566f5495fb2049bf342d9ef3c93efab` |
| `internal/format/format_test.go` | `5210efe186366b44d461f1118eeb9e34155c336ddd3482393f9aee929eb6bff0` |
| `LICENSE` | `c5d65279d02955c0fc2294ae417c3add650d228f4f5c82bd6d531fc26c89cd96` |

On an age dependency update, compare these files against the new upstream source,
review each upstream protocol/parser change, refresh this adaptation and its
provenance, and rerun plugin roundtrip/interaction, lifecycle, parser, and native
acceptance. Remove the adaptation when an upstream supported lifecycle API can
supply equivalent termination/reaping behavior. Do not silently update the
module while leaving this client behind.
