package cli

const help = `Fulla: a local-first secret custodian (development build)

Daily use:
  init [--no-git]                    Create a private store after confirmation
  init --adopt --dry-run             Inspect a compatible pa store for adoption
  add NAME                          Choose generation, hidden input, or editor
  edit NAME                         Edit an existing value with the trusted editor
  show NAME                         Write exact bytes; add no newline
  copy NAME                         Copy text with conditional expiry (default 45s)
  list                              List entry names
  move OLD NEW                      Move without overwriting
  remove NAME                       Remove an entry; no-Git needs --permanent-delete
  run --env KEY=ENTRY -- COMMAND...  Inject a selected value and execute directly

Trust and sync:
  identity show                     Show public recipient and fingerprint
  identity rotate --yes              Rotate keys, retaining sealed history by default
  peer add NAME --host HOST          Discover identity; pin with --expect-fingerprint
  peer list | peer show NAME         Inspect enrolled peers
  peer rotate NAME                  Repin with an independently verified fingerprint
  peer remove NAME                  Confirm revocation of local peer authorization
  sync NAME --dry-run                Required first authenticated sync preflight
  sync NAME                         Merge unique names; preserve shared names
  remote serve                      Serve the authenticated protocol over SSH streams

Recovery:
  history list [NAME]                Inspect encrypted Git history
  history show COMMIT                Inspect a full commit hash
  history restore COMMIT NAME        Confirm restoration of one entry
  backup list | backup show ID       Inspect encrypted transaction snapshots
  backup restore ID [--phase before] Confirm restoring the snapshot entry set
  backup prune --keep N --dry-run    Preview retention; --yes applies it
  transfer export --output PATH     Export logical entries; --manifest PATH scopes names
  transfer verify PATH              Verify with independent recovery material
  transfer import PATH              Import logical entries without overwriting
  backup export --full --output PATH Export complete identity, history and recovery state
  backup restore --full PATH        Restore into an empty target; clones identity/authority

Inspection and integration:
  status                            Show store/config sources and backup summary
  doctor [--deep]                    Inspect structural health; --deep checks decryption
  doctor --report PATH               Write a private redacted diagnostic report
  doctor --fix-permissions           Explicitly repair supported private mode issues
  doctor --recover-lock TOKEN        Recover a verified dead owner's operation
  git -- ARGS...                     Expert Git escape hatch with native streams
  completion bash|zsh|fish            Generate shell completion code
  version                           Print the CLI version

Examples:
  fulla add api/token --stdin        Read exact bytes from an explicitly selected pipe
  fulla add api/token --generate     Generate without printing the value
  fulla run --env TOKEN=api/token -- your-command

Writes: --stdin, --from-fd N, or --generate; never put secret values in argv.
Exports require --recipient RECIPIENT, --passphrase (hidden terminal input),
or --passphrase-fd N. Isolated verification/restore requires --identity PATH
or a passphrase source. --passphrase takes no value; export confirms it twice.
Backups are retained until explicit prune. Clipboard managers may retain copies.

Global: --store PATH, --config PATH, --json, --non-interactive, --yes,
        -h/--help, -v/--version
--json never prompts; show uses base64. Child/protocol/binary-export streams reject JSON.
--yes supplies routine confirmation, never a fingerprint or destruction acknowledgement.
This development build has not completed preview release qualification.`
