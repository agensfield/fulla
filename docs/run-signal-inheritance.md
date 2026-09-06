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
| Pending blocked SIGTERM | Preserved | **Terminates before target** |
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

## Startup-hook experiment (2026-09-06)

A source overlay adding signal.Reset before syscall.Exec still failed blocked and
ignored SIGTERM. Reset is not an original-state restoration API.

A disposable prototype then used a C constructor to capture the original signal
mask and ignored dispositions before Go startup. Go performed syscall.Exec back
into the same binary; its constructor restored that state and executed the target
before Go initialization. This kept the supported syscall.Exec coordination,
original PID, one binary, and no supervising target wrapper. The five initial
signal cases and existing attached/detached PTY Ctrl-C gate passed on macOS arm64
and the established Linux amd64 devbox. The Linux Fulla-only source/binary fixture
was removed and absence confirmed. The prototype required CGO and is not adopted
into the runtime or release packager. Local draft sources and receipts remain in
dist/run-signals-bootstrap; Linux emitted a write-result compiler warning, another
reason the prototype is not represented as production-ready code.

A sixth case disproves this design as a complete fix. The launcher blocks SIGTERM,
queues it to itself, then execs the program. Direct execution preserves the pending
signal; both stock Fulla and the prototype terminate with native status -15 during
Go startup, before the target reports. The strict gate now includes that case and
records both route statuses. Original-state capture and final restoration cannot
prevent premature delivery while Go starts or prepares the target.

The next design must protect the original process's pending and blocked signals
through preparation, not merely reconstruct its final mask. An isolated native
preparation boundary needs evaluation against target PID/terminal/group behavior,
interactive plugin input, helper cleanup and public go-install/four-platform builds.
No such mechanism is selected yet. Do not integrate the five-case passing
prototype or alter CGO-free release tooling as though signal parity were solved.

The expanded gate exits 1 for both current Fulla (three failures) and the prototype
(one failure), with direct controls and unchanged store checks. Ruff and
basedpyright pass without warnings. CI 34014436153 (restore bb4090b) and
34014675730 (audit 79377c6) both passed Linux/macOS; neither executes this currently
failing strict signal gate.

## Original design consultation (2026-09-06)

The original interview rollout (thread
019fba52-6d8f-70e0-94bc-bc4e3e72d400) and a read-only follow-up distinguish
conscious intent from the broad literal sentence. The interview explained a
normal Unix exec: no shell or remaining Fulla supervisor, same PID/terminal/stdio/
cwd/process group, direct target signal delivery and target exit behavior. It
did not discuss inherited masks, ignored dispositions, pending signals or a
pre-Go startup boundary. The original design session considers syscall.Exec
faithful to that architecture and advises against a native/helper redesign solely
for the broader interpretation without Arda's decision.

The strict reproducer and literal parity failures remain valid. A narrow question
is now pending with Arda: require direct target semantics after exec and document
Go startup limits, or require exact inherited/pending-state parity and include
the startup redesign. Neither interpretation is selected here. This clarification
does not block independent implementation/acceptance and does not authorize a
weaker release claim.

## Direct target stop and continuation

TestRunTargetStopsAndContinuesWithoutSupervisor verifies the target has Fulla's
PID after checking its mapped/clean environment. The parent sends SIGSTOP and
waits for that exact PID's kernel stop notification, then sends SIGCONT. The same
target acknowledges continuation and exits 23. No diagnostic output or store
path/mode/content changes are allowed; the owned process has bounded cleanup.
This proves explicit stop/continue after exec, not shell foreground Ctrl-Z/fg or
inherited-state preservation.

The first fixture incorrectly used Go 1.26's BSD WaitStatus.Stopped/StopSignal
helpers, which classify SIGSTOP as continued. Darwin's SDK sys/wait.h uses 0x13
for continuation and defines a stopped notification as signal<<8 | 0x7f; Linux
uses the same ordinary stop encoding. The fixture now compares the exact requested
SIGSTOP notification. This changes test interpretation, not Fulla runtime behavior.
Targeted native run race tests passed in 2.798s, then CLI vet passed.
