# Crash diagnostics

Ordinary builds, including `go build`, `go install`, and packaged binaries,
redact unexpected panics caught by the CLI invocation boundary. Human mode
writes `internal.failure: unexpected internal failure` to stderr; machine mode
writes one `fulla.cli/v1` failure envelope to stdout. The status is 1. Neither
path formats the panic value, prints its stack, or writes a crash report.

For development with synthetic fixtures, explicitly opt into Go panic output:

```sh
go build -tags fulla_debug -o dist/fulla-debug .
go test -tags fulla_debug ./internal/cli -run '^TestPanicProcess'
```

The debug build re-panics with the original value, retaining the original stack
in Go's diagnostics. This is a compile-time choice, not an environment switch
in an installed release. Do not use this build with real secrets: the value and
stack can disclose private data. The package builder clears `GOFLAGS` for its
binary builds and supplies no debug tag.

`panic_test.go` runs isolated subprocesses with synthetic string/error panic
values, in human and JSON modes, for each build policy. Default-build checks
require the exact redacted result, status 1, no stack/value/path disclosure, and
no filesystem artifacts in the private working/home/temp directory. Debug
checks require the synthetic value and original CLI stack. Core dumps are
disabled for these deliberate test crashes. Both policies run on Linux/macOS CI.

This proves the CLI boundary using a compiled test executable. It does not yet
prove deliberate fault injection into each final published executable. Go
recovery is goroutine-local: panics in independently started goroutines, runtime
fatal errors, OS core-dump configuration, and failures while writing the error
itself require separate review. No claim of universal crash containment follows
from these tests. The explicit private, redacted doctor report remains separate
from panic stacks; ordinary invocations do not create an automatic crash file.
