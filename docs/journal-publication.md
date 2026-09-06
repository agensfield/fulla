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
exclusive creation succeeded, synchronize its parent after removal, and attempt
to release the owned lock. Both transaction and rotation use the same cleanup
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
store files, no changes outside owned staging, released lock and readable
unchanged live value. Rotation fixtures retain the generated staged identity to
make the private-copy risk concrete. A changed-lock-owner test verifies that its
files remain untouched and that its release failure is reported. Reinstating
ignored cleanup errors through a source overlay makes all 13 cases fail.

These are handled-failure tests on disposable non-root fixtures, not SIGKILL
cleanup or an automatic orphan-removal mechanism. Unpublished leftover staging
has no committed recovery journal. Do not infer that ordinary recovery will
finish or discard it; automatic identification and safe cleanup remain open.

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

Four Git/no-Git × transaction/rotation tests kill a real writer before journal
publication. Doctor first observes its live lock and staging; recovery refuses
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
orphan classification/removal and sibling full-restore staging remain open.
