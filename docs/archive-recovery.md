# Interrupted full-state restoration

Full-state restore builds a private sibling `.fulla-restore-<random-id>` directory,
authenticates the complete archive, verifies the store and identities, syncs every
staged directory bottom-up, and publishes with atomic no-replace rename. It then
syncs the parent directory. Each file is synced when written. Empty directories
and newly created ancestors participate in the final directory sync.

## Tested process-crash outcomes

| Interruption | Target | Staging | Retry |
| --- | --- | --- | --- |
| Before extraction, after binding | Absent or still empty | Owned private stage after SIGKILL | Fresh restore succeeds; explicit recovery removes the owned stage |
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
targets: 22 SIGKILL cases and 22 handled-error cases. Tests compare all published
paths, file digests and modes, deep-verify the result, check retry behavior, and
assert the source is unchanged. A separate test verifies that every directory,
including empty nested directories, is synced before its parent and that sync
errors propagate. SIGKILL does not simulate storage power loss or prove hardware
flush behavior.

## Owned recovery remains explicit

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

Current writers acquire the shared lock and bind `restore_id` plus the encoded
absolute `restore_target` before extracting files. After archive EOF
authentication, deep verification, clean Git and supported-domain checks, they
also bind `restore_store_id`. The lock follows publication and releases only
after the parent sync. Handled post-publication failure retains this owned lock.

Inspect the exact stage or published target with `fulla --store PATH --json doctor`,
then run `fulla --store PATH --json doctor --recover-lock TOKEN` with its
inspected dead local owner token.
Unpublished recovery removes only that owned private stage, without publishing
its contents or changing an independently restored target. Extracted pending
journals are opaque cleanup material, never live recovery instructions. Published
recovery requires the bound store identity, understood domains and valid layout;
it refuses conflicting live journals, syncs the parent and releases the lock.
It never removes a new occupant of the old staging name.

Private contents are removed before ownership files. A real permission-denied
recovery subprocess proves replacement-token and restore-binding retention;
after restoring only the fixture mode, the old token is refused and the new
owner can retry. CLI fixtures cover partial-stage inspection and unchanged
refusal of absent, conflicting and mismatched bindings. Reconstructed fixtures
also cover opaque extracted journals and missing/mismatched published identities.

Legacy unbound siblings still have no deletion authority. Older development
binaries ignore these optional binding fields; use the current binary for an
interrupted restore. Successful pa-v1 stores and all domain versions remain
unchanged. General interruption during final lock-file removal and disk-error or
power-loss behavior beyond these sync/error boundaries remain separate work.
