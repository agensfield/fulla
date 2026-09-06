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
| Logical/full archive export | External atomic artifact publication followed by in-store receipt | `PublishArtifact`, `ExportLogical`, `ExportFull` inspected. Artifact-versus-receipt applied-state and killed temporary-file/publication boundaries still need explicit acceptance. Stdout logical export has a different, non-atomic stream boundary. |
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
