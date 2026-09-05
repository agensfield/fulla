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
- Plugin missing-executable classification and inert inspection are implemented.
  Noninteractive interaction errors, working mock-plugin round trips, and
  encrypted SSH identity unlocking remain pending.
- The 64 MiB entry limit is an explicit provisional implementation bound;
  document and test limits consistently across CRUD, bundles, and recovery.
- Human formatting currently uses structured development output for control
  commands. Guided add/edit and initialization/adoption confirmation are implemented;
  other routine confirmations and guided workflows remain pending.
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

Hosted clipboard receipt at `4379fc8`:
https://github.com/agensfield/fulla/actions/runs/33980733730. The real X11 and
macOS clipboard jobs passed, alongside fixture/terminal acceptance, race tests,
static analysis, and four-platform builds. Wayland currently has fixture and
upstream-interface coverage; real compositor acceptance is still outstanding.

## Backup retention checkpoint

`backup prune --keep N` and/or `--older-than DURATION` select retained snapshots
explicitly. Invocation previews by default; `--yes` applies the selection, and
`--dry-run` always previews. Combined criteria intersect: a snapshot must be
outside the newest N and older than the selected duration. `--keep 0` explicitly
selects all snapshots unless an age criterion narrows it. No criteria is an
invalid invocation, and there is no background expiry. Example: preview with
`fulla backup prune --keep 10 --older-than 720h --json`, then apply the same
criteria with `--yes`.

Pruning journals the complete selected backup IDs outside the snapshots before
unlinking anything. The shared lock remains held through deletion and receipt
publication. `doctor --recover-lock TOKEN` resumes a dead owner's pruning,
including a snapshot whose own journal was already unlinked. A conflicting
combination of pending operation journals fails closed. Receipts remain after
pruning; live entries, Git history, identities, and peer pins are unchanged.

Tests compare the complete fixture tree before/after preview, preserve exact
live entry bytes, verify count/age intersection, and kill actual subprocesses
at preparation, deletion, and receipt boundaries. A partial-snapshot fixture
covers recovery after loss of the deleted snapshot's own journal. Backup sorting
now compares parsed instants, including differing fractional-second precision.
`status` reports retained backup count, total bytes, oldest/newest timestamps,
and oldest age in seconds.

Doctor now includes backup statistics and peer-registry validation in structural
inspection. An unhealthy report returns status 1 with `doctor.unhealthy` and a
structured report in JSON error details; human diagnostics show the issues and
lock ownership evidence. Deep verification refuses pending operations even when
an orphaned journal has no lock directory. Explicit permission repair and private reports are implemented below; broader
permission and diagnostic acceptance remains in progress.

Retention/doctor hosted receipt at `f5e7ddf`:
https://github.com/agensfield/fulla/actions/runs/33981694235. Both Linux and
macOS passed the full race suite (including killed-process pruning recovery),
static analysis, terminal and real clipboard acceptance, and cross-builds.

## Plugin diagnostics and configuration provenance

Doctor parses identity/recipient configuration and checks plugin executable
availability without executing plugins or decrypting entries. Results include
selected store/config paths and setting provenance, including explicitly set
default values and editor environment fallback. Missing plugin executables have
typed errors that survive recipient-consistency checks.

Fulla rejects plugin operations when `AGEDEBUG=plugin`: the upstream debug
transport writes plugin protocol material, including keys, to stderr. Regression
fixtures prove inspection and debug rejection never launch the executable and
that diagnostics omit private identities and entry values. Native-only age
operations remain available. The full local race suite and `go vet ./...`
passed for this checkpoint; hosted verification follows the push.

## Code review debt

Repeated manual lock-release/error paths need a contract review for ownership
and applied-state reporting. The central CLI dispatcher is approaching 500 lines;
flag applicability, help, and typed output consistency need consolidation as the
remaining surfaces land. Full before/after transaction snapshots also need
scaling measurements. These are explicit review items, not claims that passing
unit tests establishes security or release readiness.

## Private diagnostic reports

`fulla doctor --report /existing/private-directory/report.json` explicitly
publishes a new 0600 JSON file, using atomic no-replace publication outside the
store. Existing destinations, symlinks, missing parents, and stdout (`-`) are
rejected. The destination is checked before deep inspection. `--deep` may be
combined with reporting; `--recover-lock` may not.

The versioned `fulla.diagnostic/v1` artifact contains aggregate counts, health,
completion, Git/lock presence, identity/recipient validity, and deduplicated issue
codes. It deliberately excludes entry names, filesystem paths, plugin locations,
lock tokens, raw error text/details, secret values, and private identities.
Unhealthy inspection still publishes its report and returns a nonzero status.
Missing stores and store-open failures (including unsafe file/root modes) also
produce an incomplete report with the redacted error code. Tests verify that
inspection preserves unsafe modes and identity bytes. Configuration-resolution
failures still precede report creation because the selected store boundary is
not yet known; this remains part of diagnostic workflow follow-through.

CLI fixtures cover healthy deep inspection, unhealthy header inspection,
redaction, 0600 mode, no replacement, unsafe destinations, and incompatible
recovery flags. The preceding plugin diagnostics checkpoint passed hosted CI:
https://github.com/agensfield/fulla/actions/runs/33982555589 (`b0f35ec`).

## Explicit permission repair

`doctor --fix-permissions` preflights the entire selected Fulla store, acquires
the existing shared lock, and repeats preflight before changing ordinary POSIX
mode bits. It removes group/other permissions while preserving regular-file
owner bits, and restores directory owner access when the directory is still
inspectable. It does not read identity or entry contents. A receipt records
changes performed in this invocation; applied failures carry repair progress.

Native descriptor-based ACL inspection uses Darwin fgetattrlist and Linux POSIX
ACL xattrs. Repair refuses all ACL-bearing objects, unknown ACL-inspection
failures, foreign owners, special modes/types, symlinks, hard links, and directories
writable by other users. These require explicit manual review; Fulla never
rewrites their ACLs. Unreadable objects cannot be automatically repaired without
first being inspectable. Ordinary commands enforce private POSIX modes and reject ACL-bearing private
paths rather than interpreting custom rules. ACLs remain untouched.

Repair refuses another writer or a pending transaction. For a killed permission
repair owner, inspect with `doctor`, then explicitly use
`doctor --fix-permissions --recover-lock TOKEN`. Recovery requires a matching
token, dead local PID, permission-repair operation, and no other pending journal;
it repeats permission preflight and uses the existing recovery flock. This path
cannot be used to bypass private-mode checks for unrelated transaction recovery.
`--deep` and `--report` cannot be mixed with permission repair.

Fixtures prove exact identity/ciphertext/Git-config preservation, idempotence,
shared-lock exclusion, complete preflight before mutation, native ACL refusal,
and real SIGKILL recovery after a partial mode-repair sequence. The killed fixture
uses the production lock and mode-application primitives; it does not yet cover
every kill boundary in the complete public command.

The earlier status-only initialization failure at `d2f5752` was initially
unexplained; see the later Git maintenance race investigation below.
`eb60c09` improved failure diagnostics and added 20 repeated CI journeys per OS;
both hosted jobs passed (run 33983151661). Local runs of 20 repetitions with
Go 1.26.0 and 100 with Go 1.27.1 also passed. This is reproduction evidence,
not a demonstrated root-cause fix.


## Ordinary private-path ACL boundary

Normal rooted opening, whole-store validation, and bounded private reads now
inspect ACLs through the same native descriptor APIs. A private-looking `0600`
mode alone is insufficient: an ACL-bearing object is rejected before reading
its contents, even when its current ACL is restrictive. Fulla does not attempt
to normalize or evaluate arbitrary custom ACL policy. Fixture tests explicitly
retain ACLs alongside `0600` modes, then verify refusal and ACL preservation.

Hosted permission-repair acceptance at `895f0e9` passed on Linux and macOS:
https://github.com/agensfield/fulla/actions/runs/33983935870. This includes native
ACL fixtures, killed-owner recovery, full race/static gates, terminal and real
clipboard acceptance, repeated initialization journeys, and cross-builds.


## Deep diagnostic outcomes

After identity/recipient preflight, `doctor --deep` now attempts every inventoried
entry and reports each name, success, and a typed error code. An entry failure
does not hide later outcomes. The report is unhealthy and the CLI returns status
1 when any entry fails; private diagnostic artifacts retain aggregate issue
codes rather than entry names. Successful plaintext buffers are cleared after
verification. Structural doctor remains non-decrypting, and mutation preflight
continues to use the existing fail-fast verifier.

A fixture with valid age headers but corrupted first/last payloads proves that
structural inspection does not decrypt, while deep inspection reports both
failures and the valid middle entry without exposing values/private identities.
Identity/configuration preflight failures still stop the entry pass explicitly.


## Git maintenance race: reproduced and fixed

macOS CI run 33984127042 failed during initialization with
`chmodat passwords/.git/objects/maintenance.lock: no such file or directory`.
Git's detached automatic maintenance was changing the object directory while
Fulla normalized the newly created repository's modes. This is a concrete
product concurrency bug, not a proved runner fault. It plausibly explains the
earlier status-only initialization failure, whose missing details prevent exact
retrospective attribution.

Commit `6187bf4` disables `maintenance.auto` and `gc.auto` for internal Git
commands so maintenance cannot outlive their lock/staging scope. It does not
change persistent user Git configuration or the explicit expert passthrough.
A real Git trace2 regression includes a positive control, fails on the old
implementation, and passes with the fix: Fulla initialization and writes no
longer spawn automatic maintenance children. Externally scheduled maintenance
or an unrelated process ignoring the store lock remains outside this control.

References: [observed CI failure](https://github.com/agensfield/fulla/actions/runs/33984127042),
[Git maintenance settings](https://git-scm.com/docs/git-config#Documentation/git-config.txt-maintenanceauto).


## Initialization and adoption confirmation

Interactive `init` now runs preflight and asks through the controlling terminal,
showing the quoted target path and Git history's retention of encrypted versions,
entry names, and change times. `--no-git` explains that transactional backups
still remain. `init --adopt` confirms in-place adoption and preservation of the
existing identity/entries. Application repeats validation after confirmation.

An empty answer or no declines without mutation; handled interruption restores
the terminal and leaves no store/staging material. JSON and `--non-interactive`
never prompt and require `--yes`; `--dry-run` remains read-only without prompting.
The common terminal lifecycle is shared with interactive add/edit. `--yes` is
only routine authority and does not replace fingerprints or scoped destructive
acknowledgements. Other command domains still need their complete guided flows.

Real PTY acceptance now covers Git/no-Git initialization, cancellation/default-no,
SIGTERM cleanup, explicit noninteractive refusal, adoption decline/acceptance
with exact identity preservation, and rejection of an existing destination
without prompting. Existing hidden-input/editor/signal acceptance also passes.
The Python harness passes Ruff and basedpyright. Deep diagnostics at `377ed7e`
passed hosted Linux/macOS CI: https://github.com/agensfield/fulla/actions/runs/33984495416.


## Historical-entry restore confirmation

`history restore COMMIT NAME` now verifies the selected historical ciphertext
and prepares a verified replacement under the active recipient before human
confirmation. The shared lock covers preflight and the controlling-terminal
prompt, which receives only the full commit, entry name, and whether the entry
is being replaced or recreated. It never displays either value. Application uses
the existing journaled mutation/backup path and records a history-restore command.

Decline or handled interruption releases the lock without changing entry bytes,
Git history, or backup count. JSON/`--non-interactive` requires explicit `--yes`;
that route performs the same locked preflight. Restoration uses retired recovery
keys when needed and does not require decrypting a damaged current entry.
History show/restore refuses an untracked store instead of allowing system Git
to discover an unrelated ancestor repository.

Store fixtures prove lock exclusion, cancellation preservation, exact binary
restoration, and missing-entry recovery. Real PTY acceptance covers decline,
SIGTERM, no value disclosure, replacement/recreation wording, and prompt-free
agent refusal. The Python harness passes Ruff/basedpyright. Initialization at
`f048673` passed hosted CI: https://github.com/agensfield/fulla/actions/runs/33984832320.


## Snapshot restore confirmation

`backup restore ID --phase before|after` prepares a sorted add/replace/remove
plan while holding the shared store lock, then asks through the controlling
terminal. The prompt contains names only. Decline or handled interruption
releases the lock without changing values or creating a transaction snapshot.
JSON/noninteractive use requires `--yes` and performs the same locked preflight.
Application retains the displaced entry set through the journaled backup path.

Preflight validates the active identity/recipient pair even for an empty selected
snapshot, preventing removal of live entries when current keys are inconsistent.
Selected historical values are re-encrypted and verified under the active identity;
plaintext buffers are cleared after use. Snapshot lookup now occurs under the
lock so a concurrent prune cannot invalidate the selected backup.

Store tests cover exact binary restoration, the three plan categories, competing
writer exclusion, cancellation preservation, and mismatched keys with an empty
snapshot. Real PTY acceptance covers decline, SIGTERM, successful removal of an
extra entry, exact-byte restore, and prompt-free agent refusal. Go 1.26.0 full
race tests and vet, Ruff, basedpyright, and PTY acceptance passed locally.
Historical restore at `ff1b7ec` passed hosted Linux/macOS CI:
https://github.com/agensfield/fulla/actions/runs/33985186278.

Remaining recovery acceptance includes the complete crash matrix, full-disaster
guided flow, and source provenance/retired-key lifecycle review. This checkpoint
does not establish complete recovery or release acceptance.


## Rotation staging cleanup ordering

Rotation staging includes the newly generated private identity. Finalization
previously removed the recovery journal before deleting staging, leaving a crash
window in which an untracked key copy could survive a later destructive rotation.
Finalization now removes staging and syncs its parent directory before removing
the journal. The retained `retired` phase can replay without staging.

The rotation failure-injection fixture now interrupts after staging cleanup,
asserts the private staging directory is absent while the recovery journal remains,
and completes recovery with intact live values. This is deterministic state-machine
coverage, not a simulated power-loss or complete killed-process acceptance matrix.
Pre-journal staging crashes and previously orphaned staging remain separate audit
items; this change closes the identified finalization ordering window.

Go 1.26.0 targeted rotation tests, the full race suite, and vet passed. Snapshot
restore checkpoint `7dc38fe` passed hosted Linux/macOS CI:
https://github.com/agensfield/fulla/actions/runs/33985705977.


## Killed-process rotation acceptance

`TestRotationKilledOwnerRecovery` kills a real child process at five boundaries:
publication of an entry, recipients, identities, the Git commit, and completed
staging cleanup. Each boundary runs in continuity and explicit destructive mode
against generated Git-backed stores. Recovery uses `Store.Recover` with the
inspected owner token, rather than invoking journal finalization directly.

The ten cases assert refusal to displace a live owner or accept a wrong token,
ordinary-read refusal after interruption, exact binary live-value preservation,
old-history accessibility only in continuity mode, no remaining private staging,
and applied/destruction fields in the recovery receipt. The targeted Go 1.26.0
race run and vet passed. The standard CI race suite includes these tests on both
platforms. This extends process-death acceptance; it does not simulate power loss
or cover every syscall, pre-journal boundary, or interrupted recovery invocation.

The cleanup-order fix at `dacdceb` passed hosted Linux/macOS CI:
https://github.com/agensfield/fulla/actions/runs/33985877145.


## Real shell-pa adoption and rollback

The pinned predecessor `ardasevinc/pa@f75734b8775f72d5d2f9630c08c2b48bdb6d8104`
creates `passwords/.gitattributes`. A real shell-created fixture exposed that
Fulla rejected this as an unexpected entry during adoption preflight. Inventory
now recognizes that exact root metadata path, and snapshot entry-set restoration
skips its saved copy while preserving current Git configuration. Other unexpected
files, including nested `.gitattributes`, still fail entry validation.

`scripts/acceptance-pa.py` extracts the pinned shell implementation into a private
temporary home and creates its store with the official age 1.3.2 command tools.
It verifies read-only adoption preview, byte-for-byte preservation of predecessor
files during adoption, alternating shell-pa/Fulla reads and writes (empty, newline,
NUL, invalid UTF-8, and 256 KiB values), moves/removal, shared-lock writer refusal,
and shell-only CRUD after ceasing Fulla use. Identities and recipients remain
unchanged. Only generated fixture values enter stdin; no ambient secret-value
variables, installed user stores, or user Git configuration are used.

Local real-shell acceptance passed. CI now checks out the exact public predecessor
and builds age tools into `dist/pa-tools`, then runs the same drill on Linux/macOS.
Go fixtures additionally check snapshot restore preserves `.gitattributes` and
exact bytes while rejecting unrelated metadata paths. Full sync activation and
rollback after synchronization remain separate acceptance work; this fixture does
not claim the entire pa-adoption journey. Imported/custom Git configuration trust
and filter behavior remain part of the broader security review.

The full Go 1.26.0 race suite and vet passed, along with Ruff/basedpyright for
the new harness. The preceding killed-rotation checkpoint `486618c` passed
hosted Linux/macOS CI: https://github.com/agensfield/fulla/actions/runs/33986026518.
