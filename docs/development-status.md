# Development status

Fulla is an active implementation, not an accepted preview. No live credential
store has been changed. Public repository: https://github.com/agensfield/fulla.

## Verified core checkpoint (2026-09-05)

- Rooted private filesystem operations and atomic no-replace publication on
  Darwin/Linux; strict TOML configuration and exact-byte embedded age.
- pa-v1 initialization, read-only adoption preflight, metadata-only adoption,
  shared lock, strict CRUD, encrypted Git commits, and journaled mutations.
- Development CLI with versioned JSON, raw show, canonical aliases, explicit
  stdin/descriptor/generation input. Main sets private umask 077.
- Failure-injection tests cover staged, per-file published, committed, and
  receipted boundaries; retries reject unrelated ciphertext changes.
- `go test -race ./...`, `go vet ./...`, and local CLI build passed.

## Required follow-through

Public history/snapshot restoration, encrypted before/after snapshots, selected
PAXFER1 export/verify/import, full-state disaster archives, identity rotation,
peer enrollment, and mutually authenticated SSH sync are implemented. Local
unit/integration coverage includes successive rotation and sealed-key history
recovery. These features still require the broader acceptance gates below.

Hosted Linux/macOS race/static gates and four-platform cross-builds passed at
`831caf6`: https://github.com/agensfield/fulla/actions/runs/33976819325.
Vault checkpoint PR 212 merged and the canonical checkout fast-forwarded safely.

### Isolated cross-host acceptance (2026-09-05)

A macOS arm64 client and Linux amd64 devbox used generated, disposable stores.
The system OpenSSH transport used a temporary loopback reverse tunnel and a
fixed-command SSH fixture from `scripts/acceptance-sshd`. This fixture is not
linked into or distributed with Fulla. Saved peer transport options support
explicit ports, identity files, and known-host files without changing SSH config.

The drill verified independent mutual enrollment, the mandatory dry-run,
bidirectional unique-name sync, byte-exact binary values, and preservation of
different values under a shared name. Reverse-direction invocation also passed.
Remote fixture identity rotation caused the saved pin to fail closed; explicit
peer rotation and a fresh dry-run restored access. Strict shared-name skipping
returned status 4. Both temporary SSH processes have exited.

Local ignored receipts are `dist/acceptance/cross-host-receipt.json` and
`dist/acceptance/cross-host-rotation-receipt.json`. They document this manual
checkpoint, not reproducible CI acceptance or proof of all failure scenarios.
No live pa store was adopted. The user explicitly deferred live adoption and
sync cutover; that work remains separate from implementation and preview release.

All unchecked milestones in implementation-plan.md remain part of the goal.
In particular, do not mistake helper-level recovery tests for the public
doctor/recovery workflow or process-kill crash acceptance.

Known review items to resolve before acceptance:

- Real kill/power-loss tests must cover temporary-file cleanup, directory fsync
  chains, partial receipt writes, and backup publication. Hook-return tests
  exercise state-machine phases but allow Go defers to run.
- Recovery needs explicit stale-lock ownership/PID verification, public typed
  results, and correct pre-mutation versus applied-state status reporting.
- Git inspection uses GIT_OPTIONAL_LOCKS=0 after a test demonstrated that status
  could rewrite the index during adoption dry-run. Validate this with complete
  before/after store snapshots and shell-pa fixtures.
- Plugin missing-executable and noninteractive interaction errors need precise
  classification and mock-plugin tests. Encrypted SSH identity unlocking is
  still pending.
- The 64 MiB entry limit is an explicit provisional implementation bound;
  document and test limits consistently across CRUD, bundles, and recovery.
- Human formatting currently uses structured development output for control
  commands. Guided add/edit input is implemented; routine confirmations and
  other guided workflows remain pending.
- Untracked deletion acknowledgements and retained transactional backups need
  a precise documented recovery boundary, consistent with the locked spec.
- No release tag is authorized by merely passing the current subset of tests.

## Continuity

Root thread: `01a0720b-6a4e-77d1-b72c-165a680e14b8`.
Original design thread consulted: `019fba52-6d8f-70e0-94bc-bc4e3e72d400`.
The original session confirmed that remaining mechanics are implementation
choices, not reasons to reopen locked product decisions. The user reaffirmed
Fulla's canonical name on 2026-09-05 and left for a swim with the goal active.

## Expert and shell surfaces

`fulla git -- ...` runs system Git in the encrypted repository while holding the
shared store lock. Its stdin, stdout, stderr, and child exit status pass through;
`--json` is rejected. Explicit Git aliases/configuration remain trusted expert
code. Regression coverage observes the held lock from the real Git child and
checks that child failure releases it without adding Fulla output.

`fulla completion bash|zsh|fish` generates command/group and global-option
completion without opening a store. Source the generated script in the chosen
shell (for example, `source <(fulla completion bash)`). Generation rejects JSON.
Bash behavior and Bash/Zsh syntax were checked locally; Fish is unavailable on
this host and its syntax/runtime acceptance remains outstanding.

## Interactive write checkpoint

Interactive `add NAME` offers generated input, hidden terminal entry, or the
configured editor. `edit NAME` opens the current exact value in the editor.
Explicit stdin/descriptor/generation paths and JSON remain noninteractive.
Prompts use the controlling terminal, leaving stdin untouched. The shared lock
covers the interactive write, including the editor's lifetime, so competing
cooperating writers cannot invalidate the edited original.

Editor configuration uses the TOML argv array, then standard VISUAL/EDITOR
trusted shell commands, then vi. Temporary material uses a 0700 directory and
0600 file, preferring Linux /dev/shm when TMPDIR is not selected. Fulla rejects
symlink, hard-link, permissive, and oversized editor output. It removes the
private directory (including editor-created backups inside it) on success,
failure, and handled interrupt. Editor activity outside this directory remains
inside the documented trusted-editor boundary. Uncatchable termination cannot
run cleanup.

`python3 scripts/acceptance-interactive.py dist/fulla` passed locally with real
pseudo-terminals and is now a Linux/macOS CI gate. It verifies all three add
choices, binary edit preservation, independence from stdin, hidden input,
SIGTERM during an editor that changes terminal mode, Ctrl-C during secret
entry, restored terminal state, and removal of plaintext files/shared locks.
The Go race suite also exercises competing writes, failed-edit preservation,
editor output rejection, and cancellation cleanup. These are fixture proofs;
live credential stores remain untouched.

Hosted receipt for the interactive checkpoint at `a5ef9a9`:
https://github.com/agensfield/fulla/actions/runs/33979798139. Both Linux and
macOS passed the full race suite, static analysis, controlling-terminal
acceptance, and four-platform builds. The Python acceptance harness also
passes Ruff and basedpyright without warnings.

## Clipboard checkpoint

`copy NAME` (`clip`) writes UTF-8 text without NUL and verifies the clipboard's
exact byte digest before reporting success. Native macOS pbcopy/pbpaste,
Wayland wl-copy/wl-paste, and X11 xclip are optional system integrations. Fulla
rejects macOS RTF/PostScript header inputs before pbcopy can reinterpret them;
`show` remains the exact-byte surface for those values and arbitrary binary data.

The default 45-second expiry uses a fresh detached Fulla process. Its private
stdin request contains only the digest, deadline, and backend command paths.
The worker does not open a store or inherit decrypted memory, and subprocess
environments contain only desktop/locale variables. Clipboard data at expiry
is streamed into a digest; a different digest prevents clearing. Config
`clipboard.clear_after`, `--clear-after`, and `--no-clear` control expiry.
Scheduling is not a guarantee against logout, worker termination, or desktop
failure. Clipboard managers remain outside Fulla's control.

The platform tools expose inspection and clearing as separate operations, not
an atomic compare-and-swap. A concurrent replacement in that narrow interval
can race; this remains a limitation to reconcile in the final security audit.
The implementation does not claim atomic clipboard ownership.

Fixtures verify exact UTF-8/newlines, rejection before mutation, post-write
failure status 3, secret-free envelopes/diagnostics, filtered child environments,
worker completion, and preservation of replacement values. The CLI harness is
`scripts/acceptance-clipboard.py`; it defaults to fixture utilities. Its explicit
real-clipboard mode is restricted to disposable GitHub runners. CI now includes
an isolated X11 display and the macOS runner's real pasteboard in addition to
fixture tests. Local tests never accessed the user's clipboard.
