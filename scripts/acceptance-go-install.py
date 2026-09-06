"""Install public Fulla at an exact commit into a disposable Go environment."""

import base64
import hashlib
import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import cast

if len(sys.argv) != 4:
    raise SystemExit("usage: acceptance-go-install.py COMMIT CLI_VERSION GO_BINARY")
commit, version, go_argument = sys.argv[1:]
go = str(Path(go_argument).resolve(strict=True))
if re.fullmatch(r"[0-9a-f]{40}", commit) is None:
    raise SystemExit("commit must be a complete lowercase Git object ID")
module = "github.com/agensfield/fulla"


def object_json(data: bytes) -> dict[str, object]:
    value = cast(object, json.loads(data))
    assert isinstance(value, dict), "expected JSON object"
    return cast(dict[str, object], value)


_ = os.umask(0o077)
with tempfile.TemporaryDirectory(prefix="fulla-go-install-") as temporary:
    home = Path(temporary).resolve()
    env = {
        "PATH": "/usr/bin:/bin:/usr/sbin:/sbin",
        "HOME": str(home),
        "TMPDIR": str(home),
        "XDG_CONFIG_HOME": str(home / "config"),
        "GOBIN": str(home / "bin"),
        "GOPATH": str(home / "go"),
        "GOMODCACHE": str(home / "modules"),
        "GOCACHE": str(home / "cache"),
        "GOTOOLCHAIN": "local",
        "GOPROXY": "https://proxy.golang.org,direct",
        "GOSUMDB": "sum.golang.org",
        "GIT_TERMINAL_PROMPT": "0",
        "GIT_CONFIG_NOSYSTEM": "1",
        "GIT_CONFIG_GLOBAL": os.devnull,
        "CGO_ENABLED": "0",
    }

    def run(command: list[str], data: bytes | None = None) -> bytes:
        result = subprocess.run(
            command,
            input=data,
            stdin=subprocess.DEVNULL if data is None else None,
            capture_output=True,
            env=env,
            cwd=home,
            timeout=300,
            check=False,
        )
        if result.returncode != 0:
            raise RuntimeError(
                f"{Path(command[0]).name} exited {result.returncode}: "
                + result.stderr.decode(errors="replace")
            )
        return result.stdout

    metadata = object_json(run([go, "list", "-m", "-json", f"{module}@{commit}"]))
    origin = metadata["Origin"]
    assert isinstance(origin, dict), "missing source origin"
    origin = cast(dict[str, object], origin)
    assert metadata["Path"] == module and origin["Hash"] == commit
    assert origin["VCS"] == "git" and origin["URL"] == f"https://{module}"
    module_version = metadata["Version"]
    assert isinstance(module_version, str)
    _ = run([go, "install", f"{module}@{commit}"])
    binary = home / "bin" / "fulla"
    build_info = run([go, "version", "-m", str(binary)]).decode()
    module_lines = [line.split() for line in build_info.splitlines()]
    matches = [line for line in module_lines if line[:1] == ["mod"]]
    assert len(matches) == 1 and matches[0][1:3] == [module, module_version]
    assert len(matches[0]) == 4 and matches[0][3].startswith("h1:")
    assert run([str(binary), "--version"]).decode().strip() == version
    store = str(home / "store")

    def fulla(tail: list[str], data: bytes | None = None) -> bytes:
        return run([str(binary), "--store", store, *tail], data)

    def success(tail: list[str], data: bytes | None = None) -> dict[str, object]:
        result = object_json(fulla(["--json", *tail], data))
        assert result["schema"] == "fulla.cli/v1" and result["ok"] is True
        return result

    _ = success(["init", "--no-git", "--yes"])
    fixture = b"installed Fulla fixture\x00\xff\n"
    _ = success(["add", "probe", "--stdin"], fixture)
    assert fulla(["show", "probe"]) == fixture
    encoded = success(["show", "probe"])["data"]
    assert isinstance(encoded, dict)
    encoded = cast(dict[str, object], encoded)
    assert (
        encoded["encoding"] == "base64"
        and encoded["value"] == base64.b64encode(fixture).decode()
    )
    _ = success(["doctor", "--deep"])
    with binary.open("rb") as executable:
        binary_hash = hashlib.file_digest(executable, "sha256").hexdigest()
    toolchain = object_json(run([go, "env", "-json", "GOVERSION", "GOOS", "GOARCH"]))
    print(
        json.dumps(
            {
                "source_commit": commit,
                "module": module,
                "module_version": module_version,
                "module_sum": matches[0][3],
                "cli_version": version,
                "go_version": toolchain["GOVERSION"],
                "goos": toolchain["GOOS"],
                "goarch": toolchain["GOARCH"],
                "binary_sha256": binary_hash,
                "fresh_module_and_build_caches": True,
                "no_git_init_add_raw_json_deep_doctor": True,
                "release_acceptance": False,
            }
        )
    )
