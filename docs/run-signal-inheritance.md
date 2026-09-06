# Native signal inheritance gap

Status: reproduced implementation gap, not accepted parity. Source `bb4090b`,
Go 1.26.0, macOS arm64, 2026-09-06. The locked specification requires process-image
replacement preserving native signal behavior (product-spec.md, lines 107–110).

## Reproduction

```
GOTOOLCHAIN=go1.26.0 go build -o dist/fulla .
python3 scripts/acceptance-run-signals.py dist/fulla
```

The strict gate exits 1 on the current implementation. It creates a disposable
no-Git store with a synthetic value, configures each inherited signal condition
in a Python launcher, then replaces that launcher with either the same Python
target directly or Fulla followed by that target. Both routes must retain the
launcher's PID. The direct control must preserve the requested condition; only
then is Fulla compared. Complete store paths, modes and hashes remain unchanged.
The selected target reports only signal state and PID, never the mapped value.
Each owned process has a bounded wait and is killed/reaped on timeout.

| Inherited condition | Direct exec | Fulla run |
| --- | --- | --- |
| Blocked SIGUSR1 | Preserved | Preserved |
| Blocked SIGTERM | Preserved | **Lost** |
| Ignored SIGHUP | Preserved | Preserved |
| Ignored SIGINT | Preserved | Preserved |
| Ignored SIGTERM | Preserved | **Lost** |

These are macOS observations, not claimed Linux acceptance. The initial scratch
probe used a symlinked macOS temporary path; canonicalizing the fixture fixed its
initialization refusal before any signal comparison. The committed gate uses
resolved private paths. Ruff and basedpyright validate the probe separately from
its deliberately failing product assertions.

## Root cause

[Go's signal documentation](https://go.dev/src/os/signal/doc.go) explicitly states
that startup unblocks certain signals, including SIGTERM, and child execution
inherits the modified mask. It preserves inherited ignored SIGHUP/SIGINT; that
promise does not extend to SIGTERM.

The pinned Go 1.26.0 source confirms this: runtime/signal_unix.go initsig records
original dispositions but sigInstallGoHandler preserves ignored disposition only
for SIGHUP/SIGINT in the ordinary case. runtime/proc.go syscall_runtime_BeforeExec
prevents thread creation and waits for Darwin preemption signals; it does not
restore the original process mask or original dispositions. syscall.Exec then
calls execve. Fulla's Go-level main runs after this runtime initialization.

Thus direct process replacement correctly preserves PID and eventual target
signal termination, but is insufficient to preserve every inherited signal
condition. Earlier SIGINT/SIGTERM delivery and PTY Ctrl-C tests do not cover this
startup-state loss. This is an implementation defect against the locked contract,
not merely an untested combination or permission question.

## Required fix boundary

A faithful fix must recover the original state from before Go runtime startup and
restore it on the executing thread immediately before replacement. Capturing the
already-modified state in main/init, selecting default dispositions, or assuming
an empty mask cannot reconstruct what the caller supplied. A supervising process
would also violate the locked PID/process-image contract.

No private runtime symbol dependency, patched toolchain, platform entrypoint shim,
CGO requirement or weaker product promise is introduced by this audit. Determine
a supported startup/exec implementation that also satisfies public go-install
and four-platform distribution, then run this strict gate on Linux and macOS and
extend it to pending signals and job suspension/resumption. The gate is checked in
as a reproducer but not wired into the currently passing CI; no release acceptance
may treat that omission as native-signal completion.
