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
