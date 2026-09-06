# Interrupted shared-lock release

Release used to unlink `lock/info`, then `lock/owner`, then the lock directory.
A process killed between those removals could leave an incomplete shared lock
without the evidence needed for explicit recovery. Normal successful tests did
not exercise that window.

Current release first atomically renames the intact lock, without replacement,
to a private store-root directory:

```
.fulla-lock-release-v1-HOST_SHA256-PID-TOKEN
```

The name retains the cleanup version, local host fingerprint, original process
ID and random owner token even after the files inside are removed. Release syncs
the store root, removes only the original `info`, `owner` and optional empty
`recovery` guard, syncs the detached directory, removes it and syncs the root.
The shared lock becomes available at the rename. A later writer's `lock/` is
independent of the detached cleanup and must never be removed by it.

## Inspection and explicit recovery

`fulla doctor --json` reports pending cleanup in `lock_cleanup`, including its
relative path, token, PID and host fingerprint, and returns unhealthy status.
Use that token with `fulla doctor --recover-lock TOKEN`. Recovery requires a
supported binding, private regular metadata files, matching remaining owner
token, the local host and an owner PID proven dead. Live, remote, unverifiable,
ambiguous and malformed ownership refuses. The empty-directory phase retains
these checks. Cleanup takes a nonblocking recovery guard and revalidates the
directory identity. A same-process release retry may finish its own detached
cleanup after a handled filesystem error.

Detached directories contain lock metadata only, never entries, keys or staged
operations. Unknown files, nonempty guards, unsafe permissions, links and future
bindings are not deletion authority. Directory enumeration is bounded and fails
explicitly when its limit is exceeded. Shareable doctor reports exclude the
tokens and paths through their existing allowlist.

A handled failure after detachment reports `store.lock_cleanup_failed` with
`lock_released`, `cleanup_required`, `cleanup_path` and `token`; operation-level
callers can wrap this in their existing applied-state error. Inspect doctor to
locate remaining cleanup. Full export refuses residue before selected recovery
input, rechecks under its lock and refuses reserved names during traversal.
Restore rejects these paths rather than cloning transient ownership.

## Evidence and limits

Git/no-Git subprocess tests kill actual owners at six boundaries: before detach,
after detach, after root sync, after removing info, after removing owner and
after empty-directory sync. They prove live-owner refusal, dead-owner recovery,
unchanged completed store bytes/modes, wrong-token refusal and preservation of
a newer active lock. Additional fixtures exercise actual unlink permission
denial followed by retry, malformed ownership and the public doctor/export
input boundary. An encrypted archive containing reserved cleanup ownership is
rejected without a target or staged residue. Bypassing cleanup-file validation
in a source overlay makes the unknown-file regression fail because refusal has
already destroyed ownership evidence. Malformed states are reconstructed
fixtures, not SIGKILL cases.

The shared validation integration also exposed a pathname bug after init
publication: `DirEntry.Info` retained the old staging path even though the open
root followed the renamed directory. Validation now uses `Root.Lstat` through
that descriptor, preserving permission, inode and ACL checks. A regression first
failed with `lstat .../stage/./entry: no such file or directory`, then passed with
the fix and still refused unsafe file permissions after publication.

Successful stores keep their existing pa-v1 layout and domain versions. Older
binaries do not understand this cleanup namespace; finish pending cleanup with
a supporting binary before rollback or full export. This change cannot recover
older incomplete locks that already lost their owner evidence. SIGKILL proves
process-crash behavior, not storage power-loss durability or malicious access by
another process sharing the owning Unix account.
