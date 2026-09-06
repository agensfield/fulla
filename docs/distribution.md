# Distribution

Fulla has no published preview yet. Creating packages is not release acceptance.
The locked release gates remain in [the product spec](product-spec.md).

## Build and inspect packages

Use a clean committed checkout. The source CLI version is also the package
version, so a later `go install` of the same tagged source reports the same value.

```sh
GOTOOLCHAIN=go1.26.0 go run ./scripts/release --output dist/packages-a
GOTOOLCHAIN=go1.26.0 go run ./scripts/release --output dist/packages-b
diff -r dist/packages-a dist/packages-b
GOTOOLCHAIN=go1.26.0 python3 scripts/acceptance-release.py dist/packages-a 0.1.0-dev
```

Update the expected smoke-test version when the source version changes. Each
output directory must be new. The tool refuses dirty/uncommitted source and
never replaces an existing archive. A failed build may leave partial output;
`checksums.txt` appears only after all five archives have been written.

Packages contain the binary, AGPL license, third-party notices, README, and `SOURCE.json` recording
the exact source commit, version, module, and Go toolchain. Binary archives cover
Darwin/Linux and amd64/arm64, with CGO disabled. The source archive contains the
tracked tree of that commit. Binaries build from an isolated extraction of that
archive; bundled documentation is read from that same snapshot. Later
working-tree edits cannot alter the packaged source or documentation.
Tar ownership, modes, timestamps, gzip headers,
build paths, and Go build IDs are normalized for repeatability with the same
source and toolchain.

CI compares two package builds and validates every checksum, archive member,
platform build setting, source provenance, and native executable version. It
extracts only the native binary for execution into a disposable directory.

## Public Go installation acceptance

Verify a reviewed public commit independently of the local checkout:

```sh
FULLA_GO_BINARY="$(GOTOOLCHAIN=go1.26.0 go env GOROOT)/bin/go"
python3 scripts/acceptance-go-install.py "$EXPECTED_COMMIT" 0.1.0-dev "$FULLA_GO_BINARY"
```

Use the expected CLI version for that commit. The harness resolves the public
module at the complete Git ID and checks Go's origin URL/hash. It runs `go install
MODULE@COMMIT` with fresh module/build caches, a temporary HOME/GOPATH/GOBIN,
explicit public proxy/checksum database and disabled Git credential prompting.
It verifies installed build-info module/version/checksum and CLI version, then
runs no-Git initialization, binary stdin add, exact raw/base64 show and deep
doctor on a disposable store. All installation and fixture paths are removed
when the harness exits. The ordinary user binary, Go caches, shell environment
and credentials are not changed.

The native macOS/arm64 run at `658573e982ade72bd269dd837a9def5778bab280`
passed with Go 1.26.0, module version
`v0.0.0-20260906024722-658573e982ad`, CLI `0.1.0-dev` and module checksum
`h1:lkg2ua4dh56o5s+8dqWWvFjTM0t/jAQ4ktSZ5UmiSZ8=`. Its observed binary SHA256 was
`b4dff3a8b91357cd172d07eac66a0601fb75eed130fa68103b5027a1b4a42ee8`.
That hash identifies this installation, not a reproducibility promise across
arbitrary Go environments. This is public development-commit installation
proof, not a tagged release, attestation verification, Homebrew installation or
cross-platform installation acceptance. Run it again against the accepted tag
commit/version as part of release closure. It is deliberately an explicit
network acceptance command rather than a required public-index lookup on every
CI commit.

### Hosted native installation receipts

The manually dispatched `install-acceptance.yml` workflow runs the same public
installation harness on Linux and macOS with fresh installation caches. It
accepts an explicit reviewed full commit ID and expected CLI version, uses only
read repository permissions and pinned setup/checkout/artifact actions, and
retains one JSON receipt per native runner for 14 days. Workflow inputs are
passed through environment variables and quoted arguments, not interpolated
into shell source. Receipts include Go OS/architecture and toolchain identity.

```sh
gh workflow run install-acceptance.yml --ref main \
  -f commit="$EXPECTED_COMMIT" -f version=0.1.0-dev
```

Inspect the run's exact workflow SHA, selected source commit and each platform's
receipt before counting it as evidence. This workflow does not tag, publish or
install into a user's persistent environment. A development commit run does not
satisfy tagged-release or Homebrew acceptance.

Hosted [run 34007934062](https://github.com/agensfield/fulla/actions/runs/34007934062)
passed both native jobs using workflow commit
`605d0aa5c4b2347acbe305858f0a657c7dc8676d` to install reviewed source
`8b5affb0f923dd2b566cc507b4d7685769188320`. Both downloaded receipts were inspected:
Go `go1.26.0`, CLI `0.1.0-dev`, module
`v0.0.0-20260906025744-8b5affb0f923`, checksum
`h1:qH8SM9vi9wwXlCtxNIyXozISUTl19svK75WSTWQpF0Q=` and passing fresh-cache,
installed-binary raw/base64/deep-doctor checks.

| Native platform | Observed installed binary SHA256 |
| --- | --- |
| Linux/amd64 | `b9a3f9c3f39d5e37f4e57ecd8b0489c3cb65236ab470b67027d60a3d92b84a87` |
| macOS/arm64 | `508d51d2ca991cb9e16df58c2268e070d27eedc6cac63ddb7ba082a2b23fffb5` |

This closes native public development-commit installation evidence for those two
platforms. It does not establish Linux/arm64 or macOS/amd64 native installation,
tagged-release installation, release signatures or Homebrew behavior.

## Tag-gated publication

`.github/workflows/release.yml` handles `v*` tag pushes only in
`agensfield/fulla`. Creating the first `v0.1.0` tag remains contingent on the
full-spec acceptance audit; the existence of this workflow is not that audit.

The read-only source gate requires the tag and event object to resolve to the
checked-out commit, that commit to be in `origin/main`, a clean checkout, an
exact source-version/tag match, and tracked nonempty regular release notes at
`docs/releases/VERSION.md`. Development versions are rejected. Both lightweight
and annotated tags are supported. For a prepared release checkout:

```sh
go run ./scripts/release --check-tag v0.1.0 --event-sha "$EVENT_SHA"
```

The workflow calls the existing Linux/macOS CI workflow at the same source
revision. Only after that entire workflow succeeds does the publishing job use
the Linux job's verified four-platform packages, source archive, and checksums.
It does not replace them with an untested rebuild. All invoked actions are pinned
to source SHAs, and write/OIDC/attestation permissions are limited to publication.

The publisher verifies the package set, creates signed provenance for all six
artifacts, verifies the attestation bundle against repository/workflow/source
identity, creates a new draft, downloads its assets, and repeats checksum,
source-provenance, native smoke, exact-byte, and attestation verification before
publishing. The remote tag is rechecked before draft creation and final publication.
All `0.x` and prerelease versions are GitHub prereleases and are not marked latest.
The seventh release asset is `attestation.jsonl`.

Existing releases/assets are never clobbered. A failed upload or verification
can leave an unpublished draft for inspection. A rerun refuses an existing release
rather than silently replacing its assets; inspect that draft and exact source
before deciding how to resume. This workflow does not configure immutable-tag
repository rules, and tag writers remain trusted.

Consumers should verify an artifact using the expected reviewed tag commit,
not a commit value trusted only because it appears inside a downloaded archive:

```sh
gh attestation verify fulla_0.1.0_linux_amd64.tar.gz \
  --repo agensfield/fulla --bundle attestation.jsonl \
  --signer-workflow agensfield/fulla/.github/workflows/release.yml \
  --signer-digest "$EXPECTED_COMMIT" --source-digest "$EXPECTED_COMMIT" \
  --source-ref refs/tags/v0.1.0 --deny-self-hosted-runners
```

The verification interface and identity constraints follow the
[GitHub CLI attestation documentation](https://cli.github.com/manual/gh_attestation_verify).
No SLSA level or independent security review is implied by generating provenance.

## Publication acceptance still open

The source gate has local real-Git coverage and both workflows pass actionlint.
No production release tag, hosted publication, attestation, or downloaded public
release has been exercised yet. Those are required delivery receipts, along with
the remaining full-spec acceptance, tagged `go install`, Fulla Homebrew formula,
and installed-binary/`brew test` acceptance. The intended user surface is
`brew install agensfield/tap/fulla`; it is not available merely because the
workflow and packaging code exist. No tag is created by either packaging or
source-verification commands.

## Homebrew formula generation

After package acceptance and release provenance verification, render the scoped
formula with an independently reviewed release version and commit:

```sh
python3 scripts/homebrew-formula.py dist/release 0.1.0 "$EXPECTED_COMMIT" > dist/fulla.rb
ruby -c dist/fulla.rb
```

The generator rejects development versions, malformed identities, incomplete or
duplicate checksum inventories, mismatched package hashes, symlinked package
files, invalid binary archive member sets and binary SOURCE.json records that
differ from the supplied release identity. It hashes all five archives and checks
provenance in each of the four binary archives. It reads packages without
executing binaries, extracting files, installing software or changing the tap.
The rendered formula uses four explicit OS/CPU URLs and SHA-256 values, installs
Bash/Zsh/Fish completions, and contains an isolated no-Git init/write/exact-read
Homebrew test. Only stdout contains the formula; failure emits no formula text.

This is additional preparation, not signature verification or complete artifact
acceptance. In particular, hashing the source archive does not inspect its tree,
and matching SOURCE.json does not cryptographically prove binary provenance.
Continue to run the existing package/attestation gates. Then publish only Fulla's
formula in the Agensfield tap and verify actual install, completions and
`brew test` on the intended platforms. Those installation/publication gates are
still open; synthetic fixture formula tests do not close them.

`scripts/acceptance-formula.py` exercises valid, mismatched checksum, missing
archive checksum, wrong commit/version, development version, injection, nonobject
provenance and symlink cases, then runs Ruby syntax validation. Linux/macOS CI runs
this fixture gate. It deliberately uses synthetic archives, not installable
release artifacts. Formula layout follows the [Homebrew cookbook](https://docs.brew.sh/Formula-Cookbook)
and [completion API](https://docs.brew.sh/rubydoc/Formula.html#generate_completions_from_executable-instance_method).

## Hosted Homebrew installation drill

`.github/workflows/homebrew-acceptance.yml` is an explicit macOS-only dispatch
lane. It runs `scripts/acceptance-homebrew.py` only on a GitHub-hosted runner for
this repository. The script refuses to run on an ordinary local workstation.

It creates a local, unpublished fixture commit changing only the CLI version to
`0.1.0-brewtest`, builds the four-platform package inventory, and runs the ordinary
formula generator against that fixture commit. Only the formula's package URLs
are rewritten to the runner's local files. The installation and test methods are
unchanged. An ephemeral `fulla-fixture/acceptance` tap then exercises actual
`brew install`, installed version, Bash/Zsh/Fish completion content (ignoring outer whitespace), and
`brew test`. The script checks that Fulla/the tap were absent beforehand, disables
Homebrew automatic updates/cleanup/analytics, and removes its formula and tap.

```sh
gh workflow run homebrew-acceptance.yml --repo agensfield/fulla --ref main
```

A successful run retains a JSON receipt for 14 days: base and fixture commits,
fixture version, platform, Homebrew version, binary/formula/completion SHA-256
values, test success and cleanup. It is installation-method acceptance on the
recorded native runner, not public release download, attestation, real Agensfield
tap installation, other-architecture acceptance or user-store adoption. The
release version and public tag stay unchanged. Failure output retains the
underlying command diagnostics for root-cause investigation.

[Run 34009919496](https://github.com/agensfield/fulla/actions/runs/34009919496)
passed on macOS/arm64 using Homebrew 6.0.13. Its downloaded receipt confirms
installation, formula test and cleanup. Base source is ad5be4ba5b329608b70e690823b2fa04aff74481;
local fixture commit is ced0e6831de7e22071a029095df1202da86dc1a0. The latter is
unpublished and only changes the fixture CLI version to 0.1.0-brewtest.

| Installed artifact | SHA-256 |
| --- | --- |
| Binary | `8b911aef3618cda8300379314569f332f2b086cff356ff566a6299a3a2dc2849` |
| Fixture formula | `67572d92e825ed302568eabf37e327391dfb2d5f161b6230c5f1c5bd60239f08` |
| Bash completion | `5c989723e1b22d97db6d00a7620ec3b6314a68072456dd921bdcf3be3be9bcc9` |
| Zsh completion | `0ed8aca655a482900b8d0bde1e9fc2e29fed2a836bdc2fa506c572d0f1825105` |
| Fish completion | `4d69c10b5e7444c0959c9854e409e2a78c6b6be8e42e074adfe534e59742f39a` |

The local downloaded JSON is `dist/homebrew-34009919496/homebrew-receipt.json`.
It explicitly records `release_acceptance: false`. Public tap/release URL
installation, other native platforms, and final tagged artifact acceptance remain
open; this fixture run does not advance the release version.
