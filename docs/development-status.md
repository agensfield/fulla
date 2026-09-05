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
  commands; guided TTY parity and the full command hierarchy remain pending.
- Untracked deletion acknowledgements and retained transactional backups need
  a precise documented recovery boundary, consistent with the locked spec.
- No release tag is authorized by merely passing the current subset of tests.

## Continuity

Root thread: `01a0720b-6a4e-77d1-b72c-165a680e14b8`.
Original design thread consulted: `019fba52-6d8f-70e0-94bc-bc4e3e72d400`.
The original session confirmed that remaining mechanics are implementation
choices, not reasons to reopen locked product decisions. The user reaffirmed
Fulla's canonical name on 2026-09-05 and left for a swim with the goal active.
