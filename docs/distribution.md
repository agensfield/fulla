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
