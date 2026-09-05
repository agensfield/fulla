# Fulla Product Spec

## Spec State

This is the canonical implementation contract for Fulla. Decisions under
**Locked** are implementation constraints. Arda approved the complete contract
on 2026-08-12; implementation begins in a fresh thread.

## Product Thesis

Build Fulla as one local-first password-manager binary for Arda, his agents,
and all of his machines. Human use should stay small and direct. Agent use
should be explicit, structured, noninteractive when requested, and safe against
accidental secret disclosure or overwrite.

The product is a new Agensfield descendant of `biox/pa`, not an upstream-branded
rewrite and not merely a renamed build of the existing fork.

## Locked

### Identity and license

- Use **Fulla** as the product name and `fulla` as the CLI binary.
- Publish under the `agensfield` GitHub organization.
- License the combined work as AGPL-3.0-or-later.
- Preserve upstream copyright and license notices, identify `biox/pa` ancestry
  prominently, and make corresponding source available with distributed
  binaries.
- Do not imply that upstream endorses the Go implementation or Agensfield
  roadmap.

### Product shape

- Ship one maintained Go binary.
- Put ordinary password operations, peer management, synchronization, backups,
  and recovery under one command hierarchy.
- Do not ship `pa-xfer` as a compatibility executable or alias.
- Human-readable output is the default; machine-readable behavior must have an
  explicit stable contract.
- Release-blocking platforms are macOS and Linux on arm64 and amd64.
- Implement in a fresh thread only after this spec is marked implementation-ready.

### Release and compatibility policy

- Publish the first preview as `v0.1.0`.
- Earn `v1.0.0` only after real-store adoption, cross-host synchronization,
  recovery, and security acceptance gates pass.
- During `0.x`, commands, metadata, and machine contracts may change, but no
  release may strand an existing store. Every state migration requires an
  explicit forward path and an explained rollback boundary.
- In `1.x`, treat the `pa-v1` store profile, canonical CLI meanings, documented
  exit semantics, and the current versioned JSON schema as compatibility
  contracts. Human-readable prose and formatting are not compatibility
  contracts.
- Maintain one current implementation rather than accumulating compatibility
  layers. Config and feature-domain metadata move forward through direct,
  transactional migrations.
- During a rolling cross-host upgrade, synchronization needs to negotiate only
  the current and immediately previous protocol generations. Older protocol
  generations fail closed with actionable upgrade guidance.
- Follow semantic versioning after `v1.0.0`. Compatible additive JSON fields may
  appear within a schema version; a breaking machine-contract change requires a
  new schema identifier and, where applicable, a new major release.

### Command and secret-I/O principles

- Running `fulla` without arguments prints concise help and examples. It never
  selects a store-dependent action implicitly.
- Use canonical words for the public command contract. Everyday store commands
  begin with `add`, `show`, `edit`, `list`, `remove`, and `move`.
- Canonical command names are normative in documentation, examples, receipts,
  and machine output. Common semantic aliases invoke exactly the same behavior.
- Lock the daily alias map as: `setup` for `init`; `new` and `create` for `add`;
  `get` and `cat` for `show`; `clip` for `copy`; `update` for `edit`; `ls` for
  `list`; `rm`, `delete`, and `del` for `remove`; `mv` and `rename` for `move`;
  `exec` for `run`; `st` for `status`; and `check` and `diagnose` for `doctor`.
- Grouped domains inherit only obvious aliases such as `ls`, `rm`, `info`, and
  `recover`; do not mechanically add every synonym at every level.
- Do not preserve the predecessor's single-letter `a`, `d`, `e`, `l`, `s`, and
  `m` aliases. Keep `cp` unused because entry duplication and copying plaintext
  to the clipboard are materially different operations.
- Provide global `-h`/`--help` and `-v`/`--version` flags.
- `fulla show NAME` writes exactly the decrypted entry bytes to standard output,
  without banners or TTY-dependent behavior. Clipboard use belongs to the
  explicit `fulla copy NAME` command.
- Entry values are exact byte sequences, not implicitly UTF-8 strings. Fulla
  does not add or remove newlines, normalize text, or otherwise transform
  decrypted content on raw input and output paths.
- Interactive `fulla add NAME` offers a guided choice between generating a
  value, entering it without echo, and using an editor.
- `add` requires the name to be absent and `edit` requires it to exist. Do not
  provide an upsert, `set`, force-add, or force-edit path.
- Noninteractive writes require an explicit source: exact-byte `--stdin`, an
  inherited `--from-fd N`, or generation. Never accept secret bytes as a
  positional argument.
- Do not accept `PA_PASSWORD`, a replacement secret-value environment variable,
  or another ambient environment value as secret input. Environment variable
  names may still be used as explicit output destinations for child injection.
- Include `fulla run --env ENV_NAME=ENTRY -- COMMAND...` in v1 so agents can use
  credentials without surfacing plaintext in their prompts or tool transcripts.
- `fulla run` executes the target directly rather than through a shell, injects
  only explicitly mapped entries, prints no secret material, and returns the
  child's exit status. Reject values that cannot be represented safely in the
  target environment, including NUL bytes.
- The launched program is inside the trust boundary and may expose its own
  environment or output. Fulla cannot promise to suppress child-process leaks.
- Implement `run` as Unix process-image replacement rather than a supervising
  parent. Resolve and validate everything first, then replace Fulla with the
  target so PID, terminal, standard streams, working directory, signals, process
  group, and exit behavior remain native.
- Inherit the caller's ordinary environment by default and override only the
  explicitly mapped names. Reject duplicate or invalid mappings before
  decrypting secrets.
- Provide `--clean-env` for an explicit allowlisted environment. Executable
  resolution occurs before replacement; the target receives only selected
  inherited names and injected mappings.
- Do not invoke a shell or reinterpret target arguments. The target may inherit
  credentials into descendants, expose them to same-user inspection, or log
  them; these effects remain inside the locked process trust boundary.
- Group manual bundle operations under
  `fulla transfer export|verify|import`; keep them public for air-gapped transfer
  and protocol recovery without crowding the daily namespace.
- Provide typed history and transactional-backup inspection and restoration
  commands. Retain `fulla git -- ...` as an expert escape hatch rather than the
  documented recovery journey.

### Locked v1 command hierarchy

```text
fulla
├── init
├── add
├── show
├── copy
├── edit
├── list
├── remove
├── move
├── run
├── sync
├── peer add|list|show|rotate|remove
├── identity show|rotate
├── transfer export|verify|import
├── history list|show|restore
├── backup list|show|restore|prune|export
├── status
├── doctor
├── git
├── completion
├── version
└── remote serve
```

- Daily operations stay flat; trust, identity, transport, and recovery domains
  stay grouped.
- `remote serve` is an advanced but documented stable protocol entrypoint for
  SSH forced-command and remote-operation integration.
- Do not add standalone `generate`, `set`, `get`, or `search` canonical commands
  in v1. Generation is an `add` input mode; `get` remains an alias for `show`.
- Do not add a configuration command in v1. Configuration remains the documented
  TOML file plus flags and narrow environment overrides.

### Machine-readable output

- Fulla-owned control commands support `--json` and emit one versioned JSON
  document with schema identifier `fulla.cli/v1`.
- Successful envelopes contain `schema`, `ok: true`, canonical `command`, typed
  `data`, and a `warnings` array. Failed envelopes contain `schema`, `ok: false`,
  canonical `command`, and a typed `error` object with stable symbolic `code`,
  human `message`, and command-specific `details`.
- JSON-mode operational failures still return a nonzero process status. Do not
  require agents to infer success from prose or exit status alone.
- Aliases never appear as distinct command identities in JSON; emit the
  canonical command name.
- Default `fulla show NAME` remains exact raw plaintext bytes. Explicit
  `fulla show NAME --json` returns the value as always-base64 content with an
  explicit encoding marker inside the standard envelope, preserving arbitrary
  bytes without a conditional UTF-8 schema.
- Prefer `fulla run` when an agent only needs to provide a secret to another
  process. `show --json` is an intentional plaintext-retrieval operation and is
  not safer merely because its bytes are base64-encoded.
- Reject `--json` on true passthrough surfaces whose standard streams belong to
  another protocol: `run`, `git`, shell-completion generation, and binary
  `transfer export` to standard output. Never silently mix JSON with child,
  script, or ciphertext output.
- Metadata and error envelopes never include secret values, private identities,
  decrypted snippets, or child environments unless the invoked operation is
  explicitly `show --json`.
- Evolve the JSON schema independently from the Fulla binary version. Additive
  compatible fields may appear within `fulla.cli/v1`; breaking shape or meaning
  requires a new schema identifier.

### Interaction, errors, and process status

- `--json` implies noninteractive operation and never prompts. Provide
  `--non-interactive` for scripts that want human-readable output without
  prompts.
- Interactive prompts communicate through the controlling terminal rather than
  standard input. Standard input remains an exact secret-data channel.
- When no controlling terminal is available, or noninteractive operation lacks
  required input or authority, fail without mutation using
  `interaction.required` and describe the missing explicit option or evidence.
- Every interactive v1 workflow has complete noninteractive parity. Agents can
  perform every operation when supplied the same input, expected fingerprint,
  or scoped destructive acknowledgement that a human would provide.
- `--yes` accepts only routine confirmations. It never substitutes for an
  independently expected peer fingerprint, permanent untracked deletion
  acknowledgement, or retired-key destruction acknowledgement.
- Do not preserve `PA_NOCONFIRM` or introduce a universal prompt-bypass
  environment variable.
- Use stable namespaced symbolic error codes such as `store.uninitialized`,
  `entry.not_found`, `peer.trust_mismatch`, and `sync.partial`. Human messages
  may improve compatibly; each code's detail fields remain typed and documented.
- Lock the Fulla-owned process-status taxonomy as:
  - `0`: success, including policy-defined sync skips unless strict mode applies;
  - `1`: operational failure before live mutation;
  - `2`: invalid invocation or usage;
  - `3`: live state changed but the requested workflow, finalization, or
    reporting did not complete;
  - `4`: a caller-requested strict condition such as `--fail-on-skip` was not
    satisfied.
- Passthrough commands return the child or Git process status. Signal
  termination preserves conventional `128 + signal` behavior.
- Human-mode diagnostics go to standard error. JSON-mode Fulla failures return
  one valid error envelope and the corresponding nonzero status.

### Configuration boundary

- Use a small non-secret TOML user config at
  `$XDG_CONFIG_HOME/fulla/config.toml`, falling back to
  `$HOME/.config/fulla/config.toml` when `XDG_CONFIG_HOME` is unset.
- Support explicit `--config PATH` and `FULLA_CONFIG` discovery overrides.
- Resolve ordinary settings in this order: command flag, selected `FULLA_*`
  environment variable, TOML value, compatibility fallback where explicitly
  defined, then compiled default.
- Resolve the store as: `--store`, `FULLA_DIR`, TOML store setting, `PA_DIR`
  compatibility fallback, then `$XDG_DATA_HOME/fulla` or
  `$HOME/.local/share/fulla`.
- `PA_DIR` selects only a candidate store location. It never bypasses validation
  or performs silent adoption.
- Keep the Fulla environment surface minimal and operational. Do not mirror
  every TOML setting into an environment variable.
- Continue honoring standard operating-system/tool variables where relevant,
  including `HOME`, XDG variables, `EDITOR`/`VISUAL`, `NO_COLOR`, `TMPDIR`, and
  OpenSSH/Git integration variables.
- Do not honor legacy `PA_LENGTH`, `PA_PATTERN`, `PA_NOCONFIRM`, or `PA_PASSWORD`.
  Generation, clipboard, retention, and other preferences belong in TOML or
  explicit flags.
- Never allow secret values, private identities, peer trust records, or recovery
  material in the user config. Those belong to invocation data or versioned
  private store metadata.
- Create config files mode `0600` and require that they are owned by the current
  user and not writable by group or others before accepting security-relevant
  paths or executable preferences.
- Reject unknown keys, duplicate definitions, type errors, and invalid values
  before store access, naming the exact offending key and expected shape.
  Handle renamed settings through explicit deprecation warnings and documented
  migrations rather than silent reinterpretation.
- `status` and `doctor` report the resolved config/store paths and setting
  sources without printing secret material.

### Destructive operations and recovery

- `move` requires the destination name to be absent and never overwrites it.
- Do not provide a general `--force` overwrite escape hatch for ordinary store
  mutations.
- In a Git-backed store, `remove` deletes the encrypted entry and commits the
  deletion. Fulla's recovery surface can restore it from encrypted history.
- In an untracked store, deletion is genuinely irreversible and requires an
  explicit permanent-delete acknowledgement in both interactive and
  noninteractive use.
- Do not support wildcard, glob, category-prefix, or bulk deletion in v1.
- Fulla does not add a separate trash or tombstone store in v1.
- Retain encrypted transactional backups until an explicit prune by default.
  Never silently expire recovery material.
- `status` reports backup age and size. `backup prune` supports a preview and
  explicit configurable retention criteria before deletion.

### Initialization and store discovery

- Only `fulla init` may create a store. Any store-dependent command against an
  uninitialized location fails without mutation and points to `fulla init`.
- A fresh store defaults to `$XDG_DATA_HOME/fulla`, falling back to
  `$HOME/.local/share/fulla` when `XDG_DATA_HOME` is unset.
- When no Fulla store exists but a compatible `pa` store does, report it and
  offer an explicit adoption path. Never silently move, copy, or duplicate the
  existing store or its identity.
- When both stores exist, the resolved Fulla store wins. Surface the `pa` store
  only through diagnostics or an explicit adoption/migration command.
- Git history is enabled by default for a new store. Show that choice and its
  metadata leakage in the initialization preflight; `--no-git` opts out.
- Initialization must be transactional: a failed or cancelled run leaves no
  usable partial store and never replaces existing store material.

### Dependency boundary

- Embed the official Go `age` implementation. Ordinary password operations do
  not require the `age` or `age-keygen` executables.
- Keep the system `git` executable as an optional first-release integration.
- Keep the system OpenSSH `ssh` executable as an optional first-release
  integration.
- Do not reimplement Git repository semantics, OpenSSH configuration, SSH
  agents, hardware keys, jump hosts, or known-host behavior in the first release.

### Identity support and local trust

- Generate a dedicated native age X25519 identity for a new store by default.
- `fulla identity show` displays only the public recipient and its fingerprint.
  No casual inspection command prints private identity material.
- Handle supported native age identity forms through the embedded official Go
  libraries, including compatible SSH identity forms where supported by the
  official `agessh` package.
- Support compatible age plugin identities by invoking the explicitly installed
  `age-plugin-<name>` executable named by the identity. Never install plugins
  automatically, and report a missing plugin precisely.
- Fulla continues to own encrypted-file and plaintext handling. A plugin is
  trusted local code used only to wrap or unwrap file keys for its identity and
  may mediate hardware touch or PIN interaction.
- Store the default local identity in a permission-restricted file and treat
  processes running as the store-owning OS user as trusted in v1. This enables
  unattended agents but is not an isolation boundary between same-user agents.
- Document that Fulla primarily protects secrets at rest, in encrypted history,
  in backups, and in transit. It does not defend against a process that already
  has the store owner's filesystem and execution privileges.

### Local plaintext and filesystem boundaries

- Treat the configured external editor as trusted code inside the plaintext
  boundary. Create edit material in a private `0700` temporary directory with a
  `0600` file, prefer memory-backed storage where the operating system provides
  it, and clean rigorously on normal exit and handled signals.
- State plainly that editor swap files, backups, plugins, crash recovery, and
  telemetry are outside Fulla's control. Noninteractive stdin and descriptor
  editing remain the safer agent path.
- `fulla copy` clears the clipboard after 45 seconds by default, but only when
  it still contains Fulla's value. Retain only a digest for delayed comparison.
- Make clipboard expiry configurable, including disabled, and provide per-call
  duration and no-clear options. Clipboard managers remain outside the security
  guarantee.
- Canonicalize and root every store path. Reject symlinks, unexpected hard-link
  relationships, non-regular private files, and path traversal before decrypting
  or mutating data.
- Fail closed on unsafe POSIX permissions for private store material. Report the
  exact problem; only explicit `doctor --fix-permissions` may repair safe mode
  issues. Never silently rewrite intentional ACLs during ordinary commands.
- Ordinary `doctor` checks layout, paths, permissions, Git state, peers, plugins,
  and ciphertext structure without decrypting entry content.
- Explicit `doctor --deep` decrypts every entry, discards its bytes without
  logging them, and reports only names and outcomes. It may trigger hardware
  identity touch or PIN interaction.

### Logging, terminals, and crash diagnostics

- Do not maintain a persistent general, audit, or debug log in v1. Emit redacted
  diagnostics only for the current invocation.
- State-changing receipts contain the minimum non-secret names, fingerprints,
  counts, timestamps, and applied-state evidence required for recovery. They
  never contain plaintext values, private identities, injected environments, or
  decrypted snippets.
- Raw `show` and `show --json` are deliberate disclosure operations. Terminal
  scrollback, shell capture, and caller-controlled output redirection are outside
  Fulla's guarantee; errors and warnings never repeat the value.
- Release builds catch unexpected internal panics and return the redacted
  `internal.failure` error without a raw stack or automatic crash file.
- Provide an explicit private diagnostic-report path with redaction. Full panic
  stacks remain available in development and test builds, not ordinary release
  output.

### Portable and full-state recovery exports

- Default manual `transfer export` contains logical live entry names and exact
  values encrypted to an explicit destination or recovery recipient. Exclude
  active identities, peers, Git history, receipts, local transaction backups,
  and retired-key artifacts.
- Support an explicit manifest or equivalent selector for exporting only named
  logical entries. A scoped recovery capsule must not require exporting every
  live password-store entry. Reject absent names before writing the artifact,
  and record only the selected names and non-secret verification metadata in
  its receipt.
- Add explicit `backup export --full` for a complete disaster image containing
  the active identity, ciphertext store, Git history, peers, receipts,
  transaction backups, and retired-key recovery material.
- Protect a full-state archive with either a separate age recovery recipient or
  a strong passphrase obtained from the controlling TTY or inherited descriptor.
  Never accept the passphrase through argv or an ambient environment value.
- Reject protection solely by the active identity contained inside the archive;
  that is circular recovery if the active identity is lost.
- Write exports atomically, mode `0600`, and without replacing an existing
  destination. Standard-output export remains an explicit binary passthrough.
- Full-state restore operates only on an empty target and represents replacement
  of a lost machine, not onboarding another live peer. Stage, validate, and
  verify the complete archive before publishing any restored state.
- Warn that restoring the full image alongside the original machine clones its
  identity and peer authority. External archive copies cannot be revoked by
  deleting the local copy.

### Identity rotation

- Ship `fulla identity rotate` as a complete v1 workflow rather than a manual
  migration note.
- Rotation creates a new identity, transactionally re-encrypts every live entry,
  verifies the staged store with the new identity, and only then publishes the
  new ciphertext, recipient, and active identity.
- Continuity mode is the default. Encrypt the retired private identity to the
  new recipient and retain it outside the password Git repository in private
  recovery storage so old encrypted history remains recoverable.
- Retired identities are loaded only by explicit history/recovery operations,
  not as ambient active decryption identities.
- Record a non-secret rotation receipt containing old and new public
  fingerprints, timestamps, applied state, and recovery-artifact location.
- Rotation changes this machine's pinned recipient. Every peer fails closed
  until its owner explicitly completes the normal peer trust-rotation and
  verification flow.
- Provide an explicit destructive mode shaped as
  `fulla identity rotate --destroy-retired-key`. It requires successful
  post-rotation verification and a typed irreversible acknowledgement before
  discarding local access to the old identity and old Git history.
- Destruction cannot revoke old identities or ciphertext already copied into
  other machines, clones, or backups. State this plainly.
- Do not rewrite Git history during identity rotation in v1.
- When rotation is marked as compromise-driven, report that re-encryption does
  not rotate the underlying passwords, tokens, or credentials. Produce a
  non-secret inventory of affected entry names as an external credential-
  rotation checklist.

### Store compatibility

- Name the inherited interoperable layout and behavior profile `pa-v1`.
- Existing shell `pa` and the new Go product must be able to read and write the
  same supported stores without a format migration.
- Preserve the existing `$PA_DIR` layout, identity and recipient files,
  password paths, `.age` ciphertext interoperability, and Git repository/history.
- Preserve ordinary command meanings, useful aliases, safe environment inputs,
  script-relevant outcomes, and stable exit semantics.
- Human prompts, help, hierarchy, diagnostics, and formatting may be redesigned.
  Byte-for-byte CLI output compatibility is not required.
- Compatibility applies to safe existing behavior. Unsafe path handling,
  plaintext-temporary formats, and ambiguous operations are not protected merely
  because an older implementation allowed them.
- Guarantee shell `pa` CRUD interoperability while a store remains on `pa-v1`.
  Compatibility covers live entry and encrypted Git-history operations, not
  every Fulla peer, receipt, backup, recovery, or diagnostic feature.
- Fulla and shell `pa` may alternate mutations through the shared store lock but
  must never mutate concurrently.
- Never silently migrate a store away from `pa-v1`. Any future Fulla-native
  format requires an explicit opt-in migration, an explained rollback boundary,
  and continued deliberate support policy for `pa-v1`.

### Adoption and protocol cutover

- `fulla init --adopt` adopts an existing compatible pa directory in place.
  Validate it before adding namespaced Fulla metadata; do not copy, move,
  re-encrypt, or replace its identity.
- Keep shell `pa` available as a supported basic CRUD rollback client after
  adoption.
- Treat synchronization as a one-way protocol cutover. Once mutual Fulla pairing
  and sync are activated for a store, retire `pa-xfer` for that store.
- Existing `pa-xfer` peer records may inform enrollment, but they do not satisfy
  Fulla's mutual authorization requirement. Mark imported pins as legacy and
  incomplete until both sides finish Fulla verification.
- Fulla cannot prevent an operator from invoking an old `pa-xfer` binary. Doctor
  and migration guidance must state that doing so reopens the weaker legacy
  SSH-only authorization path.

### Exact upgrade and rollback journey

1. Install Fulla without changing the active pa store or removing shell `pa`.
2. Run `fulla init --adopt --dry-run` against the existing directory. Validate
   canonical structure, permissions, Git state, identity and recipient
   consistency, required plugins, and decryption of every entry while discarding
   plaintext.
3. Run the real adoption. Repeat validation under the store lock, then
   atomically publish only namespaced Fulla metadata and a non-secret adoption
   receipt. Do not write a synthetic canary entry.
4. Continue ordinary shell pa CRUD as needed while evaluating Fulla. Both tools
   honor the shared lock and do not mutate concurrently.
5. On every participating host, install Fulla, adopt or initialize its store,
   complete deep verification, and complete mutual peer enrollment.
6. Require the first mutually authenticated `fulla sync PEER --dry-run` to
   succeed before any real sync. Record the inventory plan without decrypting
   shared-name values.
7. The first real Fulla sync marks that peer's protocol as activated and records
   `pa-xfer` retirement. Subsequent pa-xfer use is an explicit security
   downgrade, never an automatic fallback.
8. To roll back basic operations, stop Fulla mutations and resume shell pa CRUD
   on the same `pa-v1` directory. Leave Fulla metadata, receipts, peers, backups,
   and recovery evidence intact.
9. Do not resume pa-xfer implicitly during rollback. Prefer restoring a known
   Fulla binary or using audited manual encrypted bundles for transfer recovery.

- Version Fulla metadata by domain rather than tying it to the live entry
  format. Within v1, older binaries retain safe `pa-v1` CRUD but fail closed on
  peer, sync, backup, identity, or recovery metadata newer than they understand.
- Domain metadata upgrades are transactional and never silently migrate the
  `pa-v1` live store. An interrupted upgrade leaves either the old valid domain
  state or the new valid state plus explicit applied-state evidence.

### Synchronization contract

- Each machine retains its own age identity.
- Synchronization is a set union by entry name.
- Never overwrite an existing destination entry during sync.
- Skip a shared name even when the plaintext values differ.
- Preserve pinned peer trust and require an explicit trust-rotation operation.
- Keep encrypted transfer, private snapshots, receipts, clean-store checks,
  locking, cancellation cleanup, and applied-state reporting.
- Preserve the low-level protocol and recovery invariants proven in the accepted
  `pa-xfer` baseline even though its executable surface disappears.
- `fulla sync PEER` performs a bidirectional set union by default. It pushes
  locally unique names and pulls remotely unique names in one user workflow.
- Each host applies its side transactionally, but Fulla does not claim a global
  distributed transaction across both hosts.
- If one direction commits and the other fails, report the exact applied state
  and retain receipts. Rerunning is the supported recovery path and safely
  converges because sync never overwrites.
- Shared-name skips are a successful policy outcome. Human output, JSON, and
  receipts report their names and counts; `--fail-on-skip` provides a strict
  nonzero result for automation.

### Peer trust and authorization

- Daily synchronization addresses a saved peer by a human-sized name such as
  `fulla sync devbox`; fingerprints are enrollment and rotation material, not
  arguments humans memorize.
- Enrollment fetches the remote recipient over SSH, shows its fingerprint and
  an exact remote verification command, and requires explicit confirmation
  after out-of-band verification before pinning it.
- Pairing is mutual. Each machine independently pins and authorizes the other;
  successful SSH authentication alone does not authorize remote sync, export,
  or secret retrieval.
- A changed recipient always aborts before local secret decryption or mutation.
  Trust changes require an explicit peer-rotation command and repeat the
  verification flow.
- Never silently replace a peer record. Preserve enough previous-record and
  receipt evidence to diagnose or recover from an accidental rotation.
- Use existing OpenSSH configuration and accounts as the default transport.
  Document a dedicated restricted key with a forced `fulla remote serve`
  command as the hardened profile, but do not require it for ordinary peers.
- Before exposing inventory or ciphertext operations, require a fresh
  application-level proof of the mutually pinned age identity. Encrypt a random
  one-use challenge to the saved recipient and authorize the SSH session only
  after the caller decrypts and returns it.
- Bind challenge authorization to one SSH/protocol session and reject replay.
  Plugin-backed identities may require their normal touch or PIN interaction.

### Git behavior

- Existing Git-backed stores continue automatic encrypted-history commits.
- New stores enable Git history by default as a visible part of `fulla init`,
  not as a side effect of an ordinary password command or merely because `git`
  is present on `PATH`.
- `fulla init --no-git` creates an untracked store deliberately.

### Distribution

- Repository: `github.com/agensfield/fulla`.
- Local checkout: `<checkout>`.
- Follow the established Scriba distribution lane: tagged releases, checksums,
  macOS/Linux arm64/amd64 archives, `go install`, and the Agensfield Homebrew tap.
- Homebrew surface: `brew install agensfield/tap/fulla`.

## Locked v1 Non-Goals

Fulla v1 is a focused local secret custodian. It explicitly excludes:

- Windows support
- embedded Git or embedded SSH implementations
- hosted/cloud accounts or a central secret service
- organization/team administration
- multi-user access control, roles, or policy administration
- browser extensions
- desktop or mobile GUI applications
- browser/mobile autofill and consumer account UX
- an always-on general-purpose daemon or public local agent API
- automatic conflict resolution or last-writer-wins sync
- overwriting an existing secret during ordinary synchronization
- backwards-compatible installation of the `pa-xfer` executable

The restricted `remote serve` transport and direct `fulla run` execution
surface remain in scope. They do not establish a general daemon, SDK service,
or agent-control plane.

## Open Interview Questions

None. Product and behavioral scope is locked. The remaining work before
implementation is complete.

## Acceptance Contract

### Required end-to-end journeys

| Journey | Required proof |
| --- | --- |
| Fresh human store | Initialize with and without Git, add/show/copy/edit/move/remove exact-byte entries, inspect history/backups, and recover a deletion. |
| Existing pa store | Dry-run adoption, adopt in place, alternate locked Fulla and shell-pa CRUD, activate Fulla sync, then stop Fulla and resume shell-pa CRUD without changing the `pa-v1` live format. |
| Noninteractive agent | Perform every supported workflow without a prompt, receive canonical `fulla.cli/v1` results, inject selected entries through `fulla run`, and never require a secret in argv or ambient environment. |
| Two-host operation | Mutually enroll independently verified peers, complete the mandatory first dry-run, converge disjoint entries bidirectionally, skip shared names, and rerun safely after a one-sided partial application. |
| Trust change | Abort a recipient mismatch before decryption, complete explicit peer rotation, reject replayed challenges, and resume only after mutual authorization is restored. |
| Identity lifecycle | Rotate transactionally, retain sealed history continuity by default, restore old encrypted history explicitly, and exercise destructive retired-key removal with irreversible acknowledgement. |
| Disaster recovery | Export and import a logical bundle, create a separately protected full-state archive, restore it only into an empty target, and verify complete state before publication. |
| Scoped infrastructure recovery | Export an explicit Agensfield entry manifest to a separate recovery recipient or controlling-TTY passphrase, verify the capsule without mutating the live store, reject whole-store overreach and circular protection, then prove an isolated recovery read. |
| Failure recovery | Interrupt every state-changing workflow at its staged publish boundaries and prove the live store remains old-valid or new-valid with accurate applied-state evidence. |

### Compatibility and migration fixtures

The implementation test suite must keep versioned, non-secret fixtures for:

1. A pristine Git-backed `pa-v1` store created by shell `pa` with a native age
   X25519 identity.
2. An untracked `pa-v1` store and a Git-backed store with nested entry names,
   deletion, rename, and recoverable encrypted history.
3. Exact entry values covering empty bytes, a trailing newline, embedded NUL,
   invalid UTF-8, and a representative large payload.
4. A mock age-plugin identity plus missing-plugin and interaction-required
   outcomes. Hardware-specific tests may remain an opt-in integration lane.
5. Legacy `pa-xfer` peer pins that may seed enrollment but never count as mutual
   Fulla authorization.
6. Shell-pa mutations made before adoption, after adoption, and after a basic
   Fulla rollback.
7. Each versioned Fulla metadata domain in previous, current, malformed, and
   unknown-future forms.
8. Interrupted domain migrations captured before staging, after staging, and at
   atomic publication boundaries.
9. Two-host inventories with disjoint names, shared equal names, shared unequal
   values, and a failure after only one direction commits.
10. Sync protocol pairs for current/current and current/immediately-previous,
    plus an older generation that must fail closed without mutation.
11. Recipient mismatch, unpaired SSH access, expired/replayed challenges, unsafe
    permissions, symlinks, unexpected hard links, traversal attempts, and dirty
    Git states.
12. Logical transfer bundles, encrypted-history recovery, transaction backups,
    sealed retired identities, and separately protected full-state archives.

Every persisted-state change introduced during `0.x` adds fixtures for the old
state, successful migration, interruption, idempotent retry, and the documented
rollback boundary. We support direct migration into the current state, not a
permanent runtime implementation of every historical state.

### Testable security invariants

1. Ordinary commands never create a store, silently adopt one, or replace an
   existing entry, destination, export, identity, peer record, or restore target.
2. Fulla validates configuration, canonical paths, file type and permissions,
   invocation mappings, and peer trust before decrypting secret material.
3. A changed or unauthorized peer recipient causes zero local decryption and
   zero mutation.
4. Every mutation holds the shared store lock and publishes atomically. A crash
   leaves an old-valid or new-valid state, never a partially usable store.
5. Synchronization never overwrites a shared name or compares its plaintext.
   Partial cross-host success is reported precisely and safely converges on retry.
6. Plaintext values enter or leave Fulla only through an explicitly selected
   channel: raw `show`, base64 `show --json`, controlling-TTY input, stdin, an
   inherited descriptor, the trusted editor boundary, clipboard, or a mapped
   child environment.
7. Secret values and passphrases are never accepted through argv or ambient
   secret-value environment variables, and never appear in errors, warnings,
   receipts, diagnostics, logs, or panic output.
8. `--json` and `--non-interactive` never prompt. Missing authority or evidence
   fails before mutation with a typed error.
9. Private files and exports are created without replacement at mode `0600`;
   private directories use mode `0700`. Unsafe paths and modes fail closed.
10. Clipboard clearing removes a value only if the clipboard still contains the
    copied bytes; delayed state retains only a digest.
11. `fulla run` validates all mappings before decryption, invokes no shell, and
    replaces the process image after successful preparation.
12. Peer challenges are fresh, one-use, bound to one protocol session, and
    accepted only from the independently pinned identity.
13. Identity rotation publishes nothing until all live entries decrypt under
    the staged new identity. Default rotation preserves old-history access
    without making the retired identity ambient.
14. A full-state archive is never protected solely by the active identity stored
    inside it and is never restored over a non-empty target.
15. Release builds convert unexpected panics into redacted `internal.failure`
    output without persisting a stack trace or secret-bearing crash artifact.
16. Fulla makes no false isolation claim between processes sharing the store's
    Unix account; tests and documentation preserve this explicit trust boundary.

### Release gates

- `v0.1.0` requires the complete command tree to build on macOS and Linux
  arm64/amd64; unit, integration, compatibility, migration, race, and static
  analysis gates to pass; the fresh-store, pa-adoption, agent, two-host, and
  recovery journeys above to run successfully; and preview limitations to be
  documented.
- `v1.0.0` additionally requires repeated use against real stores on at least
  two real hosts, a successful disaster-restore drill, identity and peer
  rotation drills, current/previous rolling-upgrade proof, security-invariant
  review, signed release artifacts with checksums, and no unresolved issue that
  can lose, disclose, silently overwrite, or irrecoverably strand secret state.

## Definition of Implementation-Ready

Satisfied on 2026-08-12:

- [x] every open interview question has a recorded answer;
- [x] the selected name passes a documented collision check;
- [x] the human and agent journeys are concrete enough to write acceptance tests;
- [x] compatibility fixtures and migration/rollback tests are enumerated;
- [x] security invariants are written as testable statements;
- [x] v1 scope and non-goals are explicit;
- [x] Arda explicitly accepts the final synthesis.

The next agent should create `github.com/agensfield/fulla` at
`<checkout>`, use this document as the
scope authority, and begin with a phased implementation plan mapped directly to
the acceptance contract. Scope changes return here as explicit spec revisions.

