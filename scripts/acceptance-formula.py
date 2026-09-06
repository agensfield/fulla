"""Exercise formula generation with public synthetic package fixtures."""

import hashlib
import io
import json
import subprocess
import sys
import tarfile
import tempfile
from pathlib import Path

generator = Path(__file__).with_name("homebrew-formula.py").resolve()
version = "0.1.0"
commit = "1" * 40
cases = (
    "valid",
    "checksum",
    "missing",
    "commit",
    "version",
    "development",
    "injection",
    "nonobject",
    "symlink",
)
for case in cases:
    with tempfile.TemporaryDirectory(prefix="fulla-formula-") as temporary:
        root = Path(temporary)
        packages = root / "packages"
        packages.mkdir()
        sums: dict[str, str] = {}
        for system in ("darwin", "linux"):
            for arch in ("arm64", "amd64"):
                name = f"fulla_{version}_{system}_{arch}.tar.gz"
                provenance: object = {
                    "version": version,
                    "commit": "2" * 40 if case == "commit" else commit,
                }
                if case == "nonobject":
                    provenance = []
                with tarfile.open(packages / name, "w:gz") as archive:
                    for member in (
                        "fulla",
                        "LICENSE",
                        "README.md",
                        "THIRD_PARTY_NOTICES",
                        "SOURCE.json",
                    ):
                        data = (
                            json.dumps(provenance).encode()
                            if member == "SOURCE.json"
                            else b"synthetic fixture"
                        )
                        info = tarfile.TarInfo(member)
                        info.size = len(data)
                        archive.addfile(info, io.BytesIO(data))
                sums[name] = hashlib.sha256((packages / name).read_bytes()).hexdigest()
        source = f"fulla_{version}_source.tar.gz"
        _ = (packages / source).write_bytes(b"synthetic source fixture")
        sums[source] = hashlib.sha256((packages / source).read_bytes()).hexdigest()
        first = next(iter(sums))
        if case == "checksum":
            sums[first] = "0" * 64
        if case == "missing":
            del sums[first]
        if case == "symlink":
            original = packages / first
            moved = root / "outside.tar.gz"
            _ = original.rename(moved)
            original.symlink_to(moved)
        _ = (packages / "checksums.txt").write_text(
            "".join(f"{digest}  {name}\n" for name, digest in sorted(sums.items()))
        )
        selected = {
            "version": "0.2.0",
            "development": "0.1.0-dev",
            "injection": '0.1.0";puts 1',
        }.get(case, version)
        result = subprocess.run(
            [sys.executable, str(generator), str(packages), selected, commit],
            capture_output=True,
            check=False,
            timeout=20,
        )
        if case != "valid":
            assert result.returncode == 1 and not result.stdout, case
            assert b"Traceback" not in result.stderr, result.stderr
            continue
        assert result.returncode == 0 and not result.stderr, result.stderr
        formula = result.stdout.decode()
        assert formula.count("      url ") == 4 and formula.count("      sha256 ") == 4
        assert commit in formula and 'license "AGPL-3.0-only"' in formula
        for name, digest in sums.items():
            if name != source:
                assert f"/v{version}/{name}" in formula and digest in formula
        assert '"fixture\\n"' in formula and '"--no-git"' in formula
        output = root / "fulla.rb"
        _ = output.write_bytes(result.stdout)
        _ = subprocess.run(
            ["ruby", "-c", str(output)], capture_output=True, check=True, timeout=20
        )
print(
    json.dumps(
        {
            "formula_cases": list(cases),
            "ruby_syntax": True,
            "installation_test": "generated, not executed",
        }
    )
)
