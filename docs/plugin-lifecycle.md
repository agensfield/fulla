# Plugin lifecycle investigation

The pinned age v1.3.2 client owns plugin subprocess creation and shutdown.
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

## Resolution constraints

This remains an implementation defect. Returning from a timer while leaving an
age call in a goroutine would leave both the goroutine and plugin alive and could
retain key material. Mutating the process-wide PATH to insert a wrapper would
also change concurrent executable resolution. Neither is a lifecycle fix.

A correction must provide owned child termination and reaping, preserve official
age protocol/key handling, preserve interactive PIN/touch cancellation and typed
redacted errors, and retain supported installation paths. A local module replace
or vendor-only patch is insufficient for `go install` distribution, which must
receive the same behavior as packaged builds. Evaluate an upstream-supported
process-control API or an isolated supervised helper boundary before choosing
the implementation. Do not impose a short timeout on legitimate hardware touch
merely to bound post-error process shutdown.

Relevant upstream source:
https://github.com/FiloSottile/age/blob/v1.3.2/plugin/client.go.
