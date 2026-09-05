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
| J1 | Fresh human store | [TTY/editor harness](../scripts/acceptance-interactive.py), [clipboard harness](../scripts/acceptance-clipboard.py), [history](../internal/store/history_confirmation_test.go), [snapshots](../internal/store/backup_confirmation_test.go). | Open: one traceable complete Git/no-Git human journey including deletion recovery; real Wayland and Fish behavior remain unverified. Existing checks are distributed across fixtures. |
| J2 | Existing pa store | [Pinned real-pa harness](../scripts/acceptance-pa.py): Git/no-Git adoption, alternating CRUD, deep verification, mutual OpenSSH loopback enrollment/sync, rollback, preserved metadata and identity. | Bounded fixture journey is covered. Keep distinct from live-user adoption, which is deferred, and physical-host failure acceptance in J4. |
| J3 | Noninteractive agent | [Agent exact-byte test](../internal/cli/cli_test.go), [native run](../internal/cli/run_test.go), machine refusal cases in the TTY harness and transfer input tests. | Open: every canonical command/input/authority combination needs an explicit machine-result and no-prompt table. The existing agent test primarily covers init/add/show/parser behavior. |
| J4 | Two-host operation | [Current/previous sessions](../internal/remote/remote_test.go), [one-sided commit/retry](../internal/remote/partial_test.go), real OpenSSH loopback in J2; historical Mac/devbox receipt in the development ledger. | Open: reproducible physical-host failure-after-one-direction and retry proof. Loopback, in-process interruption, and the successful manual two-host drill are separate evidence. |
| J5 | Trust change | [Mismatch/unpaired tests](../internal/remote/remote_test.go), [strict challenge and replay](../internal/remote/auth_test.go), peer rotation/recovery tests, earlier Mac/devbox rotate/repin drill. | Open: a combined traceable mismatch/repin/mutual-authorization/replay journey, including explicit evidence of zero local decryption on failed trust. |
| J6 | Identity lifecycle | [Continuity/destruction](../internal/store/rotation_test.go), [killed rotation](../internal/store/rotation_crash_test.go), explicit historical recovery and sealed chains. | Open: complete staging/orphan and irreversible-retirement copy audit, including interruption before the recovery journal is published. |
| J7 | Disaster recovery | [Full archive](../internal/store/archive_test.go), [logical transfer](../internal/store/transfer_test.go), TTY passphrase/plugin restore cases. | Open: one composite restore fixture containing all supported state classes (history, peers, snapshots, retired identities), with whole-state comparison and interrupted publication/cleanup. |
| J8 | Scoped infrastructure recovery | [Scoped export/import](../internal/store/transfer_test.go), [CLI input boundaries](../internal/cli/transfer_input_test.go), explicit manifest and TTY passphrase isolated recovery in the interactive harness. | Bounded synthetic manifest flow exists. Open: reconcile the exact Agensfield manifest/overreach contract with a complete documented recovery receipt; do not use live secrets merely to fill a test gap. |
| J9 | Failure recovery | Transaction/rotation/prune/permission crash tests; [peer killed owners](../internal/store/peer_crash_test.go), [receipt reconciliation](../internal/store/peer_recovery_test.go), [handled errors](../internal/store/peer_error_test.go). | Open: every mutation's staged/published/receipt/cleanup boundaries. Current post-publication peer cases are not all pre-publication cases, nor physical power-loss proof. |

## Twelve compatibility fixture families

| ID | Contract fixture | Evidence and boundary | Status |
| --- | --- | --- | --- |
| F1 | Pristine shell-pa Git store, native X25519 | Real-pa harness extracts exact predecessor `f75734b8775f72d5d2f9630c08c2b48bdb6d8104` and builds with pinned age tools. | Bounded evidence. |
| F2 | Untracked/Git stores, nested names, rename/deletion/history | Real-pa Git/no-Git variants plus store history and backup fixtures. | Bounded evidence across fixtures; composite history journey remains J1. |
| F3 | Empty/newline/NUL/invalid UTF-8/large bytes | Real-pa values include empty, newline, binary, and 256 KiB payload; agent and transfer tests preserve exact bytes. | Bounded evidence. |
| F4 | Mock/missing/interactive plugin outcomes | [Plugin process fixture](../internal/crypt/plugin_fixture_test.go), [missing/debug guard](../internal/crypt/plugins_test.go), TTY PIN refusal/cancellation. | Bounded evidence; hardware lane is allowed opt-in. Process wait/termination behavior still needs review. |
| F5 | Legacy pa-xfer pins never confer mutual trust | [Pinned predecessor-format fixture](../internal/remote/legacy_pins_test.go): Git/no-Git adoption preserves root `peers/*.json`, imports no Fulla authorization, refuses one-sided enrollment, and permits dry-run after reciprocal Fulla enrollment on both supported protocols. | Bounded fixture covered. This uses the predecessor schema at `f75734b`, not execution of the old pa-xfer binary; real shell-pa acceptance is J2. |
| F6 | Shell mutations before/after adoption and rollback | Real-pa harness performs all three, including rollback after Fulla sync activation. | Bounded evidence. |
| F7 | Previous/current/malformed/future metadata domains | [Metadata matrix](../internal/store/metadata_test.go), [live server-domain checks](../internal/remote/domain_test.go), old unbound lock-record cases. | Partial: version checks exist; actual prior/current domain transformations and safe older-binary CRUD policy are not fully proved. |
| F8 | Interrupted domain migrations | No complete migration engine/fixture series was identified. Adopted pa stores and same-format recovery are not domain migration proof. | Open: old state, staging, publication, retry, rollback boundary. |
| F9 | Disjoint/equal/different inventories and one-sided commit | Remote session and partial-retry fixtures, plus J2 OpenSSH integration. | Bounded protocol evidence; physical-host failure remains J4. |
| F10 | Current/current, current/previous, obsolete refusal | Remote tests run protocols 2/1 and reject an older generation. | Bounded protocol evidence; distinct released-binary rolling upgrade remains a stable-release gate. |
| F11 | Mismatch/unpaired/replay/filesystem/dirty Git attacks | Remote auth/domain tests; securefs/ACL tests; [Git conversion](../internal/store/git_conversions_test.go), store permissions/history tests. | Partial: individual adversarial cases exist; whole-surface trust/decryption-order audit remains open. |
| F12 | Bundles/history/backups/retired identities/full archives | Transfer, archive, backup, rotation and stream tests. | Bounded component evidence; combined restore/state-completeness gap is J7. |

## Sixteen security invariants

These are universal contract claims. A linked test is evidence for a case, not a
universal proof. Every invariant remains subject to the final implementation and
security review before stable release.

| ID | Invariant | Evidence inspected | Remaining proof or limitation |
| --- | --- | --- | --- |
| I1 | No implicit creation/adoption/replacement | Agent init/add refusals; no-replace securefs, transfer, restore and peer tests. | Audit every command, destination type, and competing-writer publication boundary. |
| I2 | Preflight before decryption | Config validation, rooted securefs reads, run mapping preflight, peer authentication checks. | Complete operation-by-operation ordering review, including Git configuration and plugin boundaries. |
| I3 | Changed/unauthorized peer causes zero local decryption/mutation | Client checks greeting against pin before Keys; server denies unpaired access; mismatch tests. | Add an explicit decryption-count/sentinel negative-control fixture, not only an error assertion. |
| I4 | Shared lock and recoverable publication for every mutation | Existing journals, killed-writer and handled-error tests; peer receipt bindings. | J9 remains incomplete; lock release, pre-journal staging, and all export/restore/adoption boundaries need enumeration. |
| I5 | No shared-name overwrite/plaintext comparison; accurate retry | Plan partitions names; scoped import skips; interrupted reply tests verify exact committed bytes and convergence. | Physical failure receipt and remaining activation/finalization cases. |
| I6 | Explicit secret input/output channels only | stdin/fd/TTY/editor/clipboard/run surfaces; raw and base64 show tests. | Complete per-command flag applicability and channel matrix (J3). |
| I7 | No argv/ambient secret-value input or diagnostic disclosure | Parser/input tests, fixed failure messages, plugin debug guard, diagnostic redaction and panic handling. | Complete error-path and subprocess environment/output audit, including plugin waits and Git execution. |
| I8 | JSON/noninteractive never prompt; typed early refusal | TTY harness machine-mode cases, plugin refusal, CLI authority checks. | Every command/workflow row, not just tested interactive surfaces. |
| I9 | 0600 files/0700 directories, no replacement, fail closed | securefs permissions/ACL/path tests, artifact and editor checks. | All staging/cleanup/restore/publication paths and supported-filesystem fault cases. |
| I10 | Conditional clipboard clearing; delayed digest only | Clipboard worker implementation and fake/real X11/macOS harness. | OS clipboard read/clear lacks atomic compare-and-clear; do not claim race-free ownership. Real Wayland acceptance open. |
| I11 | Validate mappings, no shell, native process replacement | [run implementation](../internal/cli/run.go), PID/exit/environment subprocess test. | Bounded native-exec proof; complete invalid-mapping/no-decrypt cases and inherited signal/TTY behavior still need explicit acceptance. |
| I12 | Fresh one-use session-bound pinned challenges | Strict challenge, expiry, same-session/fresh-session replay tests for both protocols. | Final adversarial protocol review; do not infer every transport failure from replay tests. |
| I13 | Verify new identity before rotation; sealed historical continuity | Rotation staging/continuity/destruction and killed-boundary fixtures. | Complete retired/private copy lifecycle and pre-journal cleanup audit (J6). |
| I14 | Independent full-archive protection; empty-target restore | Archive circular-protection, corrupted archive, target-race, and confirmation tests. | Composite full-state restore and all publication interruptions (J7). |
| I15 | Redacted release panic output, no secret crash artifact | App/main recovery boundaries and plugin panic redaction; explicit diagnostics. | Deliberate release-binary panic acceptance and development/test stack policy reconciliation. |
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
| Expert/distribution | `git`, `completion`, `version` | Git execution/config/output bounds, real Fish behavior, installed version/distribution proof. |

## Release and delivery gates

| Deliverable | Current evidence | Unfinished work |
| --- | --- | --- |
| Public Agensfield repository | Public `agensfield/fulla`, pushed main; scoped history and CI receipts in the ledger. | Keep exact-SHA verification and clean worktree at each release checkpoint. |
| Four-platform binaries/checksums/source | [Packager and acceptance](distribution.md), repeated byte-identical builds, native smoke on Linux/macOS CI. | These are untagged development artifacts, not publication approval or installed-user acceptance. |
| Preview `v0.1.0` | CI covers race/vet, TTY, clipboard, real pa, and reproducible packages. | Complete required journeys, compatibility/migration gates, known limitations, tag-gated publication, asset verification, and go-install proof. |
| Agensfield Homebrew tap | Intended command is documented. | Fulla formula, release provenance, install and brew-test acceptance; no unrelated formula mutation. |
| Stable `v1.0.0` | Isolated manual Mac/devbox drill and protocol-version fixtures exist. | Repeated real-store use on two real hosts, disaster/rotation drills, distinct-version rolling upgrade, signed artifacts and security review. Live-user cutover remains deferred. |
| Vault closure | Scoped implementation ledger and overview are regularly published. | Final requirement-by-requirement closure, with unresolved items explicitly retained; current goal remains active. |

## Next implementation order

1. Resolve F7/F8: a domain migration / older-binary recovery policy. Current version guards must not stand in for it.
2. Complete J3 and J1 with an explicit command/input/authority table, then fill
   missing human/runtime proofs (including Fish and Wayland where applicable).
3. Complete J6/J7/J9 staging, composite restore, private-copy, and interruption
   coverage. Review plugin cancellation and Git execution/configuration boundaries.
4. Re-run the required journeys at one accepted source SHA, then finish preview
   publication, installation/tap acceptance, and scoped vault closure. Preserve
   stable-release operational gates separately; do not silently waive them.
