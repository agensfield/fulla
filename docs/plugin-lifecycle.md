# Plugin lifecycle

The upstream age v1.3.2 client owns plugin subprocess creation and shutdown.
`plugin/client.go` closes its pipes, signals `os.Interrupt`, and calls `cmd.Wait()`
without a deadline. Its public client interface exposes UI callbacks but no
process handle, cancellation context, or termination hook.

## Reproduced failure

Build the normal executable and existing synthetic key fixture:

```sh
go build -o dist/fulla .
go test -c -o dist/age-plugin-fullafixture ./internal/crypt
python3 scripts/diagnose-plugin-shutdown.py dist/fulla dist/age-plugin-fullafixture
```

This is a diagnostic reproducer, not an acceptance success gate. It creates a
private disposable store and a synthetic plugin recipient. A fixture executable
returns malformed protocol output, closes stdout, records receipt of SIGINT,
and remains alive. The observed result on macOS was:

```json
{"plugin_received_sigint":true,"operation_still_running_after_sigint":true,"known_shutdown_gap_reproduced":true}
```

The SIGINT marker distinguishes the shutdown wait from an ordinary slow unwrap
or user touch request. Fulla's add operation remained running after that marker.
The reproducer bounds observation and kills only its newly created process group,
including the fixture plugin, before removing its temporary directory. It never
uses installed hardware plugins or live credentials.

## Implemented correction

Fulla now uses a [maintained source-derived plugin client](../internal/ageplugin/README.md)
with a narrowly scoped lifecycle patch. After the protocol finishes or fails,
it closes the pipes, sends SIGINT, allows one second for graceful cleanup,
then kills an unresponsive child and waits for it to be reaped. Official age
continues to handle file encryption/decryption and identity/recipient encoding.
No detached goroutine is left running after Close returns.

The source parser is retained byte-for-byte from age v1.3.2. Client changes,
upstream file hashes, update obligations, retained deterministic tests, and BSD
license are recorded with the adaptation. Binary packages include the license
notices. No module replacement, helper protocol, or PATH mutation is required;
Go installation compiles the same implementation as packaged builds.

The native reproducer now reports SIGINT received and no operation still running.
Run it with `--expect-cleanup` to require bounded completion, status 1, the typed
`crypto.encrypt_failed` JSON error, empty stderr, and no synthetic-value leakage.
Both hosted platforms run this mode. Direct regression tests require graceful
exit for a cooperative child and SIGKILL after the grace period for a stubborn
child, with the process reaped in either case. A five-second test watchdog makes
the original unbounded Wait fail deterministically.

This bounds shutdown, not protocol progress: a trusted plugin that never replies
or waits indefinitely for hardware remains a separate behavior. The change does
not impose a timeout on legitimate PIN/touch interaction, sandbox the plugin,
or promise termination of arbitrary independent descendants it spawns.

Relevant upstream source:
https://github.com/FiloSottile/age/blob/v1.3.2/plugin/client.go.
