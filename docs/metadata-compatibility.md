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
a supporting binary: older strict JSON readers reject the new field. Finish the
pending operation before rolling back. Never remove its lock or journal to make
an older binary proceed.

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
