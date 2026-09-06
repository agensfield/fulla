"""Exercise Fulla's real copy/worker processes against isolated clipboard tools."""

import json
import os
import subprocess
import sys
import tempfile
import time
from pathlib import Path
from typing import cast

binary = str(Path(sys.argv[1]).resolve())
real = "--real-clipboard" in sys.argv[2:]
wayland = "--wayland" in sys.argv[2:]
if wayland and (
    not real or sys.platform != "linux" or not os.environ.get("WAYLAND_DISPLAY")
):
    raise RuntimeError("Wayland acceptance requires an explicit real Linux compositor")
if real and os.environ.get("GITHUB_ACTIONS") != "true":
    raise RuntimeError(
        "real clipboard acceptance is restricted to disposable GitHub runners"
    )
with tempfile.TemporaryDirectory(prefix="fulla-clipboard-") as temporary:
    home = Path(temporary).resolve()
    state = home / "clipboard"
    bindir = home / "bin"
    bindir.mkdir()
    helper = f"""#!{sys.executable}
import os, pathlib, sys
assert 'FULLA_TEST_AMBIENT_SECRET' not in os.environ
state = pathlib.Path({str(state)!r})
if pathlib.Path(sys.argv[0]).name in ('pbcopy', 'wl-copy'):
    data = sys.stdin.buffer.read()
    state.write_bytes(data)
    if state.with_name('fail-write').exists():
        print('synthetic-copy-secret', file=sys.stderr)
        sys.exit(23)
else:
    if state.with_name('fail-read').exists():
        print('synthetic-copy-secret', file=sys.stderr)
        sys.exit(23)
    sys.stdout.buffer.write(state.read_bytes())
"""
    for name in ["pbcopy", "pbpaste", "wl-copy", "wl-paste"]:
        tool = bindir / name
        _ = tool.write_text(helper)
        tool.chmod(0o700)
    env = {
        **os.environ,
        "HOME": str(home),
        "PATH": (str(bindir) + os.pathsep if not real else "") + os.environ["PATH"],
        "XDG_CONFIG_HOME": str(home / "config"),
        "FULLA_CONFIG": "",
        "FULLA_TEST_AMBIENT_SECRET": "synthetic-ambient-secret",
        "WAYLAND_DISPLAY": (os.environ["WAYLAND_DISPLAY"] if wayland else "")
        if real
        else "fixture-wayland",
    }
    base = [binary, "--store", str(home / "store")]

    def read_clipboard() -> bytes:
        if not real:
            return state.read_bytes()
        args = (
            ["pbpaste", "-Prefer", "txt"]
            if sys.platform == "darwin"
            else ["wl-paste", "--no-newline", "--type", "text/plain;charset=utf-8"]
            if wayland
            else ["xclip", "-selection", "clipboard", "-out", "-target", "UTF8_STRING"]
        )
        return subprocess.run(
            args,
            env={**env, "LC_ALL": "en_US.UTF-8"},
            capture_output=True,
            check=True,
            timeout=10,
        ).stdout

    def replace_clipboard(value: bytes) -> None:
        if not real:
            _ = state.write_bytes(value)
            return
        args = (
            ["pbcopy"]
            if sys.platform == "darwin"
            else ["wl-copy", "--type", "text/plain;charset=utf-8"]
            if wayland
            else ["xclip", "-selection", "clipboard", "-in", "-target", "UTF8_STRING"]
        )
        _ = subprocess.run(
            args,
            input=value,
            env=env,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            check=True,
            timeout=10,
        )

    def cli(
        *args: str, data: bytes | None = None, expected: int = 0
    ) -> dict[str, object]:
        result = subprocess.run(
            base + list(args),
            input=data,
            env=env,
            capture_output=True,
            check=False,
            timeout=30,
        )
        assert result.returncode == expected, f"unexpected status {result.returncode}"
        output = cast(dict[str, object], json.loads(result.stdout))
        assert b"synthetic-copy-secret" not in result.stdout + result.stderr
        assert output["schema"] == "fulla.cli/v1"
        return output

    _ = cli("init", "--yes", "--json")
    value = "synthetic-copy-secret秘密\n\n".encode()
    _ = cli("add", "text", "--stdin", "--json", data=value)
    output = cli("clip", "text", "--no-clear", "--json")
    assert output["command"] == "copy" and output["warnings"]
    if wayland:
        assert cast(dict[str, object], output["data"])["backend"] == "wayland"
    assert read_clipboard() == value
    _ = cli("copy", "text", "--clear-after", "200ms", "--json")
    deadline = time.monotonic() + 5
    while read_clipboard() and time.monotonic() < deadline:
        time.sleep(0.02)
    assert read_clipboard() == b"", "expiry did not clear matching clipboard"

    _ = cli("copy", "text", "--clear-after", "200ms", "--json")
    replace_clipboard(b"newer clipboard")
    time.sleep(0.6)
    assert read_clipboard() == b"newer clipboard", "expiry erased a replacement"
    _ = cli("add", "binary", "--stdin", "--json", data=b"\x00\xff")
    _ = cli("copy", "binary", "--no-clear", "--json", expected=1)
    assert read_clipboard() == b"newer clipboard", "unsupported input changed clipboard"
    _ = cli("copy", "text", "--no-clear", "--clear-after", "1s", "--json", expected=2)
    assert read_clipboard() == b"newer clipboard"
    for marker in [] if real else ["fail-write", "fail-read"]:
        (home / marker).touch()
        failed = cli("copy", "text", "--no-clear", "--json", expected=3)
        assert not failed["ok"] and read_clipboard() == value
        (home / marker).unlink()
print(
    json.dumps(
        {
            "backend": "wayland" if wayland else "platform" if real else "fixture",
            "exact_text": True,
            "expiry": True,
            "replacement_preserved": True,
            "binary_rejected": True,
            "ambient_environment_filtered": True,
        }
    )
)
