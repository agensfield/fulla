# Recovery of interrupted initialization

Current initialization holds a shared lock inside its exclusively created
`.fulla-init-ID` stage before generating private keys. `lock/info` binds `init_id`
to the intended store ID and `init_target` to the absolute destination (encoded
as URL-safe base64 so spaces cannot split the record). The binding and parent
entry are synchronized before key creation. Initialization retains the lock
through validation, staging synchronization and atomic no-replacement rename.
Normal completion synchronizes the destination parent before releasing the lock.
No sidecar directory or new CLI command is introduced.

## Inspection and explicit recovery

Use the stage path reported by cleanup evidence, or the surviving staging
location found during inspection. A matching basename alone never authorizes
cleanup. `fulla --store STAGE doctor` reports the lock even if pa-v1 initialization
is incomplete. Use the inspected owner token with:

```
fulla --store STAGE doctor --recover-lock TOKEN
```

The existing doctor recovery route recognizes a bound initialization and opens
its private partial tree without requiring complete pa-v1 files. It still checks
safe ownership, modes, links and tree structure, the exact bound location and
explicit token, and a provably dead local owner. It refuses live/remote/unknown
owners, unbound legacy staging and mismatched/conflicting bindings. Read-only
validation precedes recovery-guard creation and repeats under the recovery lock.

For an unpublished stage, recovery deletes only that bound stage and reports
`init_applied: false`. It never publishes a partially initialized store or touches
an independent occupant of the intended destination. Retry `init` explicitly.

For a published destination, recovery verifies the bound store ID, supported
metadata and valid live layout, synchronizes the parent and releases the lock.
It reports `init_applied: true`. It does not touch an occupant that reuses the
former staging pathname. Recovery validates structure without decrypting values.

## Cleanup failure and retry

Recovery takeover preserves both initialization fields with the replacement
owner token. Cleanup removes non-lock contents first, then the lock and stage.
A removal failure while private contents remain therefore retains ownership for
inspection and retry. The unchanged initial owner also checks its token before
handled cleanup. Once only the lock remains, a later failure may leave incomplete
lock cleanup; this is not claimed as a fully solved general lock-release problem.

Unsafe modes still block ordinary recovery. The combined permission-repair path
is for interrupted permission repair, not initialization. The denial fixture
restores only its deliberately altered mode before retrying with the newly
inspected token; it does not establish a general automatic permission bypass.
Parent synchronization failure after successful removal is reported as incomplete
cleanup even if the path is already absent. No evidence is recreated implicitly.

## Evidence and compatibility

Git/no-Git SIGKILL fixtures cover bound-empty, keys-before-metadata, fully staged
and published states, using a destination containing spaces. They check live/wrong
owner refusal, recovery's applied state, explicit unpublished retry, preserved
published paths/modes/bytes and reused-stage occupants, deep health and missing-
lock refusal. Staged cases also run a real recovery subprocess that encounters
permission denial after validation; replacement ownership survives its exit and
ordinary retry succeeds after the fixture mode is restored.

The public CLI fixture accepts bound partial staging and rejects absent,
mismatched-target and conflicting bindings without changing files. Handled
cleanup denial must retain the binding; an overlay that skips contents-first
cleanup loses lock/info and fails that regression.

Successful stores retain the same pa-v1 and domain-version-1 layout. The optional
lock fields are new development recovery metadata. Use a supporting binary for
pending initialization: older development readers may ignore them, cannot open
partial initialization, and cannot implement this cleanup protocol. Legacy
unbound init/restore/atomic staging, the empty pre-binding creation window,
general lock-release interruption and physical power-loss evidence remain
separate. There are no private keys in the pre-binding window.
