# Implementation and acceptance plan

The canonical vault contract was approved on 2026-08-12, extended with scoped
recovery on 2026-08-30, and authorized for implementation/publication on
2026-09-05. Fulla's name, binary, and Agensfield repository remain locked.

## Milestones

- [x] Read canonical contract, original interview, and predecessor audit.
- [ ] Define persisted state, recovery protocol, and concrete machine results.
- [ ] Strict configuration, rooted filesystem validation, embedded age,
      exact-byte protocol fixtures, and structural/deep diagnostics.
- [ ] Transactional initialization/adoption, daily CRUD, Git history, backups,
      and interrupted-operation recovery.
- [ ] Human TTY/editor/generation, clipboard lifecycle, direct process injection.
- [ ] Logical transfers and separately protected scoped/full recovery.
- [ ] Peer enrollment/rotation, mutual session challenges, SSH synchronization,
      mandatory first dry-run, strict skips, and partial convergence.
- [ ] Identity rotation, sealed history continuity, destructive retirement.
- [ ] All nine acceptance journeys, twelve fixture families, sixteen invariants.
- [ ] Four-platform CI, preview release artifacts/checksums, installation proof.
- [ ] Scoped vault closure and accurate remaining v1 acceptance conditions.

No placeholder command counts as implementation. Passing build gates alone does
not satisfy a workflow. Preview publication requires the preview acceptance
contract. A stable release additionally requires the real-store and real-host
operational proofs in the product contract.

## Scope of this execution

The public repository and Fulla-only release work are authorized. Fixtures must
use generated, non-secret material. Existing credential stores, infrastructure
credentials, and unrelated projects are not silently migrated or mutated.
Cross-host tests use explicitly established targets and disposable Fulla stores.
The Agensfield tap change is a Fulla distribution change only; existing formulae
must remain untouched.

## Source and dependency decisions

Reuse the accepted predecessor's PAXFER1 encoder/decoder and malformed-input
tests, retaining attribution. Reimplement the CLI and identity operations with
embedded official age rather than preserving the predecessor's external age
subprocess boundary or SSH-only authorization.

Initial direct dependencies: official `filippo.io/age` v1.3.2 (current hardening
release), `github.com/pelletier/go-toml/v2` v2.4.3 (strict TOML parsing, maintained,
no runtime dependencies of its own), and Go's maintained Unix/terminal packages
where portable syscalls or hidden terminal input require them. Standard Go
testing and the system Git/OpenSSH integrations keep the remaining surface small.

Sources checked 2026-09-05:
- https://github.com/FiloSottile/age/releases/tag/v1.3.2
- https://github.com/pelletier/go-toml/releases/tag/v2.4.3

## Recovery mechanics to implement

The interoperable root retains `identities`, `recipients`, `passwords/`, and
the shared mkdir-based `lock/`. Fulla state lives in `.fulla/`, outside the
password Git repository. Metadata domains are versioned separately.

Mutations validate and lock, stage encrypted replacement material, verify it,
durably write a journal, publish each path, finalize Git and receipts, and then
release the lock. A journal records expected old/new ciphertext digests and
progress. Interrupted publication remains locked and unavailable to ordinary
Fulla operations until explicit recovery completes. Recovery is idempotent and
never guesses past a digest mismatch. This is crash-recoverable publication,
not a claim that multiple POSIX file renames are one atomic operation.

Stale-lock repair requires an explicitly selected owner token, a verified dead
local PID, and recovery of any pending journal. Never steal a live, foreign,
unknown, or changed lock. Shell pa interoperability requires the common lock;
operators must not manually remove a lock around interrupted Fulla work.

Support local POSIX filesystems with reliable exclusive creation, rename, hard
links, and directory fsync. Fail closed for known network filesystems. Private
symlinks, external hard links, permissive modes, and foreign ownership are
rejected. Rooted descriptor operations prevent path replacement escaping a
validated store. Internal publication links are temporary and accounted for.
