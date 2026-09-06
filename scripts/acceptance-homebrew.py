"""Install a generated Fulla fixture formula on a disposable GitHub macOS runner."""

import hashlib
import json
import os
import platform
import re
import subprocess
import sys
from pathlib import Path

if (
    os.environ.get("GITHUB_ACTIONS") != "true"
    or os.environ.get("RUNNER_ENVIRONMENT") != "github-hosted"
    or os.environ.get("GITHUB_REPOSITORY") != "agensfield/fulla"
    or platform.system() != "Darwin"
):
    raise SystemExit("requires the explicit hosted macOS Fulla acceptance workflow")

os.environ.update(
    HOMEBREW_NO_AUTO_UPDATE="1",
    HOMEBREW_NO_INSTALL_CLEANUP="1",
    HOMEBREW_NO_ANALYTICS="1",
)


def run(*args: str) -> str:
    result = subprocess.run(args, check=False, capture_output=True, text=True, timeout=600)
    if result.returncode != 0:
        print(result.stdout, file=sys.stderr)
        print(result.stderr, file=sys.stderr)
        raise RuntimeError(f"{args[0]} failed with status {result.returncode}")
    return result.stdout.strip()


base = run("git", "rev-parse", "HEAD")
assert base == os.environ["GITHUB_SHA"]
assert not run("git", "status", "--porcelain", "--untracked-files=normal")
assert "fulla" not in run("brew", "list", "--formula").splitlines()
tap = "fulla-fixture/acceptance"
qualified = tap + "/fulla"
assert tap not in run("brew", "tap").splitlines()
version = "0.1.0-brewtest"
source = Path("internal/cli/cli.go")
updated, count = re.subn(
    r'var Version = "[^"]+"', f'var Version = "{version}"', source.read_text()
)
assert count == 1
_ = source.write_text(updated)
_ = run("git", "add", str(source))
_ = run(
    "git",
    "-c",
    "user.name=Fulla acceptance",
    "-c",
    "user.email=fixture@localhost",
    "commit",
    "-m",
    "test: set disposable Homebrew fixture version",
)
fixture_commit = run("git", "rev-parse", "HEAD")
packages = Path("dist/brew-packages").resolve()
_ = run("go", "run", "./scripts/release", "--output", str(packages))
formula = run("python3", "scripts/homebrew-formula.py", str(packages), version, fixture_commit)
for system in ("darwin", "linux"):
    for arch in ("arm64", "amd64"):
        name = f"fulla_{version}_{system}_{arch}.tar.gz"
        remote = f"https://github.com/agensfield/fulla/releases/download/v{version}/{name}"
        assert formula.count(remote) == 1
        formula = formula.replace(remote, (packages / name).as_uri())

_ = run("brew", "tap-new", "--no-git", tap)
receipt: dict[str, object] = {}
try:
    tap_path = Path(run("brew", "--repository", tap))
    formula_path = tap_path / "Formula/fulla.rb"
    _ = formula_path.write_text(formula + "\n")
    print("installing generated fixture formula", flush=True)
    _ = run("brew", "install", "--formula", qualified)
    prefix = Path(run("brew", "--prefix", qualified))
    executable = prefix / "bin/fulla"
    assert run(str(executable), "--version") == version
    completions = {
        "bash": prefix / "etc/bash_completion.d/fulla",
        "zsh": prefix / "share/zsh/site-functions/_fulla",
        "fish": prefix / "share/fish/vendor_completions.d/fulla.fish",
    }
    hashes: dict[str, str] = {}
    for shell, completion in completions.items():
        data = completion.read_bytes()
        assert data.strip() == run(str(executable), "completion", shell).encode()
        hashes[shell] = hashlib.sha256(data).hexdigest()
    print("running installed formula test", flush=True)
    _ = run("brew", "test", "--verbose", qualified)
    receipt = {
        "base_commit": base,
        "fixture_commit": fixture_commit,
        "fixture_version": version,
        "platform": platform.machine(),
        "brew": run("brew", "--version").splitlines()[0],
        "binary_sha256": hashlib.sha256(executable.read_bytes()).hexdigest(),
        "formula_sha256": hashlib.sha256(formula_path.read_bytes()).hexdigest(),
        "completion_sha256": hashes,
        "installed": True,
        "brew_test": True,
        "release_acceptance": False,
    }
finally:
    if "fulla" in run("brew", "list", "--formula").splitlines():
        _ = run("brew", "uninstall", "--force", qualified)
    _ = run("brew", "untap", tap)
assert "fulla" not in run("brew", "list", "--formula").splitlines()
assert tap not in run("brew", "tap").splitlines()
receipt["cleanup"] = True
_ = Path("dist/homebrew-receipt.json").write_text(json.dumps(receipt, indent=2) + "\n")
print(json.dumps(receipt), flush=True)
