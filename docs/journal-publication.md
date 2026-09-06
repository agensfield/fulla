# Recovery-journal publication failures

Transactions and identity rotation publish a prepared recovery journal before
changing live entries or identities. Atomic publication and directory durability
are distinct: `PublishNewPublished` can return `true` with an error when rename
succeeds but the following directory sync fails.

Both callers previously used `PublishNew`, which discarded the publication bit.
On this error path their deferred cleanup treated the operation as unpublished,
removed its staging, and released the shared lock. The remaining journal could
then refer to missing recovery data. This is a code-path defect established by
inspection and failure-injection tests, not a reported user-store incident.

Both callers now retain the publication bit even on error. A published journal
keeps its staging and owned lock. The error is `transaction.incomplete`, status 3,
with `transaction`, `recovery_required: true`, and `applied: false`: the journal
exists, but no live entry or identity has been published at this boundary. The
raw filesystem or callback error is not included. Durability is not claimed
after a directory-sync failure. Inspect the lock and use the normal explicit
recovery procedure once its owner has exited; never discard its journal/staging
or bypass a live owner.

Evidence:

- The securefs test injects an error after actual no-replace rename and verifies
  the API reports publication with an error, preserves the complete new file,
  and refuses replacement.
- Four Git/no-Git × transaction/rotation store cases inject failure at the new
  internal `journaled` boundary. They require retained staging/journal/lock,
  unchanged live entry and identity bytes, redacted status/details, refusal of
  ordinary reads, successful journal completion, and deep verification.
- A negative-control overlay discards published ownership again. All four store
  cases fail because staging is deleted, while the error remains typed.
- Existing transaction killed-owner recovery tests now include `journaled` on
  Git/no-Git; rotation tests include it for retained/destructive old-key policy.
  These kill a real child, enforce live-owner/wrong-token refusals, and invoke
  the public recovery operation after death.

The internal hook is not a runtime flag. Store tests inject after successful
journal publication; the lower-level securefs test separately covers sync-error
publication reporting. They do not simulate physical power loss or establish
that a failed fsync reached persistent media. Pre-journal killed-writer orphan handling and other publication surfaces remain
separate acceptance work.

## Unpublished staging cleanup

Handled failures before journal publication now remove only a directory whose
exclusive creation succeeded, synchronize its parent after removal, and release
the owned lock after successful cleanup. Failed cleanup with a recorded staging
binding retains the lock for explicit recovery. Both operations use the same cleanup
path. Cleanup or lock-release failures are no longer silently hidden behind the
original operation error.

`transaction.cleanup_failed` reports `applied: false`, `cleanup_required: true`,
and the applicable `staging_path`/`staging_cleanup_required` or
`lock_cleanup_required`. A staging path means removal or its durability could
not be confirmed; some or all files may already have been removed. A lock flag
means lock cleanup needs inspection, not that the original owner still owns it.
The original typed operation code is retained without its message/details;
signal statuses 129/130/131/143 are preserved. Other cleanup failures use status 1.
Human output quotes the staging path to escape control characters and directs
lock inspection to `fulla doctor`; JSON retains structured details.

Twelve real permission-denial cases cover Git/no-Git × transaction/rotation ×
ordinary error/refusal/signal. They verify cleanup evidence, unchanged existing
store files, no changes outside owned staging, unchanged live bytes and retained bound lock when cleanup fails. Rotation fixtures retain the generated staged identity to
make the private-copy risk concrete. A changed-lock-owner test verifies that its
files remain untouched and that its release failure is reported. Reinstating
ignored cleanup errors through a source overlay makes all 13 cases fail.

These are handled-failure tests on disposable non-root fixtures, not SIGKILL
cleanup or an automatic orphan-removal mechanism. Unpublished leftover staging
has no committed recovery journal. Current writers provide the separate owned
staging binding below; older unbound leftovers remain inspection-only.

## Read-only staging inventory

Doctor now reports `staging` as sorted paths relative to the store and adds
`transaction.staging_present` when the understood transaction directory is
nonempty. The report is unhealthy even if no lock or pending journal remains.
Human failure output labels these paths as inspection evidence, explicitly not
deletion authority. Inventory reads directory entries only, not their private
contents, and creates no lock or cleanup mutation.

The inventory accepts only version-1 transaction metadata and valid transaction
ID directories, with a 1024-directory inspection limit. Unknown versions,
unexpected files/names, read failures, or larger inventories produce
`transaction.staging_unavailable` and an unhealthy result rather than a partial
list that could be mistaken for complete coverage. Existing secure-tree
validation precedes inventory; this limit does not bound the entire doctor scan.

The four legacy-unbound Git/no-Git × transaction/rotation fixtures kill a real
writer before journal publication. Doctor first observes its live lock and staging; recovery refuses
that live owner. After death, recovery releases the lock without claiming a
completed journaled operation. Doctor must still report the exact staging path,
leave all evidence unchanged, and retain access to the original live values.
The rotation cases retain a generated staged private identity. A negative
control suppressing inventory once unlocked makes all four cases fail.
Unknown-domain, unexpected-file and over-limit fixtures also fail closed without
changing the store.

This inventory is a momentary observation, not an ownership or abandonment
proof. A live writer may be using the directory, a valid recovery journal may
need it, or an interrupted unpublished operation may have left it. Automatic
cleanup of older unbound stages and sibling full-restore staging remains open.

## Destructive retirement and leftover key capsules

An abandoned rotation can leave both a generated private identity and the old
private identity encrypted to its recipient under staging. Together they can
recover the old key even if a later rotation deletes the ordinary retired-key
directory. Tests now decrypt that staged capsule, compare the recovered key
exactly with the current fixture identity, and use it to decrypt an existing
entry. This is local retained recovery material, not merely an external-copy
limitation.

Fresh destructive rotation therefore refuses nonempty or uninspectable staging
before acquiring a lock and rechecks under its owned lock before reading private
keys or creating its own stage. `identity.staging_present` includes the paths
and `applied: false`, directing inspection through doctor. No inferred orphan
is deleted automatically. Continuity-preserving rotation does not make a local
key-destruction claim and keeps its existing behavior.

Destructive recovery also checks for unrelated staging, excluding only the
validated pending journal's own transaction ID. This protects recovery of older
journals as well as fresh operations. A resumed journal may already have changed
live state, so this internal refusal does not assert `applied: false`; the normal
recovery-incomplete wrapper retains the lock and applied-state warning.

Two actual killed-rotation fixtures prove the key-capsule risk and unchanged-store
refusal on Git/no-Git. Two pending destructive-recovery fixtures introduce an
unrelated private stage and verify that refusal preserves all live files,
journals, staging and lock evidence. Disabling the guard makes all four refusal
cases fail. Normal continuity/destruction and destructive killed-owner recovery
with only the owned stage continue to pass.

The inventory is deliberately conservative: any other stage requires inspection,
without guessing from filenames whether it contains a private key. Safe cleanup
of those leftovers remains required work. The shared-lock contract coordinates
Fulla/pa writers; this does not add same-Unix-user isolation or revoke external
copies, backups, filesystem snapshots, or previously exported recovery archives.

## Owned unpublished staging recovery

Current transaction and rotation writers create an empty private stage, then
persist its ID as `stage_id` in their owned `lock/info` before writing private
staging material. Binding publication/durability must succeed before proceeding.
An interruption before the binding can leave an empty unbound directory; it
cannot leave private staging material written by these paths. Cleanup only owns
a stage after its exclusive directory creation succeeds.

The lock reader rejects duplicate/invalid IDs and simultaneous staging/peer
receipt bindings. Recovery validates a pending journal against the binding;
mismatched transaction/rotation IDs or a prune conflict fail closed. A published
journal still follows its normal recovery procedure, preserving its staging.

When no journal exists, explicit dead-owner recovery removes only the bound
transaction directory and syncs its parent before releasing the lock. It never
sweeps unrelated directories. Missing bound staging is safe to retry; unexpected
non-directory evidence is refused. Ownership takeover preserves `stage_id`, so a
failed or interrupted cleanup remains bound to the replacement owner/token.
`staging_cleaned` identifies the handled stage in the recovery result. Handled
pre-journal cleanup failures likewise retain the bound lock and report
`recovery_required` rather than losing ownership by releasing it.

Eight real killed-writer fixtures cover Git/no-Git × transaction/rotation ×
current bound/reconstructed legacy unbound lock schemas. Current cases clean
staging and return a healthy unchanged live store. Legacy cases only release the
dead lock and leave staging visible. The legacy schema is reconstructed by
removing the new binding from a killed current writer's fixture lock; this is
not execution of an older binary. Non-root current cases also fail cleanup in
a separate recovery process after validation, verify the changed owner/token
retains the stage binding, then repair only fixture permissions and retry.
Dropping the binding on takeover makes all four current cases fail.

The initial permission fixture denied access before recovery validation and thus
never exercised takeover. It was corrected to inject the filesystem denial after
validation through the existing internal validation seam. There is no product
environment flag for this injection. The four disposable failed-test directories
were moved to Trash after restoring their directory permissions.

This is an additive lock-record contract, not a domain-version migration. Older
readers may ignore the field and release an unjournaled lock without cleanup;
use a supporting Fulla binary to recover bound staging before rolling back.
Previously unbound stages, foreign/remote/live owners, and sibling restore
staging do not gain deletion authority. Physical power-loss and all interrupted
cleanup boundaries still require separate acceptance.
