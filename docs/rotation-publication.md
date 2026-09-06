# Rotation publication stays in owned staging

Rotation now prepares each publication copy inside its bound transaction directory:
`.fulla/transactions/ID/publish-PATH_HASH`. The hash is SHA-256 of the validated
relative destination name. The original `after/` file remains intact so recovery
can validate the whole journal after any subset of files has been published.

Before publication the copy is written privately and synced. If retry finds it
already present, securefs validates the file and its bytes must match the journal's
expected after digest. A mismatch refuses without discarding the journal or copy.
Publication atomically renames this copy into the live destination. New targets
use no-replacement rename; existing journal-authorized targets use replacement.
Both the source and destination parent directories are then synced. Recovery also
repeats those syncs when the destination already contains the journal's expected
bytes, covering an earlier rename whose synchronization did not complete.

The no-replacement primitive accepts direct child names relative to two open
parent roots. It uses the existing Darwin RENAME_EXCL / Linux RENAME_NOREPLACE
syscalls with separate source/destination directory descriptors. It never passes
a multi-component user path to those syscalls. The original same-parent helper
now delegates to it. Tests prove successful cross-parent publication and refusal
of occupied targets, traversal and dot/parent names with source/target preserved.

The initial integration test caught an attempted use of the old same-parent
helper with a multi-component source, reporting "publication requires direct
child names". The cross-parent primitive resolves that actual constraint while
preserving no-replacement behavior; no error was suppressed.

## Interruption and compatibility

The existing rotation journal and stage binding own the publication copy before
it contains data. A test hook after file synchronization and before rename allows
a real subprocess to be killed at the private-identity and sealed-retired-key
boundaries. Git/no-Git and continuity/destruction cases verify:

- Only the bound transaction directory appears in staging inspection, with no
  new unbound atomic key sibling in the root or retired directory.
- The publication copy is private and equals its intact `after/` source.
- Live-owner and wrong-token recovery refuse; SIGKILL recovery with the inspected
  token completes and releases the lock.
- Exact live bytes survive, Git historical access follows the chosen retirement
  policy, the receipt records the correct destruction result, and staging clears.

A separate corrupted-copy fixture tests refusal without changing evidence and
successful retry after restoring only the deliberate fixture corruption. This is
in addition to the existing journaled/published/committed/cleaned crash matrix.
SIGKILL is not storage power loss or proof of physical secure erasure.

No domain or journal version changes. Old prepared journals with no publication
copies remain readable; the current writer creates a copy as needed. Older
readers still have their original `after/` files, but may again use unbound atomic
siblings, so pending recovery should use a supporting current binary. This is
not a claim of released-binary migration acceptance.

The previous atomic-key staging guard remains necessary for older interrupted
writers and reserved-name files of unknown provenance. It reports/refuses them
without granting deletion authority. Initialization/restore siblings, legacy
unbound files, external archives and physical durability remain separate work.
