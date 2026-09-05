# Interrupted full-state restoration

Full-state restore builds a private sibling `.fulla-restore-<random-id>` directory,
authenticates the complete archive, verifies the store and identities, syncs every
staged directory bottom-up, and publishes with atomic no-replace rename. It then
syncs the parent directory. Each file is synced when written. Empty directories
and newly created ancestors participate in the final directory sync.

## Tested process-crash outcomes

| Interruption | Target | Staging | Retry |
| --- | --- | --- | --- |
| During file extraction | Absent or still empty | One private orphan after SIGKILL | Fresh restore succeeds; orphan remains unchanged |
| After validation/directory sync | Absent or still empty | One private orphan after SIGKILL | Fresh restore succeeds; orphan remains unchanged |
| After removing a pre-existing empty target | Absent | One private orphan after SIGKILL | Fresh restore succeeds |
| After no-replace publication | Complete restored store | No remaining stage at its old name | Refuses to replace the populated target |
| After parent sync | Complete restored store | No remaining stage at its old name | Refuses to replace the populated target |

The corresponding handled-error cases attempt to remove their stage and sync
the parent to confirm deletion. Errors injected
after publication return applied-state status 3; errors before publication retain
their pre-publication error. Removal of an existing *empty* target creates a brief
absence before publication, but does not remove any existing credential contents.

These outcomes are exercised on Git and no-Git fixtures with absent and empty
targets: 18 SIGKILL cases and 18 handled-error cases. Tests compare all published
paths, file digests and modes, deep-verify the result, check retry behavior, and
assert the source is unchanged. A separate test verifies that every directory,
including empty nested directories, is synced before its parent and that sync
errors propagate. SIGKILL does not simulate storage power loss or prove hardware
flush behavior.

## Orphan handling remains explicit

A process killed before publication cannot execute deferred cleanup. Its private
stage can contain the restored active identity and other recovery material, not
just ciphertext. Tested stages retain 0700 directories and 0600 files. Retry does
not reuse or delete these directories: a filename prefix alone is insufficient
authority to destroy recovery evidence.

Retry from the original encrypted archive and independent recovery material into
an empty target. Do not treat an interrupted staging directory as a verified live
store or copy its partial contents over a target. A published populated target
must be inspected as restored state; retry is deliberately non-overwriting.

If stage removal or confirmation of that removal fails, Fulla returns
`recovery.cleanup_failed` with `cleanup_required`, the exact `staging_path`,
`target`, and `applied: false`. When the original error is typed, `operation_code`
retains its code without copying its potentially sensitive message. Signal exit
statuses 129, 130, 131, and 143 remain signal statuses. The retained stage may
contain identity material; a failed directory sync can also leave deletion's
durability uncertain even if the path is currently absent.

Actual permission-denied cleanup is tested for ordinary failure, interaction
refusal, and cancellation. Once publication succeeds, cleanup stops referring to
the old stage name: a new occupant at that name must be preserved. A regression
test exercises this replacement explicitly.

Automatic orphan identification/cleanup remains implementation work. No automatic
scanner or cleanup command is claimed here. Disk-error and power-loss behavior
beyond the stated sync/error boundaries remains separate acceptance work.
