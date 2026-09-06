# Basic writes across future feature metadata

The v1 compatibility promise keeps safe pa-v1 CRUD available when feature-domain
metadata is newer. Merely bypassing the transaction-version check would let an
older reader write journals, stages and snapshots into a namespace it cannot
interpret. Basic writes now select a fixed, isolated protocol when the manifest's
transaction version is newer than understood. Malformed or missing required
domain declarations still refuse; a larger positive version is not malformed.

## Fixed v1 ownership

`basic-v1` is a reserved protocol, not another upgradeable feature domain. Its
codec, ownership rules and paths must remain readable throughout Fulla v1.
Future domain upgrades must preserve this namespace and the shared lock contract.
No manifest counter or live pa-v1 entry layout changes to enable it.

| Resource | Fixed path / binding |
| --- | --- |
| Staging | `.fulla/basic-v1/transactions/ID` |
| Pending journal | `.fulla/basic-v1/pending.json` |
| Encrypted snapshots | `.fulla/basic-v1/backups/ID` |
| Receipts | `.fulla/basic-v1/receipts/ID.json` |
| Shared owner | Existing `lock/owner` and `lock/info`, with `stage_id=ID stage_protocol=basic-v1` |
| Journal codec | Version 1, `snapshot_domain: basic-v1`, only add/edit/move/remove |

The same journal engine verifies old/new ciphertext hashes, stages changes,
performs Git commit reconciliation, retains encrypted snapshots and publishes a
receipt. Routing is explicit in the journal and lock binding. It does not read,
rewrite, delete or migrate unknown feature journals, snapshots or receipts.
A known transaction domain continues to use its existing path and snapshot
policy; existing successful stores do not acquire the extra directory eagerly.

Only basic entry operations use this fallback. Import, history/snapshot restore,
and identity rotation still require the transaction metadata they use. Rotation
now checks that requirement before private-key access and repeats it under the
shared lock. Non-basic journal and rotation completion also retain domain guards.
Basic snapshot records can be located by an understood backup reader; backup
operations still enforce their own domain and mutation eligibility checks.

## Interruptions and diagnostics

Any shared active lock or recognized pending-operation path blocks competing
writes, including the basic pending journal. A newer unfinished feature operation
cannot be bypassed just because basic CRUD has an independent codec. Unknown
future recovery data remains the responsibility of a supporting reader.

Explicit recovery requires a local dead owner, its exact token and a matching
supported staging protocol. Recovery validates the journal/ID/operation binding
before taking ownership and repeats validation under the recovery guard. It
preserves the protocol when publishing a replacement token. A competing feature
journal, unknown protocol, absent binding, malformed basic journal, wrong ID or
unsupported operation refuses without changing the inspected state.

Unpublished owned basic stages are removed; published journals finish through
the fixed codec. Failed cleanup retains the replacement owner for an ordinary
explicit retry. Basic stage paths participate in staging inspection even when
the separate feature-domain inspection reports an unsupported version. These
stages contain encrypted entries and encrypted Git history, never private keys.

## Evidence and compatibility limits

Git/no-Git tests run add, binary read, empty edit, move and remove with a future
transaction version and with all feature versions set to 99. Opaque feature
sentinels, manifest bytes and modes remain unchanged. JSON and noninteractive
human CLI journeys also cover stdin/descriptor input and raw/base64 retrieval.
These future-version fixtures establish isolation against unknown bytes; they do
not pretend a real production version-99 schema or migration exists.

Native subprocesses are killed at staged, journaled, first-path publication,
Git-committed and receipted boundaries. Two additional cases force real removal
denial in recovery after validation, then verify replacement-token/protocol
retention and successful retry after fixture-mode restoration. Malformed binding
cases are reconstructed fixtures and are distinct from those killed owners.

Development binaries predating this protocol do not understand basic staging or
snapshots. Source inspection shows staging-aware 1f39f93 refuses this bound recovery when
the transaction version is newer. Earlier 831caf6 lacks stage_id validation and
can release a lock without recognizing the basic journal; do not use it for
this recovery. This is a source-audited development limit, not new native binary
acceptance. Finish pending work using a supporting binary
before any rollback; manually reducing a manifest counter does not make older
recovery safe. This protocol is intended to establish the v1 guarantee before
v1 is released, not to retroactively add it to already-built development binaries.
The separate domain-transformation applicability question, physical power-loss
acceptance and released rolling-upgrade gate remain open.
