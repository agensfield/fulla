"""Render a Fulla formula from checked package bytes; never install or publish."""

import hashlib
import json
import re
import sys
import tarfile
from pathlib import Path
from typing import cast


def render(root: Path, version: str, commit: str) -> str:
    if not re.fullmatch(
        r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[A-Za-z0-9]+([.-][A-Za-z0-9]+)*)?",
        version,
    ):
        raise ValueError("expected a release version")
    if re.search(r"(^|[.-])dev($|[.-])", version) or not re.fullmatch(
        r"[0-9a-f]{40}", commit
    ):
        raise ValueError(
            "development versions or invalid expected commits are unsupported"
        )
    targets = [
        (system, arch) for system in ("darwin", "linux") for arch in ("arm64", "amd64")
    ]
    names = {f"fulla_{version}_{system}_{arch}.tar.gz" for system, arch in targets}
    expected = names | {f"fulla_{version}_source.tar.gz"}
    sums: dict[str, str] = {}
    checksum_file = root / "checksums.txt"
    if (
        checksum_file.is_symlink()
        or not checksum_file.is_file()
        or checksum_file.stat().st_size > 16384
    ):
        raise ValueError("invalid checksum file")
    for line in checksum_file.read_text().splitlines():
        digest, name = line.split("  ")
        if (
            name not in expected
            or name in sums
            or not re.fullmatch(r"[0-9a-f]{64}", digest)
        ):
            raise ValueError("invalid checksum inventory")
        sums[name] = digest
    if set(sums) != expected:
        raise ValueError("incomplete package inventory")
    for name in expected:
        archive_path = root / name
        if (
            archive_path.is_symlink()
            or not archive_path.is_file()
            or archive_path.stat().st_size > 512 << 20
        ):
            raise ValueError("invalid package file")
        with archive_path.open("rb") as stream:
            actual = hashlib.file_digest(stream, "sha256").hexdigest()
        if actual != sums[name]:
            raise ValueError("package checksum mismatch")
        if name not in names:
            continue
        with tarfile.open(archive_path, "r:gz") as archive:
            seen: set[str] = set()
            for member in archive:
                if (
                    member.name
                    not in {
                        "fulla",
                        "LICENSE",
                        "README.md",
                        "THIRD_PARTY_NOTICES",
                        "SOURCE.json",
                    }
                    or member.name in seen
                    or not member.isfile()
                ):
                    raise ValueError("invalid binary archive members")
                seen.add(member.name)
                if member.name == "SOURCE.json":
                    if member.size > 16384:
                        raise ValueError("oversized provenance")
                    source = archive.extractfile(member)
                    if source is None:
                        raise ValueError("missing provenance")
                    decoded = cast(object, json.loads(source.read()))
                    if not isinstance(decoded, dict):
                        raise ValueError("invalid provenance object")
                    provenance = cast(dict[str, object], decoded)
                    if (
                        provenance.get("version") != version
                        or provenance.get("commit") != commit
                    ):
                        raise ValueError(
                            "package does not match expected release identity"
                        )
            if len(seen) != 5:
                raise ValueError("incomplete binary archive")
    lines = [
        "class Fulla < Formula",
        '  desc "Local-first secret custodian for humans and agents"',
        '  homepage "https://github.com/agensfield/fulla"',
        f'  version "{version}"',
        '  license "AGPL-3.0-only"',
        f"  # Reviewed release commit: {commit}",
    ]
    for system in ("darwin", "linux"):
        lines += ["", f"  on_{'macos' if system == 'darwin' else 'linux'} do"]
        for arch in ("arm64", "amd64"):
            name = f"fulla_{version}_{system}_{arch}.tar.gz"
            lines += [
                f"    on_{'arm' if arch == 'arm64' else 'intel'} do",
                f'      url "https://github.com/agensfield/fulla/releases/download/v{version}/{name}"',
                f'      sha256 "{sums[name]}"',
                "    end",
            ]
        lines += ["  end"]
    lines += r"""
  def install
    bin.install "fulla"
    generate_completions_from_executable(bin/"fulla", "completion")
  end

  test do
    ENV["HOME"] = testpath
    assert_equal version.to_s, shell_output("#{bin}/fulla --version").strip
    system bin/"fulla", "--store", testpath/"store", "init", "--no-git", "--yes"
    pipe_output("#{bin}/fulla --store #{testpath}/store add probe --stdin", "fixture\n", 0)
    assert_equal "fixture\n", shell_output("#{bin}/fulla --store #{testpath}/store show probe")
  end
end
""".splitlines()
    lines.append("")
    return "\n".join(lines)


if __name__ == "__main__":
    try:
        if len(sys.argv) != 4:
            raise ValueError(
                "usage: homebrew-formula.py PACKAGES VERSION EXPECTED_COMMIT"
            )
        print(render(Path(sys.argv[1]), sys.argv[2], sys.argv[3]), end="")
    except (OSError, ValueError, tarfile.TarError) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
