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
- Plugin missing-executable classification, inert inspection, real mock-plugin
  round trips, terminal interaction, and encrypted OpenSSH identity unlocking
  are implemented. Hardware-touch wait/timeout policy remains open.
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


## Git ciphertext conversion preflight

An isolated real-Git probe configured a clean filter that replaced an entry with
a fixture marker during `git add`. Fulla returned success and its live ciphertext
remained readable, but the committed blob was not age ciphertext and historical
restore failed. This was a history-integrity bug, not merely an untested feature.

Internal Git status now follows non-executing attribute inspection of existing
entries and pa's root `.gitattributes`; transaction preflight also checks future
entry names before publication. Converting attributes fail with
`git.conversion_unsupported`. Filter and working-tree-encoding declarations are
conservatively rejected, including explicit unset forms because Git's text
report cannot distinguish those from a literal value named `unset`. Text/EOL/ident
conversion settings are refused; `-text` and pa's `diff=age` remain supported.
Internal Git disables implicit `core.autocrlf` conversion without editing user
configuration. Explicit expert Git remains under the caller's control.

Regression fixtures assert no filter subprocess runs, no live entry or Git head
changes, no backup is created, and locks are released for both new and existing
names. Attribute cases include `filter=unset` and `filter=unspecified` so ambiguous
output does not bypass the guard. This does not claim isolation from same-account
processes changing Git configuration concurrently.

The check uses Git's own attribute precedence and NUL-delimited output:
https://git-scm.com/docs/git-check-attr. The prior real-pa checkpoint `a731083`
passed hosted Linux/macOS CI:
https://github.com/agensfield/fulla/actions/runs/33986364480.

Commit also repeats the conversion check before staging, so resumed recovery
fails closed if conversion rules changed during interruption. Local Go 1.26.0
full race/vet and real shell-pa acceptance passed; focused conversion and killed
rotation recovery tests passed after adding this final commit-time check.


## Killed multi-entry transaction recovery

`TestKilledTransactionRecoveryAndChangedGitFilter` kills a child during a
two-entry delete/add transaction after either entry publication, Git commit, or
receipt publication, with and without Git (eight cases). Recovery uses the public
store recovery API and inspected stale-owner tokens. Fixtures assert live-owner
refusal, ordinary-read refusal during interruption, exact destination bytes,
source removal, clean Git state, and retained committed backup/receipt evidence.

One case changes Git conversion rules after the transaction owner dies. A separate
recovery process fails without executing the configured filter or changing Git
HEAD, exits, and leaves recoverable ownership. Removing the fixture conversion
rule and reinspecting the new owner token allows a subsequent recovery to finish.
This proves retry after a failed recovery invocation, rather than only calling
journal-finalization helpers directly. The targeted Go 1.26.0 race run and vet
passed. These tests run in the standard hosted race suite.

Coverage remains bounded to these publication seams. Pre-journal process death,
power-loss durability, arbitrary syscall interruption, and all other workflow
boundaries still require their own acceptance evidence.


## Working plugin fixture and typed interaction boundary

A real mock-plugin subprocess exposed that Fulla supplied a nil `ClientUI` to
age's plugin client. The first valid plugin response triggered a nil-pointer
panic in `ClientUI.readStanza`; previous missing-plugin fixtures never reached
that path. Parsed plugin objects now receive a non-nil UI with noninteractive
input/confirmation refusal. Informational plugin messages produce no unsolicited
output when no display callback is supplied.

The official client translates callback errors into protocol failures, so Fulla
retains redacted per-operation interaction state around plugin Wrap/Unwrap.
Missing input/confirmation becomes `interaction.required`, including during
recipient consistency verification; supplied callback failure becomes
`plugin.interaction_failed`. Reused objects reset this state and serialize their
operations. Recipient labels continue through the official WrapWithLabels API.
Missing-plugin inspection and the AGEDEBUG guard recognize the wrapped objects.

`plugin_fixture_test.go` launches the test binary as `age-plugin-fullafixture`,
uses native X25519 for actual file-key protection, and speaks the official plugin
protocol. Fixtures cover exact binary round-trip, consistency verification, PIN
and confirmation refusal, successful reuse after refusal, explicit UI-supplied
PIN input, and redacted callback errors. Only generated keys and values are used.

This proves the core plugin boundary. Controlling-terminal PIN UI, hardware-touch
timeout behavior, hardware integration, and broader CLI acceptance remain open.
The preceding killed-transaction checkpoint `73e682d` passed hosted Linux/macOS
CI: https://github.com/agensfield/fulla/actions/runs/33986961005.

The full local Go 1.26.0 race suite and vet passed, including the working mock,
existing missing-plugin/debug-mode fixtures, and native crypto/store tests.


## Controlling-terminal plugin interaction

Human CLI operations now supply plugin UI callbacks when opening stores, adopting
pa stores, running diagnostics/recovery, and parsing explicit transfer identities
or recipients. PIN and public-input requests use the existing hidden, cancellable
controlling-terminal reader. Plugin names, prompts, messages, and choice labels
are quoted to prevent terminal control sequences from being interpreted.

JSON, noninteractive, and remote protocol modes receive no interactive callbacks.
Plugin confirmation uses an independent default-no prompt; routine `--yes` does
not approve it. Callback errors are redacted while preserving interaction-required
and cancellation classification. Handled prepublication signals retain their exit
status through the crypto boundary and release the store lock. Applied-state
failures still follow the transaction recovery contract.

Real PTY acceptance now uses the compiled mock plugin to prove hidden PIN entry,
successful raw show, no prompting in JSON/noninteractive mode, SIGTERM before
publication with unchanged ciphertext/no lock, and explicit plugin confirmation
acceptance/refusal despite `--yes`. Ruff and basedpyright pass. To run the harness:

```sh
go build -o dist/fulla .
go test -c -o dist/age-plugin-fullafixture ./internal/crypt
python3 scripts/acceptance-interactive.py dist/fulla
```

CI compiles that test-only plugin before PTY acceptance. Hardware-touch timeout
policy, hardware integration, and complete plugin-backed disaster restore remain
open. The prior core plugin fix `0901368` passed Linux/macOS CI:
https://github.com/agensfield/fulla/actions/runs/33987347981.

The full Go 1.26.0 race suite and vet passed. After refining informational-message
handling, CLI/crypto race tests, vet, Ruff/basedpyright, and the full PTY harness
passed again. The mock emits an ANSI control sequence to verify quoted rendering;
nonterminal commands continue when only an informational message is requested.


## Recovery streams now share plugin policy

Source review while tracing full-restore UI found five direct age calls in full
archives and logical transfers. Those paths bypassed the entry crypto boundary's
AGEDEBUG plugin guard and replaced actionable plugin errors with generic recovery
errors. They now use shared `crypt.EncryptStream` / `crypt.DecryptStream` helpers;
entry encryption/decryption uses those helpers too.

Plugin policy runs before header wrapping/unwrapping. Callers retain their existing
payload limits, authenticated-EOF requirements, and publication sequencing.
Archive/bundle-specific ordinary error codes are preserved, while plugin and
interaction errors (including cancellation) remain actionable and redacted.
The only remaining direct production age decryption outside this boundary is
structural doctor with an inert identity that cannot invoke plugins or decrypt.

Fixtures cover logical export/verify/import and full export/restore with a
marker-writing mock executable. All five refuse AGEDEBUG=plugin before execution
or artifact/target publication and release acquired locks. Valid age ciphertext
is supplied to the read paths so the test does not rely on malformed-header
rejection. Crypto fixtures continue exercising the working real plugin protocol.

This fixes the shared streaming policy gap. Passing a human UI into validation
of a restored plugin-backed store, guided full-restore confirmation, and hardware
touch/timeouts remain open. Terminal checkpoint `97ad593` passed hosted CI:
https://github.com/agensfield/fulla/actions/runs/33987854529.

The full local Go 1.26.0 race suite and vet passed, including recovery streams,
working plugin fixtures, and native archive/bundle authentication tests.


## Full restore confirmation and plugin-backed validation

Full-state restore now supplies the controlling-terminal plugin UI when opening
and deeply verifying private staging. After complete archive authentication,
Git conversion checks, and entry verification, human callers receive a default-no
publication prompt with file/byte counts and the identity/peer-authority cloning
warning. JSON and noninteractive callers still require explicit --yes; that flag
does not bypass plugin interaction policy.

Deep verification preserves plugin cancellation and interaction errors for each
entry, and clears discarded plaintext. Store tests cover absent and empty target
cancellation, occupied-during-confirmation refusal, staging cleanup, and corrupt
archives rejected before confirmation. Real PTY acceptance restores a working
plugin-backed store, checks machine-mode refusal without prompting, cancels
while decrypting an entry and at final confirmation, declines with empty input,
then accepts and verifies the restored value and identity. The terminal harness
now consumes matched prompts so repeated PIN steps cannot match stale output.

Local PTY acceptance and focused archive race tests passed. Hardware touch and
wait/timeout policy, abrupt-death staging recovery, and release acceptance remain
open. Earlier stream checkpoint 2eaecf4 passed Linux/macOS hosted CI run 33988306509.

The complete local Go 1.26.0 race suite and vet also passed for this checkpoint.


## Encrypted OpenSSH identity unlocking

Encrypted OpenSSH Ed25519 and RSA identities now use the embedded official
agessh.EncryptedSSHIdentity implementation. Parsing and structural doctor do not
unlock keys. The library checks the public recipient stanza before requesting a
passphrase, and caches the unlocked key only in the parsed in-process identity.
Fulla serializes reuse, owns a copy of the encrypted input, and clears callback
passphrase buffers after success or failure. It does not write unlocked keys.

The identity UI explicitly separates built-in SSH unlocking from external plugin
callbacks. Human commands use hidden controlling-terminal input, independent of
stdin. Machine/remote modes supply no input callback and fail with
interaction.required when unlocking is needed. --yes does not supply a
passphrase. Incorrect passphrases and callback failures are redacted as
identity.unlock_failed; handled terminal signals retain their exit status through
identity.unlock_cancelled, including entry and archive verification paths.

Unit tests cover Ed25519/RSA, parse-only behavior, recipient mismatch without a
prompt, wrong-passphrase retry, cached unlocking, caller input-buffer clearing,
passphrase disposal, and cancellation/error redaction. Real PTY acceptance covers
creation with stdin independent of the terminal, structural doctor, machine-mode
refusal, signal cancellation with unchanged ciphertext and released lock, a wrong
passphrase, and successful read. No live SSH identities are used.

Legacy encrypted PEM without an embedded public key is explicitly unsupported;
Fulla does not guess or read adjacent .pub files. There is no passphrase option in
argv or ambient environment and no new dependency. Hardware-specific plugin
acceptance and bounded wait/cancellation policy remain open.

Local Go 1.26.0 full race suite and vet passed; final encrypted SSH unit tests
also passed with race detection. PTY acceptance, Ruff, and basedpyright passed.


## Controlling-terminal recovery passphrases

The scoped recovery journey explicitly requires a strong passphrase obtained
from the controlling TTY or an inherited descriptor. Only descriptors were
previously implemented. --passphrase now selects hidden terminal input for
logical transfer export/verify/import and full-state archive export/restore.
Export requires two matching entries; reads request one. The existing 20-4096
byte bound also applies to terminal input. --passphrase accepts no value, and
conflicting recipient/identity/descriptor sources fail before input. Machine mode
returns interaction.required instead of prompting. Snapshot restore rejects
archive-only identity/passphrase options rather than silently ignoring them.

Passphrase descriptor buffers are cleared after conversion to the string needed
by age's scrypt API. Terminal input clears its accumulated bytes on failure and
removed bytes on backspace. This is best-effort buffer hygiene, not a claim of
eliminating all Go/library/OS memory copies.

The PTY recovery fixture selects one exact manifest entry, exports with confirmed
terminal protection, verifies without creating a live store, imports into an
isolated store, and compares exact binary bytes and the one-entry inventory.
It also covers short input, confirmation mismatch, SIGTERM, conflicting sources,
machine-mode refusal, absence of output on failure, and hidden passphrases.

Plugin wait investigation remains separate: age v1.3.2 ClientUI.WaitTimer is a
notification callback without cancellation; its clientConnection.Close waits for
the child after SIGINT without a deadline. A timer around an application goroutine
would leave the process running and does not solve this lifecycle gap. No such
workaround or timeout-completion claim has been added.

The final PTY run also passed full-archive passphrase export/restore with exact
bytes and original identity preserved. Local full Go race suite, final CLI race
tests, vet, Ruff, and basedpyright passed.


## One-direction sync commit, lost acknowledgement, and retry

The locked sync contract requires exact partial-state reporting and retained
receipts. Sync previously returned status 3 with observed state but exited before
writing its sync receipt. It now attempts a private, locked receipt for partial
outcomes. The receipt records outcome=partial and the same push/pull/activation
and remote-uncertainty fields returned to the caller. Receipt lock/publication
failure preserves status 3 and reports sync.receipt_failed in receipt_error.
Receipt paths are returned only after publication succeeds, rather than being
advertised before acquiring the receipt lock. Completed receipts carry
outcome=completed; existing fields and version remain unchanged.

Production client/server tests for both current and immediately previous wire
versions drop either the import acknowledgement after the remote transaction
commits, or the export reply after acknowledged push. They prove status 3,
correct remote uncertainty, no local pull publication, released locks, persisted
partial evidence, safe retry without re-encrypting the previously committed
remote entry, exact binary/empty bytes, and divergent shared-name preservation.
A contention fixture holds a real local store lock at the lost-acknowledgement
boundary and proves receipt failure does not obscure the remote commit.

These are in-process transport-disconnect fixtures using the production RPC
and real stores, not a new physical two-host or power-loss claim. The existing
manual Mac/devbox drill remains separate. Restart during receipt publication,
further adversarial protocol cases, and full acceptance remain open.

The full local Go 1.26.0 race suite and vet passed, including all five new
partial-commit/receipt cases.


## Strict challenge transcripts and captured-proof replay

Challenge validation used a single json.Decoder.Decode with unknown-field
rejection. A failing regression demonstrated acceptance of duplicate fields,
a second JSON document, and trailing non-JSON garbage. It now uses the existing
StrictJSON boundary, retaining protocol/issuer/responder/32-byte nonce checks.
The three previously accepted cases now fail with peer.authentication_failed.

Protocol tests for current and immediately previous versions obtain a real
server challenge, prove successful mutual authentication, then reuse the proof
in the authenticated session and replay it in a new session. Both are rejected.
Complete file-content inventories of both stores remain unchanged, including
a remote exact-byte entry. Primitive tests also cover nonce length, identity and
protocol binding, fresh-session mismatch, unknown fields, and proof expiration
using an already-expired start time without sleeping.

Replay refusal already worked before the parser fix. This checkpoint adds
explicit evidence and closes transcript parsing ambiguity; it does not claim a
previous authentication bypass or a physical-network adversarial audit. Previous
partial-sync checkpoint c4322c1 passed Linux/macOS CI run 33989879961.

The complete local Go 1.26.0 race suite and vet passed, including the final
same-session and fresh-session replay cases.


## Live metadata-domain validation

An opened Store cached its domain-version map and RequireDomain consulted only
that snapshot. A failing regression changed the persisted peers domain to v2
and then successfully wrote a v1 peer through the old handle. RequireDomain now
reads and validates the current store manifest, using the same decoder as Open.
Repeated checks inside peer mutation locks therefore observe a later upgrade.
It does not mutate the shared cached Metadata object or rewrite the manifest.

The fixture matrix covers all five domains (peers, sync, backup, identity,
transactions) with absent, invalid negative, current-v1, and future-v2 versions.
Unsupported domains fail individually while independent supported domains remain
available. Duplicate/trailing JSON and future root versions are rejected by both
fresh and already-open handles. The peer-upgrade regression verifies no peer
publication and no retained lock. Ordinary adoption fixtures still pass.

This is domain-check coverage, not proof of the complete migration program.
No previous released Fulla domain format exists yet; legacy shell-pa adoption
is exercised separately. Safe older-binary CRUD alongside newer backup domains
needs a complete policy/acceptance audit because CRUD creates transactional
backup snapshots. Interrupted domain upgrades and every cross-domain mutation
boundary remain open rather than being inferred from the version-check matrix.

The complete local Go 1.26.0 race suite and vet passed, including the live
manifest regression and 20 domain-version combinations.


## Confirmed peer removal and applied receipts

Human peer removal now confirms the current saved name, host, and fingerprint
with a default-no controlling-terminal prompt explaining local authorization
revocation. The shared store lock spans re-reading the pin, confirmation, and
removal. JSON/noninteractive callers still require --yes; cancellation writes
neither a peer change nor a prepared removal receipt.

Successful deletion now advances the retained public-trust receipt from prepared
to applied after synchronizing the peer directory. Receipt-finalization or lock
release failure after deletion returns an applied status-3 error, and the CLI
reports removed=true for that outcome. Previously release errors were ignored
and the receipt stayed prepared even after successful removal.

Store tests prove locked confirmation, no peer/receipt change on cancellation,
current fingerprint evidence, applied receipt content, and status 3 when a fixture
changes lock ownership before release. Real PTY acceptance covers machine-mode
refusal, default-no decline, SIGTERM cleanup, quoted public evidence, and accepted
removal using an isolated synthetic peer. No network connection or live peer
record is used. Abrupt-death recovery between deletion and receipt finalization
remains part of the broader crash-acceptance work.

Local full Go 1.26.0 race suite, final peer-removal/CLI/remote race tests, vet,
real PTY acceptance, Ruff, and basedpyright passed.


## Reproducible packaging and untracked shell-pa acceptance

The distribution tool now builds all four binaries from an isolated extraction
of the exact committed source archive and reads the bundled LICENSE/README from
that same snapshot. Two local builds at `48ae47d` were byte-identical; archive
checksums, platform settings, source/document correspondence, and native version
smoke passed. The first packaging commit `0ccc793` passed Linux/macOS CI (run
33991058913). See [distribution](distribution.md) for reproducible commands and
remaining publication gates. These are untagged `0.1.0-dev` packages.

The real pinned shell-pa acceptance script now runs the same disposable journey
with Git enabled and with PA_NOGIT set. Both prove unchanged adoption dry-run,
predecessor-file preservation, alternating exact-byte CRUD, shared lock refusal,
unchanged identity/recipients, and shell-pa CRUD after ceasing Fulla use. The
untracked store remains untracked, including after adoption and rollback. Fulla
refuses unacknowledged untracked deletion without changing the store; the fixture
then supplies the required --permanent-delete acknowledgement.

This exposed a status defect in pinned predecessor f75734b: its stdin add/edit
and delete functions end in `$git_enabled && git_add_and_commit`. With Git off,
that expression returns 1 even when publication succeeded. The harness records
that exact predecessor status and independently verifies resulting bytes or
absence. It does not normalize Fulla statuses or modify the predecessor. Both
real-pa modes passed locally, along with Ruff and basedpyright. The existing CI
step automatically exercises both modes. Full adoption-plus-sync-cutover
acceptance remains separate and incomplete; this does not authorize live-store
adoption or cutover.


## Combined real-pa adoption, sync activation, and rollback

The real predecessor fixture now runs the previously separated adoption and sync
steps as one continuous journey, for both Git and untracked pa stores. After
alternating CRUD, both participating stores pass deep verification. Two disposable
loopback SSH servers launch Fulla remote serve against their configured stores;
the production client uses real OpenSSH with generated identities and pinned
host keys. Public fingerprints are obtained independently from each local CLI,
then both sides enroll through the public peer-add surface.

Acceptance verifies first-sync refusal before dry-run without any local store
change, the authenticated inventory plan, unchanged live ciphertext during
preview, bidirectional application, preserved shared-name divergence, activation
on both peers, and the local receipt's pa-xfer retirement flag. A second sync
transfers nothing. After the fixture servers stop, shell-pa CRUD resumes on the
adopted store; all Fulla metadata, receipts, backups, and peers remain byte-for-byte
unchanged throughout that rollback. Original identity/recipient files remain
unchanged, and the no-Git store stays untracked.

The fixture server accepts `--remote-port 0` to write a known-hosts pin for its
actual ephemeral listener; the established reverse-forwarding default remains
available. CI builds this helper and runs the combined journey on both platforms.
The helper is not linked into the Fulla binary. This is repeatable real-SSH
loopback acceptance, not a claim of two physical hosts or network-failure testing.
One-sided interrupted sync/retry is covered separately by production wire tests;
the earlier real Mac/devbox drill remains separate evidence.

Both fixture variants, Ruff, basedpyright, and Go vet for the SSH helper passed
locally. The preceding no-Git checkpoint `f67d0c4` passed hosted Linux/macOS CI:
https://github.com/agensfield/fulla/actions/runs/33991562690.

Reproduce after building the pinned age tools and fetching the predecessor:

```sh
GOTOOLCHAIN=go1.26.0 go build -o dist/fulla .
GOTOOLCHAIN=go1.26.0 go build -o dist/acceptance-sshd ./scripts/acceptance-sshd
python3 scripts/acceptance-pa.py dist/fulla dist/pa-predecessor dist/pa-tools dist/acceptance-sshd
```


## Server-side sync-domain compatibility enforcement

A future sync-domain fixture exposed a server/client asymmetry. Sync checked the
local domain, but the remote server could authenticate and serve an inventory
from a store whose persisted sync domain was v2. Four regression cases failed
before the fix: current/previous protocol, with the domain changed before
authentication or after session authentication.

The server now checks the sync domain before issuing its authentication challenge
and before subsequent operations. Public hello/bye discovery remains independent
of sync support. Locks with an expected peer session also revalidate the sync
domain while held, closing the gap between an earlier request check and mutation.
MarkPeer checks the domain both before and after acquiring its lock, even when
called independently of a remote session.

The wire regression verifies typed metadata.unsupported and unchanged file
contents on both stores. A store regression upgrades the manifest inside lock
validation, verifies rejection and lock cleanup, then proves both direct dry-run
and activation marks refuse the future domain without changing the peer. This is
specific forward-domain enforcement, not completion of the transactional domain
migration program or every activation crash/applied-state boundary.

The combined real-pa/OpenSSH adoption journey at `c81b250` passed both hosted
platforms: https://github.com/agensfield/fulla/actions/runs/33991840041.

The full local Go 1.26.0 race suite and vet passed. Final store/remote
regressions also passed with race detection, including fresh hello/bye discovery
against a future sync domain.


## Applied-state reporting for peer sync marks

MarkPeer previously discarded lock-release errors and used a replacement helper
that returned only an error. A successful rename followed by failed directory
synchronization was therefore indistinguishable from failure before publication.
The filesystem helper now exposes ReplacePublished, reporting whether rename
occurred independently of the later fsync result; existing Replace callers retain
their previous interface and behavior.

Peer dry-run/activation marks use that publication result and return status 3 with
applied=true after publication if finalization or lock release fails. Errors remain
Fulla-owned and redacted. A local activation that is explicitly known to have
published is also reflected in Sync's partial result instead of reporting
activated=false; this still returns partial failure rather than clean success.

The filesystem regression injects a directory-sync failure after rename and
checks that the new bytes are visible, publication is reported, and no stage is
left. A missing destination is a negative control for pre-publication failure.
Four store cases cover dry-run/activation crossed with finalization/lock-release
failure. They verify status 3, actual saved peer state, owned-lock cleanup, and
preservation of a changed-owner lock. The injection seam is internal, never an
environment or CLI option. These are deterministic process-level failure tests,
not abrupt-death/power-loss durability proof. The broader activation crash and
recovery program remains open.

The full local Go 1.26.0 race suite and vet passed. Server-domain enforcement
at `2bebb66` passed hosted Linux/macOS CI:
https://github.com/agensfield/fulla/actions/runs/33992089407.


## Peer enrollment and rotation publication evidence

The publication-state audit also found SavePeer set its applied flag only after
its filesystem helper returned success. Thus rename followed by directory-sync
failure lost publication evidence. Rotation receipts also remained prepared after
successful pin changes. Enrollment now uses PublishNewPublished, the no-replace
counterpart to ReplacePublished, while rotation uses ReplacePublished. Both
preserve applied status 3 when finalization fails after rename. Existing callers
of PublishNew retain the original error-only interface and no-replace behavior.

Successful rotation advances the public-trust receipt to applied, retaining the
previous and replacement pin records. Receipt finalization failure reports the
already-applied pin change rather than implying the old authorization remains.
A failure before receipt finalization may still leave prepared evidence for manual
inspection; automatic abrupt-death reconciliation remains open.

Six store cases cover enrollment/rotation crossed with success, injected
post-publication failure, and changed-owner lock-release failure. They inspect the
actual live fingerprint, cleared prior activation/dry-run state, receipt phases
and both pin records, and lock cleanup/preservation. Filesystem acceptance injects
failure after no-replace rename, verifies visible new bytes and publication state,
then proves another creation refuses to replace those bytes and leaves no stage.
The tests do not equate publication with durability after failed fsync.

The preceding sync-mark fix at `5270eb2` passed hosted Linux/macOS CI:
https://github.com/agensfield/fulla/actions/runs/33992357335.

The complete local Go 1.26.0 race suite and vet passed for the enrollment and
rotation publication changes.


## Killed peer publication checkpoints

Eight subprocess cases now kill the actual store writer after peer enrollment,
peer rotation, dry-run marking, or activation publication, each with Git enabled
and disabled. The fixture uses the test binary and existing internal publication
hooks; production executables expose no failure switch. Rotation uses a genuinely
different generated recipient pin, passed to the child as public metadata only.

The cases verify a live owner cannot be recovered, a killed owner's lock blocks
ordinary secret reads, and a wrong owner token cannot release it. Explicit
matching-token recovery releases the dead owner's lock and accurately reports
recovered=false because these operations have no pending transaction journal.
The published peer fingerprint/host and dry-run/activation state remain intact;
entry plaintext, ciphertext, and local identity material remain byte-for-byte
unchanged. The rotation's prepared receipt retains both old and new public pins.

This proves post-publication killed-owner state preservation and safe lock
recovery. It also makes a remaining gap concrete: lock recovery does not reconcile
that prepared rotation receipt to applied, and no pre-publication or removal
boundary is covered here. These cases are not full peer-operation crash recovery
or power-loss durability acceptance. A durable reconciliation design must preserve
old-binary fail-closed behavior instead of silently adding an ignored journal.

The targeted Go 1.26.0 race tests and store vet passed. The hosted full suite will
exercise these cases on Linux/macOS as part of its existing race gate.


## Bound peer-rotation receipt reconciliation

Peer rotation now records its exact prepared receipt ID in lock/info before
publishing the replacement pin. The binding is a validated optional operation
field, not a raw owner token, guessed timestamp, or scan of historical receipts.
Recovery preserves it while replacing the dead owner's PID/token. A bound peer
receipt conflicts with other pending journals instead of allowing mixed recovery.

Under the recovery guard, Fulla strictly validates the bound rotation receipt
and both public peer records. Exact equality with the replacement marks the
receipt applied; exact equality with the previous record marks it aborted.
Unexpected live state, contradictory phases, missing records, or unsupported
peer metadata fail with the lock retained. Recovery never changes the live pin.
Already finalized matching receipts are idempotent. Post-publication receipt
sync failures preserve applied-state evidence, as do lock-release failures after
reconciliation.

The killed-process matrix now includes ten Git/no-Git cases, including the old
lock-info format without a receipt binding. Bound rotation recovery finalizes its
receipt and reports recovered=true. Legacy unbound recovery retains the previous
recovered=false behavior and leaves historical prepared receipts for inspection;
it never guesses that one belongs to this lock. A second subprocess test injects
an unexplained live record, lets recovery take ownership and refuse it, kills that
recovery process, restores only the fixture's deliberately changed record, and
successfully retries with the new dead-owner token and preserved receipt binding.
Unit cases cover old/new/ambiguous/contradictory states, unchanged authorization,
and byte-identical repeated receipt reconciliation.

Compatibility boundary: the live pa-v1 files, receipt v1 shape, and domain manifest
remain unchanged. The optional lock-info field is used only by this recovery
implementation. Complete an interrupted bound rotation with a Fulla build that
supports this binding before rolling back to an older build; older builds can
ignore the field and leave the receipt unresolved. No automatic migration of
legacy ambiguous receipts is claimed. Pre-publication removal/enrollment failure
boundaries and power-loss durability remain separate acceptance work.

The prior killed-peer checkpoint `270a0c5` passed both hosted platforms:
https://github.com/agensfield/fulla/actions/runs/33992813831.

The full local Go 1.26.0 race suite and vet passed. The final ten-case killed
writer matrix, reconciliation cases, and killed-recovery takeover/retry case
also passed under race detection, followed by store vet.


## Bound peer-removal recovery

Peer removal now binds its prepared receipt to the held lock before deleting the
pin, sharing the existing rotation binding helper. The removal fixture can pause
after deletion and directory synchronization but before receipt finalization.
Recovery strictly distinguishes removal from rotation: a valid existing peer
registry with the named pin absent finalizes removal as applied; the exact old
record finalizes it as aborted. A different live record, unexpected replacement
payload, missing registry, or contradictory receipt fails closed. Reconciliation
never recreates a removed peer or changes live authorization.

The killed-process matrix now includes removal on Git and no-Git stores, twelve
cases in total with the legacy rotation fixtures. Removal recovery verifies the
pin remains absent, the receipt becomes applied, and local secret/identity bytes
remain unchanged. Unit cases cover old, removed, unexpected, and missing-registry
states, preserving evidence on refusal and retrying successful reconciliation.
Existing rotation, takeover/retry, cancellation, and applied-error cases remain
part of the relevant store gates. The internal post-publication pause is not a
production CLI/environment control.

The same compatibility boundary applies: complete an interrupted bound operation
with a supporting Fulla binary before using an older recovery implementation.
Unbound historical receipts remain untouched. Actual power-loss behavior and the
remaining pre-publication boundaries are not inferred from these subprocess and
injected-state tests.

The prior bound-rotation recovery checkpoint `cf030f0` passed hosted Linux/macOS
CI: https://github.com/agensfield/fulla/actions/runs/33993278466.

Remaining audit: handled I/O failures can release the owned lock while leaving a
prepared receipt. The new reconciliation binding survives only while the lock
metadata exists; that error path needs a separate retention/finalization policy.
The killed-owner tests do not prove it.

The full local Go 1.26.0 race suite and vet passed. Final targeted removal,
rotation-reconciliation, killed-owner, and takeover/retry race cases and store
vet also passed after adding the missing-registry guard.


## Retained evidence after handled peer-operation errors

Rotation and removal now retain their owned lock when an operation fails after
its receipt binding has been published. The previous defer released that lock,
deleting the binding and stranding prepared evidence. Binding publication is
reported independently of its directory-sync result, so a binding that became
visible before an fsync error is retained too. Errors state recovery_required=true
and preserve whether the live peer operation published (status 3/applied=true)
or failed before publication (peer.incomplete with applied=false).

Four subprocess cases exercise handled post-publication errors for rotation and
removal on Git/no-Git stores. The process reports the expected typed failure and
exits normally. The parent verifies a dead-owner lock and receipt binding remain,
ordinary secret reads are blocked, and matching-token recovery finalizes the
receipt, releases the lock, and preserves the published authorization state and
exact entry ciphertext/plaintext. These complement SIGKILL tests rather than
using their result to infer normal-error behavior. The existing successful,
cancellation, unbound enrollment, and changed-owner cases retain their policies.

This fixes binding loss on the covered handled-failure path. Binding and operation
pre-publication interruption, actual filesystem durability faults, and the wider
acceptance matrix remain separate work; deterministic injected failures are not
physical power-loss proof. The preceding removal recovery `f1f9b52` passed both
hosted platforms: https://github.com/agensfield/fulla/actions/runs/33993589759.

The complete local Go 1.26.0 race suite and vet passed, including the final
normal-exit subprocess cases with bounded execution time.


## Transaction writes into future backup domains

The full-contract audit reproduced a cross-domain compatibility bug: ordinary
mutations checked only `transactions`, then published current-format snapshots
into `backup` even when the live manifest advertised backup version 2. Journal
finalization had the same omission. Four regression cases failed on the prior
implementation: new mutation and interrupted-transaction finalization, each on
Git and no-Git stores, all incorrectly succeeding.

Mutation now checks the backup domain under its shared lock before staging;
journal finalization rechecks before publishing entries, Git, backups, or
receipts. The regression compares every store file after refusal, verifies
independent reads remain available, and retries after undoing only the fixture's
injected version change. This retry is not a real metadata downgrade.

An older binary must not write a format it does not understand. This guard does
not fulfill the complete older-binary CRUD promise: writes currently depend on
the backup schema and refuse when that domain is unsupported. A compatible
write/migration policy and actual prior-state upgrade fixtures remain open in
the acceptance matrix. No new migration format or silent bypass was introduced.

The four cases passed after the guard, including supported-version retry and
read independence. The complete local Go 1.26.0 race suite and vet passed; the
final expanded regression was also run separately under the race detector.
The prior acceptance-matrix/legacy-pin checkpoint `da7fd0f` passed hosted CI:
https://github.com/agensfield/fulla/actions/runs/33994466322.


## Historical Fulla binary upgrade and rollback

`scripts/acceptance-upgrade.py` runs two real binaries on disposable Git and
no-Git stores. CI checks out and builds the exact historical source
`831caf68655b41b4ca5b064b7693af35df68a6e6`, the first CLI checkpoint exposing the
recovery and identity workflows. The earlier library checkpoint `7fd872a` had
those store methods but not the grouped CLI dispatch; attempting its history
command correctly returned `invocation.invalid`. It is not used as the CLI
acceptance baseline. Neither historical commit is a published release.

The historical binary initializes a store, adds/edits exact binary values, and
rotates its identity. Current Fulla reads/deep-verifies without changing any
store file and restores an old snapshot through the sealed identity chain. On
Git stores it also restores the historical commit. Both binaries then alternate
CRUD on the same pa-v1 store, preserving manifest and active identity files. A
current-binary identity rotation is followed by historical-binary reading and
restoration of its own earlier snapshot, verified by current Fulla.

Both variants passed locally using binaries built from the exact historical
archive and current checkout. Ruff and basedpyright pass. CI runs this journey
on Linux and macOS. The private fixture environment disables Git auto-maintenance
for the historical process; this isolates format compatibility and does not
claim that the older binary fixed its known background-maintenance race.

The original manifest at `1999674` already declared all five domains at version 1.
This journey proves specific real prior/current state interoperability without
a format transformation. It does not invent a version-0 migration or satisfy
interrupted domain-upgrade, future-domain CRUD, or released-binary cross-host
acceptance by implication. Those remain open in the full-contract matrix.


## CRUD with a newer backup domain

The initial `d51bf81` guard prevented corruption but also refused ordinary writes.
The implementation now preserves supported transaction snapshots independently
of the newer backup feature: journals bind `snapshot_domain: "transactions"`,
and full encrypted before/after snapshots publish under
`.fulla/transaction-backups/ID`. Newer `.fulla/backups` files and the manifest
remain untouched. Missing or malformed backup versions still refuse; this is
not a fallback for corrupted metadata. Existing ordinary journals keep their
serialized shape and cannot switch destination during recovery.

Compatible backup readers discover both locations and reject duplicate IDs or
mismatched journal domains. Restore uses the selected snapshot location, and
prune journals preserve it across interrupted deletion. Backup operations still
refuse unsupported backup metadata, while ordinary CRUD can proceed using the
understood transaction domain. See [the compatibility policy](metadata-compatibility.md)
for the explicit historical-binary boundary and remaining future-transaction
limitation. This completes a concrete part of the earlier open safe-write policy
without claiming all metadata migration requirements are fulfilled.

The complete local Go 1.26.0 race suite and vet passed. Targeted race tests cover
Git/no-Git CRUD with exact before/after snapshots, six killed-process boundaries,
recovery after a fixture version change without destination retargeting, mixed
snapshot restoration/pruning, interrupted transaction-snapshot pruning,
unknown-location and duplicate-ID refusal, and exact full-archive preservation
followed by restoration from the recovered snapshot.

The actual historical CLI `831caf6` was also given a killed transaction containing
the new field. It returned `metadata.invalid`, changing only recovery-lock
ownership while preserving all other files and pending evidence. Current Fulla
then recovered successfully with the newly claimed owner token, on Git and
no-Git stores. CI now builds the current store-test fixture for this drill.
Ruff, basedpyright, actionlint, and diff checks passed. Prior `cbe2510` completed
hosted CI: https://github.com/agensfield/fulla/actions/runs/33994967310.

## Canonical command machine-contract baseline

The [machine input/authority matrix](machine-contract-matrix.md) now lists all 34
canonical command leaves with explicit input/authority, baseline outcome, and
additional workflow evidence. A shared CLI test exercises one safe read/refusal
case for every leaf on Git and no-Git stores in JSON mode, and repeats all 30
non-passthrough cases with `--non-interactive`: 128 cases in total.

The test supplies an instrumented, unselected stdin reader and asserts it is
never consumed. JSON cases require exactly one `fulla.cli/v1` document, canonical
command, matching status/ok, data/warnings on success, and symbolic code/message/
details on failure. Raw show preserves bytes in noninteractive human mode;
metadata and diagnostics cannot contain the selected fixture's plaintext/base64
value. Before/after comparisons include the complete fixture tree, file contents,
and modes. Prune without `--yes` remains an unchanged preview. Clipboard and SSH
cases refuse before backend/transport use, and JSON passthrough cases refuse
before executing child/protocol streams.

All 34 rows passed initially; no production fix was needed. The full local CLI
race suite and vet passed, and the final expanded matrix passed separately under
the race detector. A static check confirms the 34 unique test rows and documented
rows agree; diff checks pass. Prior `f6cca49` and `cb4ae83` both completed hosted CI.

This is a shared baseline, not a substitute for authorized success journeys or
all TTY, descriptor, signal, and authority combinations. Those remain in J3/J1.
The transaction-domain compatibility limitation is also retained: relaxing its
version check would authorize writes into an unknown recovery format, so it
needs a deliberate persisted-format design rather than a guard bypass.


## Authorized native agent journey (2026-09-06)

The new `scripts/acceptance-agent.py` passed locally against the current native
binary with Git and without Git. CI now runs it on both hosted platforms.
Generated disposable stores exercise 26 canonical successful leaves with Git,
22 without, following the workflow documented in the machine contract matrix.
The run target verifies native PID replacement, process group/session, clean
environment, mapped value, binary stdin, exact stderr, cwd, literal `--json`
argument after the delimiter, and propagation of exit status 23.

Three initial harness assumptions were resolved by checking actual behavior and
source, without production changes: macOS Python adds runtime environment keys
(the assertion now uses a direct clean-launch control); full export publishes one
non-secret receipt after capturing its archive (all preexisting bytes/modes must
remain identical); archive restore intentionally normalizes Git's 0400 loose
objects to private 0600 files (every path, byte digest, and normalized mode is
still compared). These were acceptance assertion corrections, not product fixes.

Ruff formatting/lint, basedpyright, actionlint, and the final expanded journey
passed. The preceding `d3ae314` completed hosted CI successfully:
https://github.com/agensfield/fulla/actions/runs/33996455364.
J3 remains partial for specialized surfaces and exhaustive input/authority/signal
combinations. No live pa store, clipboard, or remote host was used in this journey.


## Real Fish completion engine acceptance (2026-09-06)

`TestFishCompletionBehavior` now runs Fish's actual `complete -C` engine with
private HOME/config and no user configuration. Seven cases cover top-level and
grouped command prefixes, a quoted store path plus JSON option, equals-form
config, shell-name completion, incomplete store input, and Git/run passthrough
boundaries. Linux CI installs Fish and explicitly requires this test, so absence
cannot silently skip the gate. The existing syntax test also exercises Fish.

All completion tests passed locally using the official Fish 4.9.2 macOS runtime
extracted into ignored `dist/fish-runtime`, without host installation or shell
configuration changes. No production correction was necessary. actionlint and
diff checks passed. This closes the previously absent real Fish engine evidence,
not exhaustive interactive-shell installation or every possible completion case.
The test uses the documented interface:
https://fishshell.com/docs/4.5/cmds/complete.html.


## Clipboard fixture receipt publication race (2026-09-06)

Linux CI at `e83a6e6` passed all seven required Fish behavior cases, then failed
`TestExpiryWorkerFreshProcessAndDigestOnly/false` with
`strconv.Atoi: parsing "": invalid syntax`. Job 101390498853 in run 33997512300
contains the exact error. The fixture used `os.WriteFile` on the final PID receipt
while the parent concurrently polled that path. Creation is visible before the
write, so the parent could read an empty receipt. A controlled create-before-write
probe reproduced that observation directly.

The fixture now writes a private pending receipt and renames it into place after
the write completes. PID parsing remains strict; the test still requires a fresh
process, its exit, exact clearing of the original clipboard, preservation of a
replacement, and no plaintext in the expiry request. No production clipboard
behavior changed. This is distinct from the previous Git maintenance CI failure.
Agent-journey commit `c93ed51` completed both hosted platforms successfully.

The corrected expiry test passed 20 repetitions under the race detector locally
(106.6 seconds). macOS CI at `e83a6e6` passed; the Linux failure was the receipt
publication race described above.


## Confirmed plugin shutdown hang (2026-09-06)

The [bounded reproducer and investigation](plugin-lifecycle.md) confirms Fulla
can remain stuck after a malformed plugin response when the plugin ignores
SIGINT. The synthetic plugin explicitly records receiving that signal, proving
the operation reached age's unbounded shutdown Wait rather than merely running
slowly. The reproducer terminates only its own process group. Ruff and
basedpyright passed. No production fix is claimed; lifecycle ownership and
consistent packaged/go-install behavior remain required implementation work.


## Owned plugin shutdown implementation (2026-09-06)

The confirmed post-error Wait hang is corrected with a maintained age v1.3.2
plugin-client adaptation. Only package/import/encoding delegation and Close's
lifecycle behavior differ: one second of graceful shutdown after SIGINT, then
kill and wait for the owned child. No operation-time deadline is imposed on
PIN/touch interaction. File encryption stays in the official age module. Source
hashes, BSD licensing, deterministic parser tests, patch/update obligations, and
precise limits are recorded in internal/ageplugin/README.md and plugin-lifecycle.md.

The original native reproducer now finishes with SIGINT acknowledged, typed
crypto.encrypt_failed/status 1, empty stderr, and no fixture-value leakage.
Linux/macOS CI requires this fixed behavior. Direct tests verify graceful exit
and forced termination/reaping, including a watchdog that catches the original
hang. Three repeated race-enabled lifecycle runs passed; plugin roundtrip,
interaction/refusal, and retained parser tests passed under the race detector.
Ruff, basedpyright, actionlint, and diff checks passed. Prior abed4d2 CI completed
successfully on both platforms (33997799228), including the clipboard fixture fix.

The binary packager now includes THIRD_PARTY_NOTICES from the exact source
snapshot; package acceptance compares those bytes along with the other bundled
documents. This maintained source adaptation has an explicit update cost and is
not represented as an untouched upstream package. Protocol stalls and arbitrary
plugin-created descendants remain separate from the post-protocol shutdown fix.

The full controlling-terminal acceptance passed after the change, including
plugin PIN/confirmation, typed refusal/cancellation, encrypted SSH unlocking,
restore confirmations, and editor signal cleanup. A temporary Go source overlay
restored only upstream's original Close implementation; the new lifecycle test
then failed deterministically with `shutdown required watchdog`. The actual
worktree was not modified by that negative-control run.


## Composite disaster restore acceptance (2026-09-06)

`TestCompositeDisasterArchiveRestoresAllState` combines binary/empty/newline
values, a deleted value, encrypted snapshots, Git history where enabled, two
successive retained identity rotations, receipts, and saved activated peer
metadata in one independently protected archive. The generated source directory
is then removed before restoration. Both absent and existing-empty targets are
covered on Git and no-Git stores, using only the archive and independent recovery
identity for restore.

Before exercising any recovery mutation, the test compares the complete restored
path set, every file digest, and all required private modes against the source
snapshot. Source export may add only its receipt and must preserve every prior
path's contents/mode. Public identity, peer pins/dry-run/activation metadata,
snapshot inventory, Git history, and exact live values are checked separately.
Old-key snapshot restore recovers binary bytes and the deleted entry through the
two-step retired-key chain. Git mode also overwrites a value and restores its
pre-rotation history. Neither recovery operation changes restored peer authority.
The peer record is generated metadata for archive acceptance, not a claim that
this fixture performed real mutual SSH synchronization.

All four combinations passed under the race detector (27.5 seconds test time).
No production change was needed. This closes the missing composite state fixture
in J7; interrupted archive staging/publication, broader intrusion fixtures, and
other full-spec gates remain open. The separate transaction-snapshot namespace
archive test remains relevant. Plugin-shutdown commit e9dabe5 completed hosted CI
on both platforms: https://github.com/agensfield/fulla/actions/runs/33998306333.


## Full restore directory durability and interruption boundaries (2026-09-06)

Restore staging previously synced each written file's immediate directory and
finally the staging root, but did not explicitly sync all newly created ancestor
or empty directories. The restore path now syncs every staged directory
bottom-up after complete validation and before publication. A test exercises
real sync calls while checking complete directory coverage and child-before-parent
ordering; an injected error must propagate immediately.

An internal callback seam exposes extraction, validated, empty-target-vacated,
published, and parent-synced boundaries without adding command flags. Thirty-six
Git/no-Git and absent/empty-target cases cover handled errors and actual SIGKILL
(18 each). Pre-publication handled failures remove staging; killed processes
leave one private stage, and fresh retry leaves that orphan unchanged. After
publication the target matches every expected path, byte digest and mode, passes
deep verification, and refuses an overwriting retry without mutation. Handled
post-publication failures report applied status 3. Source state remains unchanged.

The full matrix and sync test passed under the race detector (21.7 seconds).
[Interrupted restore guidance](archive-recovery.md) records exact outcomes and
remaining gaps: SIGKILL is not power-loss proof, a killed stage can retain active
identity material under private modes, no prefix-based automatic deletion occurs,
and explicit orphan identification/cleanup plus cleanup-operation failures remain
open. Prior composite-restore commit 3c7446b passed hosted Linux/macOS CI:
https://github.com/agensfield/fulla/actions/runs/33998552265.

The broader full/composite/transaction-snapshot/restore test group also passed
under the race detector (53.4 seconds); store/CLI vet and diff checks passed.


## Restore cleanup errors and path ownership (2026-09-06)

Unpublished restore cleanup no longer discards RemoveAll errors. It removes the
owned stage and syncs its parent; failure to confirm removal returns the typed
recovery.cleanup_failed error with cleanup_required, exact staging_path/target,
and applied=false. The original typed operation code is retained without raw
message leakage. Signal exit statuses survive the cleanup-error report.

A second defect was corrected: after the stage had been renamed into its target,
the old deferred RemoveAll still addressed the former stage name. A concurrent
new occupant at that name could be deleted. Publication now relinquishes cleanup
of that obsolete name before any post-publication boundary.

Tests use real chmod-based permission denial on a non-root runner, covering
ordinary error, interaction refusal, and signal cancellation while confirming
private identity material remains and the target is unpublished. A separate
post-publication replacement fixture proves its new occupant is preserved and
the published store remains valid. Targeted tests passed under the race detector.
The restoration guidance distinguishes removal failure from uncertain deletion
durability and retains automatic orphan inspection/cleanup as unfinished work.

A temporary Go overlay restored the preceding archive implementation; both new
regressions failed as expected (hidden cleanup failure and deletion of a reused
stage name). The worktree was unchanged by the negative-control run.

The broader archive/restore group passed under the race detector (57.8 seconds),
and store/CLI vet plus diff checks passed. Prior 6e42c06 completed Linux/macOS CI:
https://github.com/agensfield/fulla/actions/runs/33998879946.


## Exact-source release workflow preparation (2026-09-06)

The new tag-triggered release workflow validates the exact event/tag/checkout
commit, origin/main ancestry, source CLI version, clean checkout, and tracked
nonempty nonsymlink release notes. The existing Linux/macOS CI workflow is now
reusable; its tested Linux packages are retained only for tag runs and consumed
after both platforms succeed. Publication verifies packages, signs provenance,
verifies signer/source/tag identity, creates a new draft, downloads and checks
its assets and attestation, and only then publishes. It rechecks the remote tag
before creation/publication and never clobbers an existing release. A failed
draft stays inspectable. Preview versions are marked prerelease/not latest.

The source checker is read-only and creates no tags or artifacts. Nineteen
real-Git cases passed under the race detector: six positive version/tag-form
cases and thirteen refusal cases, including wrong tag/event/checkout, unreviewed
source, dirty checkout, development/invalid versions, and missing/empty/symlink
notes. Both workflows passed actionlint; release-package vet and diff checks passed.
Action versions and attestation verification options were checked against official
upstream sources before pinning. No new runtime dependency was introduced.

This prepares publication infrastructure; it does not satisfy the full-spec
release gates or prove hosted OIDC/signing/draft/download behavior. No version
was changed, tag created, or release published. Actual tag execution, attestation
verification and installed-release/tap acceptance remain open. Prior b89142f
completed hosted CI: https://github.com/agensfield/fulla/actions/runs/33999152217.

### Explicit development panic diagnostics (2026-09-06)

The CLI previously redacted recovered panics in every build, leaving the spec's
opt-in development/test stack behavior unimplemented. The default build keeps
its fixed `internal.failure` result. The explicit `fulla_debug` build tag now
re-panics with the original value and stack; no runtime environment switch can
enable this in an ordinary installed binary. See [crash diagnostics](crash-diagnostics.md).

Four isolated string/error × human/JSON subprocess cases run for each build
policy. The default requires status 1, exact redacted output, no private value,
stack, path, or generated crash artifact. Debug requires the original synthetic
value and CLI stack. Two negative-control overlays inverted the respective
production constants; both were rejected by the independent policy tests.

The first debug fixture incorrectly required empty stdout: Go's testing runner
writes its own failure banner when the deliberate panic escapes. Inspection
confirmed the CLI stack on stderr; the debug assertion was corrected, while the
default output checks stayed strict. Full CLI race tests (15.3s), debug-policy
race tests, CLI vet, actionlint, and diff checks passed locally with Go 1.26.0.
Linux/macOS CI now tests both policies. Final artifact fault injection and
independent goroutine/runtime containment remain open; this is not universal
panic containment. Prior source `7f12148` passed Linux/macOS CI 33999820490.

### Combined trust-change and decryption-sentinel journey (2026-09-06)

`trust_journey_test.go` combines one-sided enrollment refusal, real server-key
rotation, stale-pin refusal, explicit repin, captured-proof replay refusal,
mutual dry-run, and successful exact-byte sync with shared-name preservation.
All four Git/no-Git × supported-protocol combinations pass. During refusal
stages, each side uses a sole synthetic plugin identity. The executable records
an invocation and exits; positive entry reads prove the observer works. Refusals
must leave it uninvoked and preserve all store paths, modes, and file digests.
Restored generated native identities complete the successful half of the journey.

Two Go overlays deliberately inserted early entry decryption into client/server
paths. Both failed at the sentinel, proving the tests detect an ordering
regression. No product runtime hook or cryptographic behavior changed. See
[trust-change acceptance](trust-change-acceptance.md) for the fixture substitution
and evidence boundaries. Whole remote race tests passed (50.3s), remote vet and
diff checks passed. Prior facf6df passed both Linux/macOS CI jobs 34000210947.

The future transaction-domain write policy remains unresolved: relaxing the
version guard without an understood recovery protocol would be unsafe. Another
literal spec conflict was escalated as an optional asynchronous decision:
no-Git deletion is described as irreversible while transaction backups must be
retained until explicit prune. Current code preserves encrypted recovery and
still requires the permanent-delete acknowledgement; no destructive retention
change or spec rewrite was made while that decision is pending. Neither issue
prevents independent implementation and acceptance work.

### Preserve ownership after recovery-journal publication errors (2026-09-06)

Transaction and rotation used `PublishNew`, losing the bit that distinguishes a
successful rename followed by a failed directory sync. Their error cleanup could
therefore delete recovery staging and release the lock beneath an already
published journal. Both now use `PublishNewPublished` and retain staging/lock
when publication occurred. A fixed status-3 `transaction.incomplete` identifies
the transaction and recovery requirement while correctly reporting no live
application yet. Raw publication errors are redacted.

Four Git/no-Git × transaction/rotation handled-failure cases verify unchanged
live bytes, prepared journal/staging/lock retention and successful completion.
Discarding ownership in a negative-control overlay breaks all four cases by
removing staging. The existing killed-owner suites now include `journaled`:
transaction Git/no-Git and rotation retained/destructive key policies. Combined
handled and killed recovery tests passed under race (46.9s), as did securefs's
actual-rename/injected-sync-failure test and store vet. See
[journal publication](journal-publication.md) for exact evidence boundaries.

Pre-journal ignored cleanup failures and killed-writer orphans remain separate
work. This fix does not claim physical power-loss durability. Prior 762bc31
passed Linux/macOS CI 34000545363.

### Report unpublished-stage cleanup failures (2026-09-06)

Transactions and rotations now share owned-stage cleanup: a directory is eligible
only after exclusive creation succeeds, removal is followed by parent sync, and
owned-lock release is attempted even if staging removal fails. Cleanup/release
errors produce redacted transaction.cleanup_failed details rather than being
silently replaced by the original operation error. Signal statuses survive;
failed ownership checks do not remove another owner's lock. Human errors show a
quoted staging path and lock-inspection guidance; machine details remain typed.

Twelve real chmod-denial fixtures cover Git/no-Git × transaction/rotation ×
error/refusal/signal, including retained staged private identity files. Existing
store bytes remain unchanged and ordinary reads retain the original value after
lock release. A thirteenth test checks changed-lock ownership. A source overlay
restoring ignored cleanup/release errors fails all 13. The broader handled and
killed transaction/rotation recovery race group passed (64.0s), CLI cleanup/panic
race tests and store/CLI vet passed. See [cleanup semantics](journal-publication.md).

This does not implement automatic SIGKILL orphan discovery/removal. A cleanup
path can describe partially removed staging or uncertain deletion durability;
no false guarantee of complete retained data is made. Prior 25b46f6 passed both
platforms in CI 34000815401.

### Doctor exposes leftover transaction staging (2026-09-06)

Doctor previously reported healthy once a pre-journal killed writer's lock was
released, despite retained staging (including a generated rotation identity).
It now lists sorted relative staging paths and reports transaction.staging_present
as unhealthy. Human output identifies inspection evidence, not deletion authority.
Only known transaction metadata and valid ID directories are inventoried, with
a 1024-directory limit; unknown domains/unexpected paths/over-limit scans report
staging_unavailable and no partial list. The existing whole-tree safety scan is
not bounded by this new inventory limit.

Four actual killed-writer cases cover Git/no-Git transactions and rotations,
live-owner refusal, dead-lock release, unchanged live values, retained identity
staging and mutation-free doctor inspection. Hiding staging after lock release
in a source overlay breaks all four. Unsupported-domain, unexpected-file and
limit fixtures preserve their evidence while reporting unhealthy. Doctor,
cleanup and structural/deep checks passed under race (18.3s); CLI doctor/cleanup/
panic checks and store/CLI vet passed. Automatic orphan ownership/cleanup and
sibling full-restore staging discovery remain open; see journal-publication.md.

### Destructive retirement refuses retained staging capsules (2026-09-06)

A killed pre-journal rotation leaves a generated identity beside a sealed copy
of the old private key. The Git/no-Git crash fixtures now unwrap that capsule,
prove exact equality with the current key and decrypt an existing entry. A later
destructive rotation must not imply that local retirement succeeded while this
capsule remains. Fresh destructive rotation now checks staging before locking
and again under lock; destructive recovery excludes only its validated own
stage and refuses unrelated evidence. Refusals preserve the store and direct
inspection through doctor, without guessing deletion authority.

Two fresh and two recovery refusal cases pass. Disabling the guard causes all
four to fail. Normal continuity/destruction and killed-owner destructive recovery
with only owned staging still pass; the combined race group took 31.8s, with
store vet/diff checks passing. This is a retirement-safety fix, not automatic
orphan cleanup or isolation from same-user/external copies. See the retirement
section of journal-publication.md for evidence and applied-state boundaries.

### Recover only an explicitly bound unpublished stage (2026-09-06)

Transactions and rotations now persist stage_id in their owned lock after empty
stage creation and before private staging writes. Dead-owner recovery preserves
this ID through takeover and removes only that directory when no journal exists,
syncing removal before lock release. Published journals retain their normal
recovery path and must match the binding. Duplicate/invalid/conflicting bindings
fail closed. A handled cleanup failure now retains a bound lock for retry instead
of losing the ability to identify its leftover private stage.

Eight Git/no-Git × transaction/rotation × bound/reconstructed-legacy killed-writer
cases pass. Bound cases clean their stage; legacy unbound cases remain visible
and untouched after lock release. A separate failed cleanup recovery process
preserves the new owner token and stage ID, then a later retry succeeds after
fixture-only permission repair. The original permission-denial fixture acted
before validation, so it tested refusal instead of takeover; injection now occurs
after validation through the internal seam. Four failed-test fixture directories
were safely moved to Trash. Dropping the binding during takeover fails all four
current-writer cases. Malformed-binding, conflicting-journal and changed-owner
fixtures pass without altering evidence.

Full repository go test -race ./... and go vet ./... passed with Go 1.26.0
(store suite 259.4s), along with diff checks. Prior 9719127 passed Linux/macOS CI
34001517182. This additive lock-info field is not a domain migration. Old readers
may ignore it, so use a supporting binary for bound cleanup before rollback.
Unbound historical staging, the empty pre-binding creation window, sibling full
restore stages, all cleanup crash boundaries and physical power-loss proof remain
open. See journal-publication.md and metadata-compatibility.md.

### Check ownership before deleting unpublished staging (2026-09-06)

Cleanup previously checked ownership only when releasing the lock, after deleting
staging. It now verifies the token, stage binding and absence of conflicting peer
binding before deletion. Changed/missing/malformed ownership returns inspection
requirements while preserving all stage/lock evidence, without claiming the
original owner retained authority. Four fixtures cover token/binding changes and
missing/corrupt owner data; removing the check makes all four detect deletion.

Combined cleanup/binding/killed-writer/published-journal race tests passed (21.7s),
plus the final targeted ownership race check and store vet. This preserves the
cooperative shared-lock contract, not same-user isolation. See the ownership
section of journal-publication.md. The full-spec goal and broader human-facing
acceptance work remain active.

### Ordered human Git/no-Git journeys (2026-09-06)

The new native acceptance-human.py joins fresh initialization, hidden text/empty
input, generation, binary editor add/edit, listing, fixture clipboard copy/expiry,
move/delete, history/backups and declined/confirmed deletion recovery into one
ordered journey per Git mode. Final complete names and exact values, deep doctor,
editor cleanup, lock/staging absence and clipboard expiry pass. JSON observation
selects opaque recovery IDs without coupling the human presentation to JSON.
The no-Git case proves current retained-backup behavior without resolving the
pending irreversible-deletion wording contradiction.

Extracted the existing PTY driver into acceptance_terminal.py, preserving its
stdin independence, deadlines, terminal restoration and child cleanup. The
original full interactive harness passes after extraction. Both new journeys
pass against a fresh native build. Ruff format/lint, basedpyright (zero warnings),
actionlint and diff checks pass. Initial lint caught loop-closure capture, fixed
by making each journey a function; typing checks led to an explicit scripts
execution root and typed metadata observations. The new module cache is scoped
out of Git so it cannot dirty the release checkout.

Linux/macOS CI now runs the ordered human journey. Real clipboard backends,
human presentation, shell installation and release qualification remain their
own gates. Prior cc82d88 and 627ac88 passed hosted CI 34002250879/34002081191.
See human-journey.md for exact scope and limitations.

### Human metadata presentation (2026-09-06)

Default successful control-command output now uses command headings, indented
labels and lists. It retains every public result field, including false flags,
empty collections, nulls and recovery identifiers. Strings and metadata keys
escape terminal controls. The shared renderer projects through existing JSON
field annotations, excluding private/internal fields and preserving full integer
precision; it does not introduce a second result schema. Machine envelopes,
raw show, and stream-owning passthrough commands retain their existing paths.
Warnings remain on stderr. This provides readable complete metadata rather than
command-specific tables; scripts should explicitly request `--json`.

CLI race tests and vet passed, including output-write failure, excluded-field,
control-character, empty-state and machine-envelope checks. Fresh native builds
passed the original controlling-terminal harness, ordered Git/no-Git human
journeys and both authorized noninteractive agent journeys. The ordered journeys
now require human result headings. The predecessor checkpoint `2239555` also
passed hosted Linux/macOS CI run 34002634212. Release qualification and the
remaining acceptance-matrix gates are still open; no live store was changed.
