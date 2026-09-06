# Private disk copies and retirement boundaries

This inventory follows the owned disk-writing paths in initialization, rotation,
transactions, full restore, securefs publication and the editor. It distinguishes
an encrypted old key from a plaintext new key; either can preserve access to
historical ciphertext. It does not claim erasure of external backups, filesystem
snapshots, swap, editor-managed files or another process sharing the Unix account.

| Location / writer | Material | Cleanup and authority |
| --- | --- | --- |
| Store `identities` | Active private identity, potentially plugin references or protected identity data | Rotation publishes a replacement under the shared lock and journal. |
| `.fulla/transactions/ID/after/identities` during rotation | New private identity | Bound-stage and journal recovery retain ownership evidence until staged removal and parent sync complete. Other transaction stages block destructive retirement. |
| `.fulla/transactions/ID/after/.fulla/retired/ID.age` | Old identity sealed to the new identity | Destructive rotation removes its staged sealed copy as well as the published retired artifact; transaction staging is subsequently removed durably. |
| `.fulla/retired/ID.age` | Sealed historical identity chain | Historical use is explicit. The selected destructive rotation removes its retired artifact; external copies cannot be revoked. |
| Store-root `.fulla-stage-*` | Atomic replacement can hold a plaintext private identity before rename | No durable binding maps this file to its intended destination. Doctor reports the name; destructive rotation and recovery refuse while it remains. No automatic deletion. |
| `.fulla/retired/.fulla-stage-*` | Atomic publication can hold a sealed retired identity before rename | Same diagnostic/refusal policy. A filename does not establish authority to erase the file. |
| Parent `.fulla-init-ID` | Newly generated private identity and staged initial store | Handled cleanup is owned and reported. SIGKILL can leave an unbound sibling; there is no automatic recovery/delete authority. |
| Parent `.fulla-restore-ID` | Restored private identities, retired artifacts and complete or partial archived store | Verified before publication; handled cleanup is owned. SIGKILL can leave an unbound sibling. Retry uses the original archive and does not delete an older sibling. |
| Store `.fulla-adopt-ID` | Fulla metadata; adoption preserves existing live pa identity | Current writers bind the intended store ID into the shared lock. Recovery distinguishes unpublished stage cleanup from published metadata. Legacy unbound staging remains separate. |
| Transaction backups / Git history | Encrypted entry ciphertext, not a plaintext entry cache | Retention/prune and historical-key recovery are separate policies. Deleting a live entry does not itself erase every historical ciphertext copy. |
| Selected full-state archive | Whole store, including private identity, protected by independent recovery material | User-selected external artifact. Local identity rotation cannot revoke copies or their independent recovery material. |
| Private `fulla-edit-*` temporary directory | Plaintext edited entry and any editor files contained there | Handled/cancellable editor cleanup removes the owned directory. SIGKILL and editor-created external backups remain explicit limits. |

## Atomic key-file gap found during this audit

`securefs.ReplacePublished` and `PublishNewPublished` first write and sync a
private `.fulla-stage-ID` sibling, then rename it. A crash before rename can leave
that file. The prior staging inventory examined only `.fulla/transactions`, so
root identity copies and retired-directory atomic files were invisible to the
retirement guard. Repeated rotations could otherwise leave a recoverable key
copy after a later operation claimed local retired-key destruction.

Staging inspection now also lists reserved atomic-stage names in the store root
and retired directory, without reading their contents. Each directory is read in
256-entry batches with a 65,536-entry inspection limit; exceeding it refuses
inspection explicitly. Suspicious names are reported even if their suffix or
file type does not match a current writer. Existing filesystem safety validation
continues to apply. This is conservative identification, not proof of provenance.

Destructive rotation checks the inventory before acquiring the lock and again under its
shared lock. Destructive journal recovery also checks it, excluding only its
own bound transaction directory. It does not exclude an unrelated atomic sibling.
Non-destructive continuity rotation does not acquire authority to delete these
files. Doctor uses the existing staging field/issue family to expose them.

Git/no-Git fixtures reconstruct the write-before-rename file state and require
read-only diagnosis plus unchanged-state destructive refusal. A journaled
recovery fixture requires the same refusal while preserving its own stage and
journal. An old-inspection source overlay fails both diagnostics and recovery
guard assertions. These are state fixtures, not a claim to reproduce an actual
SIGKILL inside the securefs syscall window or storage power loss.

## Remaining work

The new guard prevents false local destruction claims in these two additional
key-bearing locations. It does not bind or automatically remove atomic files,
initialization siblings or restore siblings. A future cleanup path must establish
operation/target ownership, dead-writer evidence, replacement-token handling and
publication state before acquiring deletion authority. Prefix matching alone
must never become that cleanup authority. Physical durability, external copies
and the canonical no-Git deletion/retention wording remain separate questions.
