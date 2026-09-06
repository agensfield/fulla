# Mutation boundary inventory

This is the operation-level index for J9 of the locked specification. Rows
separate inspected implementation from native process-kill evidence. Shared
engine coverage applies to the engine's state transitions; it does not by itself
prove every caller's preparation, selected input, confirmation or returned result.
No SIGKILL case is physical power-loss or filesystem durability proof.

| Workflow | Publication mechanism | Native interruption evidence / remaining work |
| --- | --- | --- |
| Fresh init | Bound private sibling stage, complete validation/synchronization, target publication, lock release | `init_crash_test.go` and [initialization recovery](init-recovery.md): Git/no-Git before keys, after keys, staged and published; cleanup-denial retry. Pre-binding creation and low-level atomic-write windows need separate accounting. |
| pa adoption | Bound metadata stage and metadata publication under shared lock | `adoption_crash_test.go`, `adoption_retry_test.go`: bound-empty, staged, published and cleanup-denial recovery. Legacy unbound stages remain inspection-only. |
| Add/edit/remove/move | `transaction.go`: staging, journal, per-entry publication, Git commit, snapshots/receipt, cleanup | `transaction_crash_test.go` exercises a two-entry move-shaped engine mutation; `snapshot_crash_test.go` covers snapshot namespace variants. Exact public-command preparation boundaries are not all separate killed-process cases. |
| Basic CRUD with future transaction domain | Same journal engine routed through fixed `basic-v1` namespace | `basic_protocol_crash_test.go`: Git/no-Git staged, journaled, first entry published, committed, receipted and cleanup-denial retry. This is a fixed codec, not a domain migration. |
| Logical import | Complete bundle verification and selection, then `mutate` | Shared engine evidence above. Caller-specific interrupted preparation and partial-result evidence must be indexed separately. |
| History restore | Historical selection/decryption/confirmation, then `mutate` | Shared engine evidence; successful human/agent and retired-key recovery fixtures exist. Caller-specific killed preparation is not established by those journeys. |
| Snapshot restore | Complete selected snapshot verification/confirmation, then `mutate` | Shared engine evidence; normal/basic snapshot and composite archive recovery journeys exist. Caller-specific killed preparation remains separate. |
| Peer enroll | Atomic no-replace peer record, shared lock release | `peer_crash_test.go`: killed after publication, explicit dead-owner recovery preserves new pin. Atomic temporary-file preparation is not covered by this hook. |
| Peer rotate/remove | Prepared receipt, lock receipt binding, pin replacement/removal, applied receipt, lock release | Native Git/no-Git cases at bound prepared, published and receipted boundaries now cover both commands; see below. Receipt creation before binding and low-level atomic temporary-file windows remain separate. |
| Sync dry-run/activation mark | Atomic peer record replacement under shared lock | `peer_crash_test.go`: killed after publication for both marks. Pre-publication preparation remains separate. |
| Sync entry transfer | Authenticated remote session and scoped import, then per-store journal engine | [Cross-host retry](crosshost-acceptance.md) covers lost imported/exported replies and rerun convergence on Git/no-Git stores. Transport evidence does not cover every entry-publication boundary independently. |
| Identity rotation | Bound transaction, verified new ciphertext/private material, journal, per-path publication, commit, private-copy cleanup | `rotation_crash_test.go`, `rotation_publication_test.go`, [owned publication](rotation-publication.md). Pre-binding and legacy unbound private-copy accounting remains separate. |
| Snapshot prune | Independent prune journal, recursive snapshot deletion, receipt, cleanup | `prune_test.go`: prepared, removed, receipted kills; normal/basic namespaces and reconstructed partial recursive unlink. The reconstructed unlink is not an observed instruction-level kill. |
| Logical/full archive export | External atomic artifact publication followed by in-store receipt | `PublishArtifact`, `ExportLogical`, `ExportFull` inspected. Post-rename synchronization errors now retain applied-state evidence (below). Bound export-v1 receipt/staging recovery now covers encoding, binding, staging, rename, publication and receipt boundaries (below). Generic receipt-write windows and physical durability remain separate. Stdout logical export has a different, non-atomic stream boundary. |
| Full archive restore | Independently authenticated complete archive in bound sibling stage, target publication | [Archive recovery](archive-recovery.md): 44 handled/SIGKILL cases across Git modes and absent/empty targets, plus replacement-token cleanup retry. Legacy unbound stages remain separate. |
| Permission repair | Validated per-path mode changes, receipt, shared lock | `permissions_test.go` includes killed-owner recovery. Per-chmod and receipt boundaries need enumeration; a partially repaired tree is an explicitly recoverable intermediate state. |
| Doctor recovery | Validated dead-owner takeover, operation-specific reconciliation, lock release | Binding/retry tests cover transactions, init/adopt/restore, peers, prune and rotation. Recovery is itself a mutation and requires its own refusal/retry evidence. |
| Shared lock release | Atomic detach retaining ownership, bounded cleanup | [Lock release recovery](lock-release-recovery.md): six Git/no-Git killed boundaries, cleanup denial and preservation of newer writers. |
| Expert Git | Explicit trusted system Git subprocess under shared lock | Git owns its repository transaction semantics; Fulla's native passthrough/lock tests do not establish Fulla journal recovery for arbitrary Git commands. Preserve the expert-command trust boundary. |
| Clipboard copy/expiry | External backend write and digest-matched delayed clearing | Covered by dedicated clipboard acceptance, outside store journaling. Backend read/clear is not an atomic compare-and-clear operation. |

## Peer receipt boundary acceptance

`TestPeerPublicationKilledOwner` now runs 20 Git/no-Git native cases. Rotation
and removal each run three boundaries: after the prepared receipt is bound but
before the pin changes, after the pin changes, and after the applied receipt is
written. Enrollment, legacy unbound rotation, dry-run and activation retain their
existing post-publication cases. The legacy case deliberately removes the binding
and is labeled reconstructed legacy state.

Each child announces its boundary and is killed by the parent. Recovery refuses
a live owner and a wrong token, then accepts the dead owner's token. Before pin
publication it preserves the original peer and finalizes the receipt as `aborted`;
after publication it preserves the replacement/removal and finalizes or retains
`applied`. Identity bytes, entry plaintext and ciphertext remain unchanged. Legacy
unbound recovery does not guess which historical receipt it should finalize.

The internal hook now names `prepared`, `published` and `receipted`; public
methods always pass nil. Existing handled-error and lock-release fixtures retain
injection specifically at `published`. No runtime fault-injection switch or
persisted format change is introduced.

## Export publication versus directory synchronization

`PublishArtifact` previously used the error-only `securefs.PublishNew` wrapper,
which discarded whether rename had already published the artifact. A subsequent
directory-sync failure therefore became ordinary `export.publish_failed`, even
though the complete output file existed. Logical and full export both use this
helper and propagate its error.

The helper now uses `PublishNewPublished`: a post-publication failure returns
status 3 with applied=true and durability_confirmed=false, and tells the caller to
inspect the artifact before retrying. Pre-publication failures retain status 1;
underlying filesystem diagnostics are not exposed. No success receipt is invented
and an existing output remains protected against replacement.

`TestExportPublicationPreservesAppliedState` covers both outcomes on Git/no-Git
stores, actual exact-byte private-file publication for the applied case, refused
retry overwrite, diagnostic redaction and unchanged store paths/modes/digests.
Its internal publisher fixture models a post-publication failure; the securefs
`TestNewPublicationReportsDirectorySyncFailure` separately injects the error at
the actual post-rename synchronization callback. These are handled-error tests,
not physical fsync failure or killed-export acceptance. Disabling the applied
branch is the negative control and must fail the applied cases.

## Initial native killed-export boundaries (50d8407)

`TestKilledExportPreservesArtifactAndStore` runs twelve native subprocess cases:
Git/no-Git stores, logical/full exports, and completed encoding, artifact publication
or receipt publication. Private hooks are internal only; public methods pass nil.
Encoding is complete in memory at its hook, before external staging begins.

The parent refuses live-owner recovery, kills the exporter, refuses a wrong token,
then explicitly releases the dead owner's lock. At the encoding boundary there is
no external artifact. At either later boundary the artifact remains complete and
private; logical verification/import or full restore recovers binary and empty
entries. Full restore checks the entire archived tree against required 0600/0700
modes, while the source tree retains its original bytes and modes. Retry against
the existing artifact refuses without changing it. A receipted export adds exactly
one matching applied receipt; earlier cases add none. No hidden temporary file is
present at these three high-level boundaries.

Recovery reports lock release, not reconstructed export completion: there is no
bound export journal from which to recover a missing receipt after publication.
This test explicitly preserves that limitation rather than inventing an applied
receipt from the external output. Staging-write/rename/sync instruction windows,
native stdout interruption and durable export-to-receipt reconciliation still
require separate work. Handled stdout write outcomes are covered below. A successful killed-process fixture is not power-loss proof.

## Stdout write outcome accounting

Logical stdout export previously ignored the writer's byte count and returned raw
writer errors. A short write with nil error could therefore create an applied
success receipt for a truncated bundle. Export now requires the complete encoded
length with no error before proceeding to receipt publication. A zero-byte failure
returns typed export.write_failed (status 1); a failure after accepted bytes returns
status 3 with applied=true. Both include bytes_written and output_complete, without
formatting the underlying writer error. Even a full-length write accompanied by
an error remains a failure and creates no success receipt. No automatic write
retry can duplicate part of the binary stream.

Twelve Git/no-Git store cases cover success, zero/partial writes with errors,
zero/partial short writes without errors, and full-length writes with errors.
They check receipt absence/presence, typed byte accounting, redaction, unchanged
source state and whether the captured encrypted stream verifies. Two public CLI
dispatch cases prove status 3, stderr-only diagnostics and no receipt after a short
write. Previous transfer.go source fails all ten store error/short-write cases.
These controlled writer fixtures do not claim operating-system SIGPIPE handling
or atomic stream delivery.

## Bound file-export recovery

The earlier killed-export checkpoint established artifact survival but lacked
receipt reconstruction. File exports now prepare a versioned receipt containing
the canonical output path, ciphertext digest/size, operation metadata and a fresh
ID. The shared owner record binds that ID as export_receipt with the explicit
stage_protocol=export-v1 discriminator before any external stage is created.
The external stage is the private `.fulla-export-ID` file beside the output.
Only encrypted bytes are staged, including for independently protected full
archives. Receipt preparation happens after encoding, so a full archive does not
include its own pending export receipt.

Ordinary publication writes and synchronizes staging, publishes without replacing
an output, synchronizes the parent and finalizes the receipt as applied. A bound
failure retains ownership and reports recovery_required; successful completion
uses the existing atomic lock-release path. Logical stdout keeps its explicit
stream semantics and has no external-file recovery journal.

Recovery validates the receipt version/binding, exclusive operation, canonical
outside-store path, private object types, ciphertext size/digest and existing
receipt phase before taking ownership, then repeats validation under the guard.
It refuses changed outputs and unsafe stage links. Missing output means cleanup
of the exact bound stage and an aborted receipt; matching output means applied
receipt reconciliation. It synchronizes cleanup/publication evidence before
releasing ownership. Replacement owner tokens preserve the export binding on
recovery failure; retries must use the newly inspected token.

File-export recovery requires the understood transaction domain. CLI preflight
checks that eligibility before recovery input, and exports repeat it under the
shared lock before decryption. Generic artifact publication remains independent:
doctor reports must work without readable store metadata. The initial integration
incorrectly put the domain requirement in that generic path; the full CLI suite
caught the regression, and export-specific preflight fixed it.

Current native acceptance names encoded, bound, staged, renamed-before-parent-sync,
published and receipted boundaries on both export types and Git modes. Additional
fixtures reject changed outputs, staging symlinks/hardlinks, future versions,
inside-store targets, wrong receipt IDs and competing bindings without changing
store ownership. A truncated stage reconstructs an interrupted write. Real
cleanup denial after validation proves replacement-token retention and retry.
The actual previous 772cfab binary refuses the unknown export-v1 discriminator,
preserves store/stage state and permits subsequent current-reader recovery.

This adds recovery records without changing pa-v1 or manifest domain counters.
It is not a released domain migration. Older development binaries predating the
staging-protocol guard are not safe recovery tools; finish pending exports with a
supporting binary before rollback. Unbound prepared receipts are inert historical
evidence, never discovered as authority by scanning. Interruptions inside the
receipt's own generic atomic write and physical power loss remain separate from
the named process-kill boundaries. No atomic stdout delivery is claimed.
