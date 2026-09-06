# Scoped infrastructure recovery acceptance

The canonical August 30 Agensfield decision rejects exporting the entire live
password store as a substitute for an infrastructure recovery kit. It requires
an explicit inventory of needed entries, separate recovery protection,
nonmutating verification and an isolated recovery read. It does not prescribe
fixed credential names or authorize credential revocation. Physical duplication
and separate human custody are deployment responsibilities.

`TestInfrastructureManifestRecoveryJourney` invokes the public CLI in Git and
no-Git modes with generated identities and synthetic values. Its manifest selects
`agensfield/control-plane` and `agensfield/host-recovery` while excluding
`personal/excluded`. These are fixture names, not the live Agensfield inventory.

The combined journey verifies:

- Missing, duplicate, empty and wildcard-like selectors fail before artifact
  publication and leave all source paths, modes and file hashes unchanged.
- The capsule and export receipt contain exactly the selected names/count.
  The artifact is mode 0600; the source changes only by its export receipt.
- A separate generated recovery identity verifies the capsule with a nonexistent
  store path. Verification neither creates that store nor changes the source.
- A full disaster archive protected only by its contained active identity is
  refused without artifact publication or source changes.
- A correctly protected whole-store disaster archive is rejected by logical
  verify/import, preserving the logical recovery target.
- Import into an independently initialized store yields only the selected names;
  raw reads preserve exact binary and text bytes, including trailing newlines.

An uncommitted Go source overlay that ignored the explicit selector and exported
everything made both variants fail. This confirms the journey detects a scope
bypass. Existing controlling-terminal acceptance separately exercises confirmed
passphrase export and isolated passphrase recovery; this combined journey chooses
the independent-recipient branch allowed by the contract.

Explicit all-entry logical export and full disaster backup remain supported
products. They are not automatically authorized by a scoped recovery request.
The chosen manifest must be reviewed by its caller; Fulla cannot infer which
credentials a particular infrastructure deployment needs. No live credential,
offline-custody or revocation acceptance is claimed by this fixture.
