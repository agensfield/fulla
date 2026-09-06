# Internal Git repository routing

Ordinary Fulla operations must use the selected store's encrypted repository.
`git -C passwords` alone does not enforce this: Git's documented environment
variables can choose a different repository, index, object directory, common
directory or working tree. Tracing variables can also create files elsewhere.
See the [Git environment reference](https://git-scm.com/docs/git#_environment_variables).

Internal Git now supplies explicit absolute `--git-dir` and `--work-tree` paths.
It removes ambient `GIT_*` settings except the standard config-file selectors
`GIT_CONFIG_GLOBAL`, `GIT_CONFIG_SYSTEM` and `GIT_CONFIG_NOSYSTEM`, then sets
`GIT_OPTIONAL_LOCKS=0`. This includes removing runtime config injection, tracing,
alternate index/object paths, executable routing and repository-format overrides.
Normal HOME/XDG config discovery remains available. The existing explicit hook,
signing, fsmonitor, automatic maintenance and conversion guards still apply.

Before invoking internal Git, Fulla also refuses `.git/commondir` and
`.git/objects/info/alternates`. These are textual routing files, so merely
rejecting symlinks does not make their external storage self-contained. The
refusal preserves both the selected store and external repository.

Regression fixtures exercise initialization and a committed exact-byte write
under ambient repository/work-tree, index, objects, common-directory, runtime
config and trace routing. They verify clean selected-store Git state and the
complete paths, modes and file hashes of an unrelated disposable repository.
Separate cases cover configured `core.worktree` and both textual routing files.
An overlay restoring the previous helper is the negative control.

The maintenance regression's trace instrumentation now lives inside a trusted
fixture Git wrapper, after the production environment boundary. Its positive
automatic-maintenance control and actual Fulla initialization/write checks remain
intact; production does not permit ambient tracing merely to support a test.

This is a routing boundary for ordinary internal operations. Explicit `fulla git`
remains the documented expert escape hatch with user arguments/environment.
Trusted Git executables/configuration and same-Unix-user mutation are not an
isolation boundary. The internal output boundary below is now implemented. Active-operation
deadlines and the broader Git security review remain separate work.

## Internal output bounds

Ordinary internal Git stdout is limited to 4 MiB, matching Fulla's metadata
read budget. Historical ciphertext extraction uses 65 MiB instead, matching
the existing 64 MiB entry limit plus ciphertext overhead. The first excess
write cancels the owned Git command; Fulla waits for it to be reaped and returns
`git.output_limit` with `limit_bytes`, never partial stdout. Buffer content is
bounded; allocator capacity and parser allocations are not claimed to equal
exactly that byte limit. Stderr is discarded instead of retained for diagnostics.

Pipe cleanup is limited to one second after cancellation or child exit, including
inherited descriptors. This is not an elapsed-time deadline for an otherwise
active Git operation, nor termination of arbitrary independent descendants.
Explicit `fulla git` retains its native process/stream behavior. Large history
metadata may require that expert command; Fulla does not silently truncate a
metadata result and present it as complete.

Tests cover exact-limit/over-limit io.Copy, native Git output boundaries, a
producer that ignores pipe errors and remains alive, actual termination/reaping,
no partial output or store changes, and restoration of a historical value larger
than the metadata limit. A negative overlay omitting cancellation makes the
subprocess watchdog fail; it cleans only the fixture's own process group.
