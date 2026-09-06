# Ordered human acceptance

`scripts/acceptance-human.py` runs one complete journey on each fresh Git/no-Git
store. It uses the same controlling-terminal driver as the existing interactive
harness, with stdin disconnected from the terminal. All values, identities,
editor files and clipboard state are disposable fixtures.

Each journey confirms initialization, types a hidden value and an empty value,
chooses generation, adds binary/invalid-UTF-8/multiline bytes through the trusted
editor, edits the typed value, lists names, copies text with a one-second expiry,
moves the entry, inspects backups, deletes, declines recovery, and then confirms
recovery. Git mode inspects history and restores the deleted entry from its
pre-deletion commit. No-Git mode first refuses deletion without the explicit
permanent-delete flag, then restores the deletion snapshot's before state.

Final checks compare the complete name set and every exact value, deep doctor
health, absence of a lock or transaction staging, editor temporary cleanup and
clipboard expiry. Metadata/prompt operations must not echo fixture values. Raw
`show` output is captured deliberately for exact-byte verification rather than
writing binary control bytes to a human terminal. Explicit JSON observations
select opaque history/snapshot IDs, so those selections do not depend on the
presentation format of human metadata output.

Clipboard tools are fixture executables, never the host's actual clipboard.
Separate CI checks still cover real macOS/X11 backends; real Wayland remains
unverified. The shared PTY driver keeps the existing prompt deadlines, terminal
mode restoration checks, and owned-child cleanup. The original interactive
harness continues to cover cancellation, plugin and encrypted-SSH interaction,
adoption, and additional recovery refusal cases.

Run after building the local CLI:

```sh
go build -o dist/fulla .
python3 scripts/acceptance-human.py dist/fulla
```

Linux/macOS CI now runs this journey. Ruff and basedpyright check the changed
scripts; `pyrightconfig.json` describes `scripts` as the execution root used by
direct script invocation, without disabling diagnostics. The scoped Python cache
ignore keeps module imports from dirtying the release-packaging checkout.

This is bounded J1 journey evidence. Human output presentation, native Wayland,
broader shell installation and full-spec release qualification remain separate.
The no-Git test proves current retained-backup behavior; it does not settle the
spec's conflicting “irreversible” deletion and retained transactional-backup
wording. That decision remains pending, and retention has not been weakened.
