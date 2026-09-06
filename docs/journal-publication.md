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
that a failed fsync reached persistent media. Pre-journal cleanup reporting,
pre-journal killed-writer orphan handling, and other publication surfaces remain
separate acceptance work.
