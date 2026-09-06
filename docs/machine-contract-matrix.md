# Machine input and authority matrix

All 34 canonical command leaves are listed below. `--json` is noninteractive;
`--non-interactive` selects non-prompting human output. An explicit data channel
is not permission to infer unrelated authority. Raw process/protocol surfaces
reject `--json` instead of mixing envelopes with their output.

`internal/cli/machine_contract_test.go` runs one read-only or refusal case per
leaf, on Git and no-Git stores. It verifies one `fulla.cli/v1` document, canonical
command, status/ok consistency, success warnings/data, typed error details, no
implicit stdin read, no metadata/error secret leakage, and unchanged fixture
file tree (including modes). The same non-passthrough cases also run with
`--non-interactive`, checking status, input isolation, raw-show bytes, and unchanged
state without treating human prose as a stable schema. This baseline does not execute every authorized mutation or prove every
TTY, inherited-FD, or subprocess behavior. The last column points to additional
workflow evidence; the full acceptance matrix retains remaining combinations.

| Command | Required input / authority | JSON baseline | Additional workflow evidence |
| --- | --- | --- | --- |
| `init` | Explicit store; `--yes` to create/adopt, or `--dry-run` | Missing confirmation refuses | `init.go`, TTY harness; real-pa adoption harness |
| `add` | Name plus `--stdin`, `--from-fd`, or `--generate` | Missing channel refuses | Exact-byte agent test; TTY and real-pa harnesses |
| `show` | Explicit name; raw bytes or base64 JSON | Selected fixture value succeeds | Exact-byte agent test and real-pa harness |
| `copy` | Explicit name; clipboard policy from config/flags | Missing name refuses before backend use | Clipboard harness, mocked backends and guarded real runners |
| `edit` | Existing name plus explicit channel; TTY/editor otherwise | Missing channel refuses | TTY/editor harness and real-pa harness |
| `list` | Selected validated store | Names succeed | Agent and real-pa journeys |
| `remove` | Name; no-Git requires `--permanent-delete` | Missing name in Git / missing no-Git authority refuses | Real-pa and store mutation/recovery tests |
| `move` | Existing source, absent destination | Occupied destination refuses | Real-pa and killed multi-entry transaction tests |
| `run` | Explicit mappings and executable after `--` | JSON refuses | Native PID/environment/exit acceptance in `run_test.go` |
| `sync` | Mutually enrolled peer; successful initial dry-run | Unknown peer refuses before transport | Remote protocol, partial-commit/retry, real OpenSSH harness |
| `peer add` | Name/host and independently verified expected fingerprint | Missing operands refuse | Real OpenSSH enrollment and trust mismatch tests |
| `peer list` | Selected store | List succeeds | Peer registry tests |
| `peer show` | Enrolled name | Public peer record succeeds | Peer registry tests |
| `peer rotate` | Existing name and independently expected new fingerprint | Missing operands refuse | Rotation, handled-error, killed-owner receipt recovery tests |
| `peer remove` | Existing name and `--yes` in machine mode | Missing confirmation refuses | Confirmation, receipt, handled-error and killed-owner tests |
| `identity show` | Selected store | Public identity succeeds | Identity and remote verification tests |
| `identity rotate` | `--yes`; destructive mode needs exact fingerprint-bound acknowledgement | Missing authority refuses | Continuity/destruction and killed-rotation tests |
| `transfer export` | Output and explicit recipient/passphrase source; optional exact-name manifest | Missing protection refuses | Logical transfer and recovery-input tests; stdout JSON refusal |
| `transfer verify` | Bundle plus independent identity/passphrase source | Missing independent identity refuses | Isolated logical verification and TTY passphrase tests |
| `transfer import` | Bundle; selected recovery identity or explicit passphrase source | Missing bundle refuses | No-overwrite imports and partial/strict outcomes |
| `history list` | Git store, optional name | Git succeeds; no-Git refuses | Historical/current binary harness |
| `history show` | Full commit hash | Git succeeds; no-Git refuses | History fixtures and historical binary harness |
| `history restore` | Commit/name and `--yes` in machine mode | Missing confirmation refuses | TTY confirmation and old-key history restoration |
| `backup list` | Supported backup metadata | List succeeds | Snapshot/domain and historical binary tests |
| `backup show` | Snapshot ID | Journal succeeds | Snapshot/domain and historical binary tests |
| `backup restore` | ID/phase and `--yes`; full archive also needs independent recovery material and empty target | Missing confirmation refuses | TTY, snapshot, full-archive and historical binary acceptance |
| `backup prune` | Explicit retention selection; `--yes` to apply | Without authority remains a preview | CLI preview and killed/handled prune recovery tests |
| `backup export` | `--full`, output and independent protection | Missing protection refuses | Circular-protection/full-disaster tests and TTY passphrase acceptance |
| `status` | Selected store/config | Summary succeeds | Config provenance and metadata tests |
| `doctor` | Selected store; repair/recovery authority supplied separately | Structural report succeeds | Deep/plugin/permission/lock diagnostics and recovery tests |
| `git` | Explicit child argv; native streams | JSON refuses | Git subprocess/locking tests |
| `completion` | Bash/Zsh/Fish | JSON refuses | Bash/Fish engine behavior and Bash/Zsh/Fish syntax tests |
| `version` | None | Version envelope succeeds | Native packaged-binary acceptance |
| `remote serve` | Protocol-owned streams; SSH access alone does not authorize operations | JSON refuses | Mutual remote session and real OpenSSH tests |

## Authorized agent journey

`scripts/acceptance-agent.py` runs the native binary without a controlling
terminal on generated Git and no-Git stores. It covers 26 successful canonical
leaves with Git and 22 without Git: explicit initialization, stdin/descriptor/
generation input, exact raw/base64 output, CRUD, history, snapshots, native run,
selected transfer, rotation continuity, full disaster restore, prune preview/apply,
status, deep doctor, version, and completion generation.

The native child checks process replacement, clean environment against a direct
runtime control, mapped value, exact stdin/stderr, working directory, literal
argv, process group/session, and exit status 23. Transfers use an independent
recovery identity, verify without creating a selected store, and preserve shared
names on import. Full restore compares every source path and file digest with
required private modes; export may add exactly its validated publication receipt.
Prune preview must leave all source contents and modes unchanged.

CI runs the journey on Linux and macOS. The eight other leaves (copy, sync, five
peer commands, remote serve) retain their specialized fixtures. This journey does
not prove every channel, signal, interactive workflow, or authority combination.

## Remaining parity proof

The baseline intentionally avoids invoking the user's clipboard, starting SSH
connections, replacing the test process, consuming an inherited secret descriptor,
or applying destructive commands. Those need their existing specialized fixtures
and a complete per-channel/authority audit. In particular, remaining agent
success combinations, controlling-terminal absence/availability for every interactive
workflow, and all signal/FD/process combinations remain part of J3/J1 in the
full-spec acceptance matrix. No row equates a missing-operand refusal with an
implemented successful workflow.

## Write eligibility before input

`add` and `edit` now inspect the selected entry before consuming an explicit
secret stream or opening interactive input. An already-existing add returns
`entry.exists`; editing a missing entry returns `entry.not_found`. The store
still repeats eligibility checks under its write lock, so this early observation
does not authorize a raced replacement. A writer that changes state after the
preflight can still cause a later refusal after input has been read.

`TestImpossibleWriteDoesNotConsumeSelectedInput` covers both refusals on Git and
no-Git stores in JSON and noninteractive human modes. It checks zero stdin reads,
the JSON error code and unchanged store files/modes. A previous-dispatcher source
overlay consumes stdin on the duplicate-add case and fails the regression.
This is specific input-order evidence, not complete per-channel parity proof.

## Native target signals

`TestRunTargetTerminatesByNativeSignal` starts a real Fulla test process, maps
fixture entries into a clean environment, and waits for the executed target to
report its PID after checking that environment. The reported PID must equal the
original Fulla PID. The parent then sends SIGINT or SIGTERM to that owned PID and
checks the operating-system wait status is termination by that exact signal,
not an ordinary exit code translated by a supervisor. No diagnostic output or
store file/mode changes are permitted. Startup and termination are bounded by a
ten-second context; failed tests kill and reap the owned process.

Both cases and the existing PID/exit-23 test pass under race detection. This is
direct native signal evidence on the tested host. Foreground-terminal-generated
signals, ignored dispositions and inherited signal masks remain separate cases;
this test does not claim those through inference.

## Controlling terminal and keyboard interrupt

`scripts/acceptance-run-terminal.py` runs the native binary in a disposable PTY
with stdin attached and detached in separate cases. The executed Python target
checks its mapped value, PID/session/process-group identity, foreground group on
`/dev/tty` and stdout/stderr, expected stdin attachment (or EOF), and enabled
terminal signal processing. After it reports readiness, the driver writes the
terminal interrupt byte to the PTY master. Native wait status must be SIGINT
termination; fixture values must not be printed and store paths/modes/hashes must
remain unchanged. The target explicitly selects SIGINT's default disposition.

Linux/macOS CI runs this gate. The shared driver adds an opt-in attached-stdin
mode while preserving detached stdin by default; the existing full interactive
harness and ordered Git/no-Git human journeys still pass locally. The first
fixture incorrectly equated `/dev/tty`'s device number with the PTY slave; it now
checks foreground groups instead. Ruff, basedpyright and actionlint pass.
Inherited signal masks/ignored dispositions, job suspension/resumption and other
terminal modes are not proved by this Ctrl-C fixture.


## Export eligibility before recovery input

Logical and full-state exports now reject occupied, inside-store or unsafe output
paths before selecting recovery protection. Logical export also reads and parses
its optional manifest before consuming a passphrase descriptor or opening the
terminal prompt. Stdout logical exports bypass file publication checks but still
validate manifest JSON first. The store repeats publication eligibility checks
and retains atomic no-replacement publication; preflight does not grant authority
against a raced destination.

`TestImpossibleExportDoesNotConsumePassphrase` covers occupied logical/full
outputs and malformed file/stdout manifests on Git/no-Git stores, in JSON and
noninteractive human modes where stdout protocol ownership permits them. A real
selected descriptor must remain open at offset zero. The entire fixture tree,
including output/manifest/passphrase files and modes, must remain unchanged.
The old dispatcher/transfer implementation fails this test because it consumes
and closes the descriptor before refusing. The same fixture now covers valid JSON naming missing, duplicate and traversal
selectors. `CheckExportSelection` is shared by CLI preflight and `ExportLogical`,
which repeats it under the shared lock before decryption. Invalid selectors
therefore leave the descriptor untouched too. A successful preflight is advisory:
a cooperating writer may change the inventory before the locked recheck, which
can still refuse after recovery input was supplied.
