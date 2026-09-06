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
isolation boundary. Subprocess time/output bounds and the broader Git security
review remain separate work.
