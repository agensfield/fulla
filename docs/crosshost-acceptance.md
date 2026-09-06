# Physical-host partial sync acceptance

`TestCrossHostPartialRetry` in `internal/remote/crosshost_test.go` is an explicit,
opt-in acceptance lane. Ordinary tests skip it. It uses the production OpenSSH
transport and remote CLI server on the selected host, with the production sync
client in the local Go test process. It requires Python 3 and Git on the remote
host, an uploaded Fulla executable built from the reviewed source, and an existing
Fulla-only fixture parent. It does not change SSH configuration or install Fulla.

Set these variables for the single test invocation:

```sh
FULLA_ACCEPTANCE_HOST=devbox \
FULLA_ACCEPTANCE_BINARY=/absolute/fulla-fixture/fulla \
FULLA_ACCEPTANCE_PARENT=/absolute/fulla-fixtures \
GOTOOLCHAIN=go1.26.0 \
go test -race ./internal/remote -run '^TestCrossHostPartialRetry$' -count=1 -v -timeout=8m
```

Use actual approved paths, and independently verify the host identity and uploaded
binary hash before running. Each case creates a new remote temporary directory
under the selected parent and removes only that directory during test cleanup.
Local fixtures use Go's temporary-directory cleanup. The uploaded executable is
operator-owned and must be removed separately after acceptance. An interrupted
harness may leave its temporary directories for inspection; do not remove other
runs' directories. SSH host-key checking remains strict and authentication is
noninteractive. Each SSH command/session has a 45-second fixture deadline.

## What the journey proves

The matrix is Git/no-Git stores against lost `imported`/`exported` replies using
the current protocol. A test-only reader consumes a complete control reply from
the actual remote server and returns EOF before exposing it to the client. It
does not modify production behavior or inject a remote mutation hook. Both
boundaries precede the exported encrypted bundle body. The SSH session is then
cancelled and closed.

Each case requires:

- A successful authenticated mandatory dry-run before real sync.
- A remote push committed before the interruption and no local pull publication.
- `sync.partial`, status 3, and accurate pushed/remote-uncertain evidence in both
  the returned result and durable partial receipt.
- A subsequent ordinary sync that pulls the missing entry and activates the
  peer, without another push or a rewrite of the committed remote ciphertext.
- Exact empty and binary values on both hosts, preservation of divergent shared
  values, and healthy deep verification on both stores.

The fixture seeds reciprocal public pins directly from generated identities;
this is not CLI mutual-enrollment acceptance. Authentication still uses the real
protocol challenge/proof exchange. Existing enrollment journeys remain separate.
This is a deliberate lost-reply acceptance drill between two machines, not a
physical network outage, power-loss test, released-binary rolling upgrade or live
credential cutover. Previous-protocol interruption cases remain covered by the
local protocol matrix rather than this physical-host lane.

## Receipt

The first run uses local macOS/arm64 and the established devbox Linux/x86_64,
with Go 1.26.0. Production source is `26da4ab5206dcae4dfb525f0a08722245c97da3b`;
the worktree additionally contains this test harness. Uploaded Linux executable
SHA-256, verified on both machines:

```text
debdcf0cade63c6db121231eff75ce621c7f255dbfc9c075b73f5c47be2a35b8
```

All four physical-host cases passed (97.59s; race package 98.886s). The
ordinary remote race suite passed (37.731s), followed by full-project vet. The
local `dist/crosshost-partial-retry.log` retains the physical-host test output. No released artifact
or arbitrary-build reproducibility claim follows from this development receipt.
