"""Verify public release archives and smoke-test only the native executable."""

import hashlib
import json
import os
import platform
import subprocess
import sys
import tarfile
import tempfile
from pathlib import Path
from typing import cast

root = Path(sys.argv[1]).resolve()
version = sys.argv[2]
commit = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
expected = {
    f"fulla_{version}_{target_os}_{arch}.tar.gz"
    for target_os in ("darwin", "linux")
    for arch in ("amd64", "arm64")
}
expected.add(f"fulla_{version}_source.tar.gz")
checksums: dict[str, str] = {}
for line in (root / "checksums.txt").read_text().splitlines():
    checksum, name = line.split("  ")
    assert name in expected and name not in checksums
    checksums[name] = checksum
assert set(checksums) == expected
assert {item.name for item in root.iterdir()} == expected | {"checksums.txt"}
for name, checksum in checksums.items():
    assert hashlib.sha256((root / name).read_bytes()).hexdigest() == checksum
native_os = {"Darwin": "darwin", "Linux": "linux"}[platform.system()]
native_arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64"}[
    platform.machine()
]
source_documents: dict[str, bytes] = {}
with tarfile.open(root / f"fulla_{version}_source.tar.gz", "r:gz") as source:
    for document in ("LICENSE", "README.md", "THIRD_PARTY_NOTICES"):
        member = source.extractfile(f"fulla-{version}/{document}")
        assert member is not None
        source_documents[document] = member.read()
with tempfile.TemporaryDirectory(prefix="fulla-release-smoke-") as temporary:
    for target_os in ("darwin", "linux"):
        for arch in ("amd64", "arm64"):
            name = f"fulla_{version}_{target_os}_{arch}.tar.gz"
            with tarfile.open(root / name, "r:gz") as archive:
                members = archive.getmembers()
                assert {m.name for m in members} == {
                    "fulla",
                    "LICENSE",
                    "README.md",
                    "SOURCE.json",
                    "THIRD_PARTY_NOTICES",
                }
                assert len(members) == 5 and all(m.isfile() for m in members)
                for m in members:
                    assert m.uid == 0 and m.gid == 0 and m.mtime == 0
                    assert m.mode == (0o755 if m.name == "fulla" else 0o644)
                for document, expected_bytes in source_documents.items():
                    member = archive.extractfile(document)
                    assert member is not None and member.read() == expected_bytes
                source_file = archive.extractfile("SOURCE.json")
                assert source_file is not None
                provenance = cast(dict[str, str], json.loads(source_file.read()))
                assert (
                    provenance["version"] == version and provenance["commit"] == commit
                )
                binary_file = archive.extractfile("fulla")
                assert binary_file is not None
                executable = Path(temporary) / f"fulla-{target_os}-{arch}"
                _ = executable.write_bytes(binary_file.read())
                executable.chmod(0o700)
                build = subprocess.check_output(
                    ["go", "version", "-m", str(executable)], text=True
                )
                assert f"GOOS={target_os}" in build and f"GOARCH={arch}" in build
                assert "CGO_ENABLED=0" in build
                if (target_os, arch) == (native_os, native_arch):
                    env = {"PATH": os.environ["PATH"], "HOME": temporary}
                    actual = subprocess.check_output(
                        [str(executable), "--version"], env=env, text=True
                    )
                    assert actual.strip() == version
                    machine = cast(
                        dict[str, object],
                        json.loads(
                            subprocess.check_output(
                                [str(executable), "version", "--json"], env=env
                            )
                        ),
                    )
                    assert machine["schema"] == "fulla.cli/v1"
                    assert (Path(temporary) / ".local/share/fulla").exists() is False
    with tarfile.open(root / f"fulla_{version}_source.tar.gz", "r:gz") as source:
        names = source.getnames()
        assert f"fulla-{version}/go.mod" in names
        assert f"fulla-{version}/LICENSE" in names
        assert f"fulla-{version}/internal/cli/cli.go" in names
print(
    json.dumps(
        {
            "checksums": True,
            "four_platform_archives": True,
            "native_version": version,
            "source_commit": commit,
        }
    )
)
