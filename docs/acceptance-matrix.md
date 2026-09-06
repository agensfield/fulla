# Full-spec acceptance matrix

Audited against [the locked contract](product-spec.md) at implementation commit
`32c0ad8`, 2026-09-06. This is the current acceptance index; the chronological
[development ledger](development-status.md) retains individual experiments,
failures, fixes, and CI receipts. The full goal is not complete.

**Bounded evidence** means the linked fixture or implementation proves the stated
case. It does not prove every invocation, platform, interruption boundary, or
security invariant. **Open** means required evidence is missing or incomplete.
No row below replaces the wording or scope of the locked specification.

## Nine journeys

| ID | Required journey | Inspected evidence | Remaining acceptance |
| --- | --- | --- | --- |
| J1 | Fresh human store | [Ordered Git/no-Git journey](human-journey.md): fresh initialization, hidden empty/text input, generation, binary editor input/editing, copy/expiry, move/delete, history/backups and declined/confirmed deletion recovery. Final names/bytes, deep health and cleanup are checked. | Bounded cohesive journey covered with fixture clipboard tools and labeled human metadata output. Real headless Sway/Wayland passed in run 34003205577; broader shell installation remains open; targeted real Fish completion behavior already has its separate gate. |
| J2 | Existing pa store | [Pinned real-pa harness](../scripts/acceptance-pa.py): Git/no-Git adoption, alternating CRUD, deep verification, mutual OpenSSH loopback enrollment/sync, rollback, preserved metadata and identity. | Bounded fixture journey is covered. Keep distinct from live-user adoption, which is deferred, and physical-host failure acceptance in J4. |
| J3 | Noninteractive agent | [Agent exact-byte test](../internal/cli/cli_test.go), [native run](../internal/cli/run_test.go), machine refusal cases in the TTY harness and transfer input tests. | Partial: [34-leaf machine matrix](machine-contract-matrix.md) and Git/no-Git no-implicit-stdin/envelope/refusal baseline now exist. [Native authorized journey](../scripts/acceptance-agent.py) covers 26 successful leaves with Git and 22 without Git, including descriptor input, process replacement, transfer, rotation, disaster restore, and prune. Remaining specialized leaves and all input/authority combinations remain open. |
| J4 | Two-host operation | [Current/previous sessions](../internal/remote/remote_test.go), [one-sided commit/retry](../internal/remote/partial_test.go), real OpenSSH loopback in J2; historical Mac/devbox receipt in the development ledger. | Open: reproducible physical-host failure-after-one-direction and retry proof. Loopback, in-process interruption, and the successful manual two-host drill are separate evidence. |
| J5 | Trust change | [Combined Git/no-Git journey](trust-change-acceptance.md) covers one-sided enrollment, rotation mismatch, explicit repin, fresh-session replay refusal, mutual dry-run and exact-byte sync on both protocols. A positively controlled plugin sentinel detects premature decryption; refused operations preserve both stores' paths/modes/digests. | Bounded combined fixture covered. Protocol review and physical-host failure acceptance remain separate; the sentinel does not instrument every crypto parser call. |
| J6 | Identity lifecycle | [Continuity/destruction](../internal/store/rotation_test.go), [killed rotation](../internal/store/rotation_crash_test.go), explicit historical recovery and sealed chains. | Open: complete staging/orphan and irreversible-retirement copy audit, including interruption before the recovery journal is published. |
| J7 | Disaster recovery | [Full archive](../internal/store/archive_test.go), [logical transfer](../internal/store/transfer_test.go), TTY passphrase/plugin restore cases. | [Composite restore](../internal/store/archive_composite_test.go) now combines live/deleted values, history, peers, snapshots, receipts, and two retired identities; complete paths/bytes/private modes and post-restore recovery pass on Git/no-Git and absent/empty targets. [36 handled/SIGKILL restore cases](archive-recovery.md) now cover staged/publication boundaries and retry. Orphan identification/cleanup and power-loss proof remain open. |
| J8 | Scoped infrastructure recovery | [Scoped export/import](../internal/store/transfer_test.go), [CLI input boundaries](../internal/cli/transfer_input_test.go), explicit manifest and TTY passphrase isolated recovery in the interactive harness. | [Combined Git/no-Git CLI journey](scoped-recovery.md) covers an explicit synthetic infrastructure manifest, selected-only receipt/capsule, isolated verification/read, invalid selectors, circular full protection and whole-archive substitution refusal. The vault requires an explicit chosen inventory, not fixed names. Actual credential selection, offline custody and revocation remain deployment responsibilities; no live secrets were used. |
| J9 | Failure recovery | Transaction/rotation/prune/permission crash tests; [peer killed owners](../internal/store/peer_crash_test.go), [receipt reconciliation](../internal/store/peer_recovery_test.go), [handled errors](../internal/store/peer_error_test.go); [journal-publication ownership](journal-publication.md) now preserves staging/lock after publication errors and covers handled/killed recovery. | Open: every mutation's staged/published/receipt/cleanup boundaries. [Initialization cleanup](../internal/store/init_cleanup_test.go) covers handled cancellation, real cleanup denial and post-publication stage-name reuse on Git/no-Git stores. [Adoption finalization](../internal/store/adopt_cleanup_test.go) covers handled failures, stage/lock cleanup denial, changed lock ownership and reused stage names with accurate applied-state evidence and unchanged live pa material. [Killed adoption](../internal/store/adoption_crash_test.go) now covers bound-empty, staged and published recovery with explicit state evidence; the public CLI accepts only bound unadopted candidates. [Failed adoption recovery retry](../internal/store/adoption_retry_test.go) proves replacement-token/binding retention after real staging-removal denial and successful ordinary retry following fixture-mode restoration. [Initialization/adoption synchronization](../internal/store/initialization_sync_test.go) verifies all staged files, including Git-created files, precede bottom-up directory synchronization before publication. Killed initialization siblings and legacy unbound adoption staging remain separate. Current post-publication peer cases are not all pre-publication cases, nor physical power-loss proof. |

## Twelve compatibility fixture families

| ID | Contract fixture | Evidence and boundary | Status |
| --- | --- | --- | --- |
| F1 | Pristine shell-pa Git store, native X25519 | Real-pa harness extracts exact predecessor `f75734b8775f72d5d2f9630c08c2b48bdb6d8104` and builds with pinned age tools. | Bounded evidence. |
| F2 | Untracked/Git stores, nested names, rename/deletion/history | Real-pa Git/no-Git variants plus store history and backup fixtures. | Bounded evidence across fixtures; composite history journey remains J1. |
| F3 | Empty/newline/NUL/invalid UTF-8/large bytes | Real-pa values include empty, newline, binary, and 256 KiB payload; agent and transfer tests preserve exact bytes. | Bounded evidence. |
| F4 | Mock/missing/interactive plugin outcomes | [Plugin process fixture](../internal/crypt/plugin_fixture_test.go), [missing/debug guard](../internal/crypt/plugins_test.go), TTY PIN refusal/cancellation. | Bounded evidence; hardware lane is allowed opt-in. [Plugin shutdown](plugin-lifecycle.md) now has graceful/SIGKILL/reaping tests and a native malformed-response gate; protocol stalls and arbitrary descendants are separate limits. |
| F5 | Legacy pa-xfer pins never confer mutual trust | [Pinned predecessor-format fixture](../internal/remote/legacy_pins_test.go): Git/no-Git adoption preserves root `peers/*.json`, imports no Fulla authorization, refuses one-sided enrollment, and permits dry-run after reciprocal Fulla enrollment on both supported protocols. | Bounded fixture covered. This uses the predecessor schema at `f75734b`, not execution of the old pa-xfer binary; real shell-pa acceptance is J2. |
| F6 | Shell mutations before/after adoption and rollback | Real-pa harness performs all three, including rollback after Fulla sync activation. | Bounded evidence. |
| F7 | Previous/current/malformed/future metadata domains | [Metadata matrix](../internal/store/metadata_test.go), [live server-domain checks](../internal/remote/domain_test.go), [transaction backup-domain guards](../internal/store/transaction_domain_test.go), [actual historical binary upgrade/rollback](../scripts/acceptance-upgrade.py), old unbound lock-record cases. | Partial: [metadata policy](metadata-compatibility.md) and transaction-owned snapshots preserve CRUD with newer backup metadata; actual domain transformations and writes with a future transaction domain remain open. |
| F8 | Interrupted domain migrations | No complete migration engine/fixture series was identified. Adopted pa stores and same-format recovery are not domain migration proof. | Open: old state, staging, publication, retry, rollback boundary. |
| F9 | Disjoint/equal/different inventories and one-sided commit | Remote session and partial-retry fixtures, plus J2 OpenSSH integration. | Bounded protocol evidence; physical-host failure remains J4. |
| F10 | Current/current, current/previous, obsolete refusal | Remote tests run protocols 2/1 and reject an older generation. | Bounded protocol evidence; distinct released-binary rolling upgrade remains a stable-release gate. |
| F11 | Mismatch/unpaired/replay/filesystem/dirty Git attacks | Remote auth/domain tests; securefs/ACL tests; [Git conversion](../internal/store/git_conversions_test.go), [internal Git routing](git-routing.md), store permissions/history tests. | Partial: individual adversarial cases exist; whole-surface trust/decryption-order audit remains open. |
| F12 | Bundles/history/backups/retired identities/full archives | Transfer, archive, backup, rotation and stream tests. | Bounded component evidence; combined restore/state-completeness gap is J7. |

## Sixteen security invariants

These are universal contract claims. A linked test is evidence for a case, not a
universal proof. Every invariant remains subject to the final implementation and
security review before stable release.

| ID | Invariant | Evidence inspected | Remaining proof or limitation |
| --- | --- | --- | --- |
| I1 | No implicit creation/adoption/replacement | Agent init/add refusals; no-replace securefs, transfer, restore and peer tests. | Audit every command, destination type, and competing-writer publication boundary. |
| I2 | Preflight before decryption | Config validation, rooted securefs reads, run mapping preflight, peer authentication checks. | Complete operation-by-operation ordering review, including Git configuration and plugin boundaries. |
| I3 | Changed/unauthorized peer causes zero local decryption/mutation | [Trust-change sentinel](trust-change-acceptance.md): sole plugin identity, positive local-read controls, unchanged store bytes/paths/modes on unpaired/mismatch/replay refusals; premature client/server reads fail negative controls. | Bounded both-protocol/Git-mode evidence; final whole-surface ordering and protocol review remain open. |
| I4 | Shared lock and recoverable publication for every mutation | Existing journals, killed-writer and handled-error tests; peer receipt bindings. | J9 remains incomplete; lock release, pre-journal staging, and all export/restore/adoption boundaries need enumeration. |
| I5 | No shared-name overwrite/plaintext comparison; accurate retry | Plan partitions names; scoped import skips; interrupted reply tests verify exact committed bytes and convergence. | Physical failure receipt and remaining activation/finalization cases. |
| I6 | Explicit secret input/output channels only | stdin/fd/TTY/editor/clipboard/run surfaces; raw and base64 show tests. | Complete per-command flag applicability and channel matrix (J3). |
| I7 | No argv/ambient secret-value input or diagnostic disclosure | Parser/input tests, fixed failure messages, plugin debug guard, diagnostic redaction and panic handling. | Complete error-path and subprocess environment/output audit, including plugin waits and Git execution. |
| I8 | JSON/noninteractive never prompt; typed early refusal | TTY harness machine-mode cases, plugin refusal, CLI authority checks. | Every command/workflow row, not just tested interactive surfaces. |
| I9 | 0600 files/0700 directories, no replacement, fail closed | securefs permissions/ACL/path tests, artifact and editor checks. | All staging/cleanup/restore/publication paths and supported-filesystem fault cases. |
| I10 | Conditional clipboard clearing; delayed digest only | Clipboard worker implementation and fake/real X11/macOS harness. | OS clipboard read/clear lacks atomic compare-and-clear; do not claim race-free ownership. Real headless Sway/Wayland acceptance passed; other compositor implementations are not covered. |
| I11 | Validate mappings, no shell, native process replacement | [run implementation](../internal/cli/run.go), PID/exit/environment subprocess test. | Native PID, exit status and direct SIGINT/SIGTERM termination have subprocess proof. A native PTY gate covers controlling/foreground terminal inheritance, attached/detached stdin and terminal-generated Ctrl-C. [Mapping refusal sentinel](../internal/cli/run_preflight_test.go) covers eleven invalid invocation cases on Git/no-Git stores with zero plugin invocation, stdin reads or store mutation; positive and premature-read negative controls validate the detector. NUL-value rejection requires decryption; remaining value/authority combinations, inherited signal masks and job suspension remain separate. |
| I12 | Fresh one-use session-bound pinned challenges | Strict challenge, expiry, same-session/fresh-session replay tests for both protocols. | Final adversarial protocol review; do not infer every transport failure from replay tests. |
| I13 | Verify new identity before rotation; sealed historical continuity | Rotation staging/continuity/destruction and killed-boundary fixtures. | Complete retired/private copy lifecycle and pre-journal cleanup audit (J6). |
| I14 | Independent full-archive protection; empty-target restore | Archive circular-protection, corrupted archive, target-race, and confirmation tests. | Composite full-state restore and all publication interruptions (J7). |
| I15 | Redacted release panic output, no secret crash artifact | CLI recovery boundary, plugin panic redaction, explicit diagnostics; [crash policy](crash-diagnostics.md) tests default redaction and explicit development stacks in human/JSON subprocesses. | Deliberate final-release-binary panic acceptance and goroutine/runtime containment audit remain open. Development stacks now require the compile-time `fulla_debug` tag. |
| I16 | No same-Unix-user isolation claim | README trust boundary and canonical spec; explicitly trusted editor/plugin/process model. | Preserve wording through final help/docs/release review; no isolation feature is implied. |

## Canonical command-tree accounting

The locked tree has 34 command leaves. Parsing or building a leaf is not workflow
acceptance. All leaves must remain accounted for in the machine/TTY matrix.

| Surface | Canonical leaves | Acceptance work still open |
| --- | --- | --- |
| Daily/store | `init`, `add`, `show`, `copy`, `edit`, `list`, `remove`, `move` | Complete Git/no-Git human journey, input modes, permanent deletion/backups semantics, refused authority and interrupted initialization/adoption. |
| Process/transport | `run`, `sync`, `remote serve` | Native process behavior matrix; physical/reproducible partial sync and remaining transport failure cases. |
| Peer trust | `peer add`, `peer list`, `peer show`, `peer rotate`, `peer remove` | Legacy pa-xfer fixture, human guidance/confirmation review, pre-publication boundaries, rollback compatibility of recovery bindings. |
| Identity | `identity show`, `identity rotate` | Hardware/plugin lifecycle, full private-copy destruction and recovery audit. |
| Logical recovery | `transfer export`, `transfer verify`, `transfer import` | Full machine/input/manifest refusal table and all interrupted artifact/import boundaries. |
| History | `history list`, `history show`, `history restore` | Complete human/agent journey and source/applied evidence review. |
| Backups | `backup list`, `backup show`, `backup restore`, `backup prune`, `backup export` | Full-state composite restore, pre-publication cleanup, permanent-delete/retained-snapshot semantics. |
| Inspection | `status`, `doctor` | Every failure/partial-state machine result, structural versus deep scope, permission repair boundaries. |
| Expert/distribution | `git`, `completion`, `version` | Git execution/config/output bounds, broader Fish integration behavior, installed version/distribution proof. |

## Release and delivery gates

| Deliverable | Current evidence | Unfinished work |
| --- | --- | --- |
| Public Agensfield repository | Public `agensfield/fulla`, pushed main; scoped history and CI receipts in the ledger. | Keep exact-SHA verification and clean worktree at each release checkpoint. |
| Four-platform binaries/checksums/source | [Packager and acceptance](distribution.md), repeated byte-identical builds, native smoke on Linux/macOS CI. | These are untagged development artifacts, not publication approval or installed-user acceptance. |
| Preview `v0.1.0` | CI covers race/vet, TTY, clipboard, real pa, and reproducible packages. | Complete required journeys, compatibility/migration gates, known limitations, [Tag-gated workflow](distribution.md) is implemented; actual hosted publication/attestation/asset verification and tagged go-install proof remain open. |
| Agensfield Homebrew tap | [Formula generator](distribution.md) validates package hashes and expected binary provenance, emits four-platform URLs/checksums, completions and a no-Git read/write test. Synthetic refusal cases and Ruby syntax pass. | Generate from accepted release artifacts, publish only Fulla's formula, and prove actual install/completion/brew-test behavior; no tap mutation has been performed. |
| Stable `v1.0.0` | Isolated manual Mac/devbox drill and protocol-version fixtures exist. | Repeated real-store use on two real hosts, disaster/rotation drills, distinct-version rolling upgrade, signed artifacts and security review. Live-user cutover remains deferred. |
| Vault closure | Scoped implementation ledger and overview are regularly published. | Final requirement-by-requirement closure, with unresolved items explicitly retained; current goal remains active. |

## Next implementation order

1. Resolve F7/F8: a domain migration / older-binary recovery policy. Current version guards must not stand in for it.
2. Complete J3 and J1 with an explicit command/input/authority table, then fill
   missing human/runtime proofs (including Wayland where applicable).
3. Complete J6/J7/J9 staging, composite restore, private-copy, and interruption
   coverage. Review plugin cancellation and Git execution/configuration boundaries.
4. Re-run the required journeys at one accepted source SHA, then finish preview
   publication, installation/tap acceptance, and scoped vault closure. Preserve
   stable-release operational gates separately; do not silently waive them.
