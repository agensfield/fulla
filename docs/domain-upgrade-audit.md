# Domain upgrade applicability and write compatibility

Status: transformation-fixture applicability remains unresolved, 2026-09-06.
The historical audit below was made at `f611f22`. Its basic-write gap is now
addressed by the [fixed isolated basic-v1 protocol](basic-write-protocol.md);
the original diagnosis is retained to explain why the guard alone was insufficient.
This record does not waive the locked specification or claim migration acceptance.

## Real predecessor evidence

The first implementation, `1999674`, already writes root metadata version 1,
profile `pa-v1`, and domain version 1 for peers, sync, backup, identity and
transactions. Current `newMetadata` declares the same versions. There is no
historical domain-v0 schema to transform. The pinned historical acceptance binary
is `831caf68655b41b4ca5b064b7693af35df68a6e6`, not a fabricated predecessor.

The existing historical-binary harness creates real stores, writes/edits entries,
rotates identity, and checks that the current reader verifies/restores the old
snapshots and history. Read-only acceptance preserves the old tree. Alternating
old/current CRUD and identity recovery exercise actual same-version rollback.
This proves supported historical layouts, not a domain-version transformation.

Persisted additions did occur within development version 1:

- `snapshot_domain` and transaction-owned snapshots change recovery interpretation
  when the backup feature version is newer.
- `stage_id` and `adoption_id` bind unpublished staging to lock ownership.
- Rotation publication copies now live inside the bound transaction directory.

Their old/new, interruption, retry and rollback evidence is recorded in
[metadata compatibility](metadata-compatibility.md),
[journal publication](journal-publication.md), and
[rotation publication](rotation-publication.md). A reader that predates a binding
may refuse or ignore it; these are explicit development compatibility limits,
not evidence that a versioned migration engine exists.

## Two separate open questions

### Applicability of transformation fixtures

The locked fixture list requires each domain in previous/current forms and
interrupted domain migrations. No real previous domain *version* currently exists.

Proposed interpretation, pending Arda's decision: keep the real historical layout
fixtures and make actual transformation/interruption fixtures mandatory with the
first domain-version upgrade. Do not introduce a v0 or change a working persisted
format just to make a migration test possible. An alternative is to require a
transactional migration engine before preview even without a current production
transformation. The acceptance matrix remains open until that scope is resolved;
this note does not choose the interpretation on the user's behalf.

### Safe writes with a future transaction domain

This is an existing implementation gap, independently of fixture applicability.
The locked v1 contract promises older-binary basic CRUD while refusing newer
feature/recovery metadata. Current Fulla refuses mutations when `transactions`
is newer. Moving that check before input makes refusal safer; it does not satisfy
the promised writes.

The transaction domain currently owns all of these paths and recovery meanings:

| Shared resource | Current reader/writer |
| --- | --- |
| `.fulla/transactions/ID` | `mutate`, `finishJournal`, staging binding/cleanup, destructive rotation inspection |
| `.fulla/pending.json` | Journal publication, `Unlocked`, takeover/recovery and final cleanup |
| `.fulla/transaction-backups/ID` | Fallback snapshots for a newer backup domain, listing/restore/prune |
| `lock/info` stage binding | Takeover validation and unpublished-stage cleanup |

Removing `RequireDomain("transactions")` would authorize writes and cleanup in
paths whose newer meaning this reader does not know. Merely changing the fallback
snapshot directory does not isolate the journal, lock binding or cleanup rules.
Neither is a safe implementation of the promised compatibility.

A concrete design must reserve an immutable basic-write/recovery protocol for v1,
or isolate it from upgradeable recovery state with explicit ownership/routing.
It must cover normal publication, killed writers, takeover, snapshots/prune,
unknown-domain preservation and historical-reader refusal of pending newer work.
That policy must precede a format change. No new directory layout, version bump,
automatic migration or weakened guard is introduced by this audit.

## Follow-through

Resolve transformation-fixture applicability separately from the basic-write
protocol. Keep both visible in F7/F8 until supported by an agreed interpretation
and implementation evidence. Other authorized staging/recovery and release work
can proceed while the product question is pending. Live credential adoption and
sync cutover remain deferred.
