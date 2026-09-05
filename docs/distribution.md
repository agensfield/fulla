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

Packages contain the binary, AGPL license, README, and `SOURCE.json` recording
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

## Publication work still open

The established Scriba lane informs this layout. Fulla still needs accepted
tagged source, gated release publication, artifact provenance/signature
verification, the Fulla formula in the Agensfield tap, and installed-binary/
`brew test` acceptance. The intended user surface is
`brew install agensfield/tap/fulla`; it is not available merely because these
packaging scripts exist. No version tag is created by the packaging command.
