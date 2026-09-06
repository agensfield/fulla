# Metadata compatibility and recovery boundaries

The live `pa-v1` layout is independent of Fulla feature metadata. Fulla does not
rewrite `identities`, `recipients`, password paths, or Git history merely because
a metadata reader encounters a different version. Missing, malformed, duplicate,
nonpositive, and unsupported root metadata fail closed rather than being treated
as a migration request.

## Current domain behavior

All originally written manifests, starting at `1999674`, declare version 1 for
`peers`, `sync`, `backup`, `identity`, and `transactions`. There is no historical
version-0 domain schema. New readers directly accept the original version-1
records; they do not fabricate or silently rewrite a predecessor format.

| Domain newer than this binary understands | Independent live operations | Domain operations |
| --- | --- | --- |
| Peers | Basic CRUD remains independent of peer records. | Peer lookup/mutation and authentication refuse. |
| Sync | Basic local CRUD remains independent of sync state. | Sync and activation refuse, including open remote sessions. |
| Backup | CRUD retains encrypted snapshots in the understood transaction domain. | Backup listing, restoration, pruning, and full export refuse unknown backup metadata. |
| Identity | Live keys still use pa-v1; basic CRUD does not interpret retired identity records. | Identity lifecycle and historical-key recovery refuse unknown identity metadata. |
| Transactions | Unlocked live reads remain possible. | Mutations and transaction recovery currently require version 1. A wider safe-write policy for a future transaction protocol is unresolved. |

The last row remains a compatibility limitation, not a waiver of the locked
older-binary CRUD contract. No released metadata upgrade is introduced here.

## Transaction snapshots with a newer backup domain

When `transactions` is understood and `backup` is a well-formed newer version,
CRUD writes use `.fulla/transaction-backups/ID` for complete encrypted before/after
snapshots. They do not create, update, enumerate, or reinterpret files under
`.fulla/backups`. The live manifest is unchanged. Mutation results identify the
actual snapshot path in `backup` and retain the usual transaction and receipt IDs.

The operation records `snapshot_domain: "transactions"` in its version-1
transaction journal before publishing any live change. Recovery follows that
binding even if the backup version changes later. Unknown binding values are
rejected. Ordinary backup-domain journals omit this field, retaining the existing
serialized shape and their backup-version guard. A journal originally targeting
the backup domain never switches destination after an interruption.

Once a compatible backup reader is available, it discovers both snapshot
locations, validates the journal's domain and ID, rejects duplicate IDs across
locations, and restores/prunes using the recorded location. Prune journals retain
the selected domain so interrupted deletion does not depend on rediscovery.
Recovery and backup operations continue to enforce their respective domain gates.
Never change a real manifest version to bypass an unsupported reader. Tests that
restore an injected fixture version do not demonstrate a real format downgrade.

## Rollback boundary

A pre-feature binary does not list the newer transaction snapshot directory.
Retain that directory and use a supporting binary for recovery/retention.
Do not use historical development binaries for writes with a newer backup
manifest: versions before `d51bf81` have the missing backup-domain guard described
in the development ledger. Use a supporting Fulla binary or deliberate shell-pa
basic rollback; do not infer safe historical writes from their ability to read
the live pa-v1 files.
An interrupted transaction or prune journal containing `snapshot_domain` requires
a supporting binary. Readers that know the operation's journal but predate this
field reject it through strict JSON decoding. Development binaries predating
prune-journal support, including `831caf6`, do not recognize pending prune state
at all and must not be used for its recovery; they can incorrectly release its
lock. The historical-binary fixture below proves transaction refusal only.
Finish pending operations with a supporting binary before rolling back. Never
remove a lock or journal to make an older binary proceed.

The real historical-binary harness pins `831caf6`, creates a killed transaction,
checks that the historical recovery refuses the journal without changing data or
pending evidence, then retries using current Fulla. The historical recovery does
claim a new lock owner before refusing; the retry explicitly reads that owner's
new token. This is a tested development-build boundary, not released-binary
rolling-upgrade proof.

## Evidence and remaining work

See `internal/store/snapshot_domain_test.go`, `snapshot_crash_test.go`,
`transaction_domain_test.go`, and `scripts/acceptance-upgrade.py`. Covered cases
include Git/no-Git CRUD, exact encrypted before/after snapshots, unknown-domain
preservation, killed publication/commit/receipt phases, destination binding,
restore/prune, duplicate-location refusal, and historical-binary recovery refusal.

The full acceptance matrix still tracks domain-upgrade transformations and their
interruption boundaries, future transaction-domain writes, and release/stable
operational gates. This policy does not declare those complete.

## Additive staging ownership in lock records

Current writers persist `stage_id` in `lock/info` after creating an empty stage
and before placing private material there. Recovery preserves that binding when
claiming a new owner token, validates it against any pending journal, and can
remove only that bound stage when no journal exists. Malformed/duplicate IDs and
conflicting peer-receipt bindings fail closed. This optional field leaves domain
versions unchanged; it is not a migration engine.

Old binaries may ignore the field and release the lock without cleaning the
stage. Recover with a supporting binary before rollback. Unbound historical
staging remains inspection-only, including the empty-directory window between
creation and binding. See [journal publication](journal-publication.md) for
killed-writer and failed-cleanup retry evidence and the synthetic legacy schema
boundary. No distinct released-binary upgrade is claimed by these tests.

## Adoption recovery binding

New adoption writers bind `adoption_id` into the existing shared lock after
creating the empty `.fulla-adopt-ID` directory and before writing metadata. The
ID is also the intended store ID. Recovery preserves this binding across owner
takeover. It rejects duplicate/malformed IDs, conflicting transaction/peer
bindings, pending journals, mismatched published store IDs and unsupported
published domain versions. This additive lock field does not upgrade a domain.

`fulla doctor --recover-lock TOKEN` can explicitly open a compatible unadopted
candidate, but recovery refuses an unadopted candidate without this binding.
It still requires the inspected token and a provably dead local owner, with
exclusive recovery locking and revalidation. It never adopts implicitly:
unpublished recovery removes only the bound metadata stage, releases the lock,
and reports `adoption_applied: false`. Retry adoption explicitly. Published
recovery verifies the matching store ID, synchronizes publication, releases the
lock and reports `adoption_applied: true`. It does not remove a new occupant at
the former stage pathname. Neither path decrypts or rewrites live pa material.

Handled staging cleanup failures now retain the adoption lock/binding for
inspection instead of orphaning their recovery evidence. Unsafe modes still block ordinary recovery. The existing combined permission-
repair path is restricted to interrupted permission repair, so unsafe-mode
adoption cleanup requires separate inspection; this change does not bypass
filesystem validation or claim that combined repair supports adoption. Recovery errors after state
classification include applied-state and cleanup evidence.

Git/no-Git SIGKILL fixtures cover empty-bound, staged and published adoption,
wrong/live-owner refusal, exact unchanged live paths/modes/digests, preserved
reused stage paths, repeated missing-lock refusal and explicit unpublished
adoption retry. A public CLI fixture distinguishes bound from unbound candidates.
Malformed/conflicting binding and published-metadata tests fail without changing
evidence. The mkdir-before-binding window contains only an empty, unbound
directory; legacy unbound adoption staging remains inspection-only.

Use a supporting Fulla binary for pending adoption recovery before rollback.
Older development binaries may ignore the new field, cannot recover pre-adoption
state through their CLI, and do not implement bound-stage cleanup. No actual
released-binary migration or physical power-loss acceptance is claimed here.

### Retry after failed adoption recovery

A Git/no-Git fixture now denies staging removal after recovery validation using
real chmod permissions. The recovery subprocess publishes its replacement
owner, fails cleanup, emits applied=false and cleanup path/lock evidence, and
exits. Its dead owner token differs from the initial token and still carries
adoption_id. After restoring only the deliberately changed fixture mode,
ordinary recovery rejects the old token without mutation, accepts the newly
inspected token, removes the stage/lock and allows explicit adoption retry.
Live pa paths/modes/digests and history are unchanged throughout recovery.

The initial dead owner is a reconstructed fixture using an actually reaped PID;
the failed recovery is a real subprocess, and the final retry calls ordinary
Recover. Existing SIGKILL adoption cases remain separate evidence. A negative
overlay dropping adoption_id during takeover fails the retained-binding check.
This proves unpublished-stage failure/retry, not post-publication lock-release
retry, arbitrary unsafe-mode repair or physical power-loss durability.

## Mutation eligibility before secret access

Entry mutations now check transaction and snapshot-domain eligibility before
acquiring their lock, repeat it during locked validation, and retain the existing
publication-time check. Add/edit (including input callbacks), remove/move,
history restore, backup restore and logical import use this shared boundary.
An unsupported transaction domain or invalid backup declaration therefore cannot
cause these store operations to decrypt first and only then refuse publication.
Newer, well-formed backup domains still use transaction-owned snapshots.

`mutation_preflight_test.go` uses real encrypted entries/bundles and a sole plugin
identity with a positively controlled invocation sentinel. Git/no-Git cases cover
future transactions and a missing backup declaration, assert typed refusal,
no plugin/input/confirmation invocation, and unchanged paths, modes and bytes.
A previous-source overlay makes interactive edit attempt decryption and fail the
regression. Existing future-backup CRUD/snapshot cases remain positive coverage.

The CLI also calls the advisory domain check before add/edit input and before
selecting logical-import identities or passphrases. Forty Git/no-Git cases in
`internal/cli/mutation_input_test.go` cover JSON/noninteractive add/edit stdin and
inherited descriptors, plus import passphrase descriptors. Refusal leaves selected
FDs open at offset zero, never reads stdin, and preserves file bytes/modes. A
previous-CLI overlay consumes/closes the import passphrase descriptor and fails
the regression. Store locked/publication checks remain authoritative.

These selected channels are bounded evidence, not every possible CLI input or
racing state change. Future transaction-domain writes and real domain migrations
remain unresolved.


## Domain-upgrade applicability audit

[The domain-upgrade audit](domain-upgrade-audit.md) separates the absence of a
real predecessor domain version from the actual future-transaction CRUD gap.
It maps shared journal/staging/lock ownership and explains why removing a guard
or relocating snapshots alone is unsafe. Transformation-fixture applicability
has been returned to Arda as a product-scope question; no gate is waived.


## Initialization recovery binding

Current initialization binds init_id and the encoded intended destination into
its shared lock before creating keys, retaining the lock through publication.
[Initialization recovery](init-recovery.md) documents explicit partial-stage
cleanup, published-store finalization, killed-owner and failed-cleanup retry
fixtures, and the development-reader rollback boundary. Domain versions do not
change; legacy unbound staging is not granted deletion authority.
