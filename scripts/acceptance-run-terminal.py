"""Native run terminal inheritance and keyboard signal acceptance."""

import hashlib
import json
import os
import signal
import subprocess
import sys
import tempfile
from pathlib import Path

from acceptance_terminal import terminal

binary = str(Path(sys.argv[1]).resolve())
_ = os.umask(0o077)
with tempfile.TemporaryDirectory(prefix="fulla-run-terminal-") as temporary:
    home = Path(temporary).resolve()
    store = home / "store"
    env = {
        "PATH": os.environ["PATH"],
        "HOME": str(home),
        "XDG_CONFIG_HOME": str(home / "config"),
    }
    base = [binary, "--store", str(store), "--json"]
    _ = subprocess.run(
        base + ["init", "--yes", "--no-git"],
        env=env,
        capture_output=True,
        check=True,
        timeout=20,
    )
    value = b"synthetic-terminal-run-value"
    _ = subprocess.run(
        base + ["add", "token", "--stdin"],
        input=value,
        env=env,
        capture_output=True,
        check=True,
        timeout=20,
    )

    def inventory() -> dict[str, tuple[int, str]]:
        return {
            str(p.relative_to(store)): (
                p.stat().st_mode,
                hashlib.sha256(p.read_bytes()).hexdigest() if p.is_file() else "",
            )
            for p in [store, *store.rglob("*")]
        }

    before = inventory()
    target = """import os, signal, sys, termios
assert os.environ['TOKEN'] == 'synthetic-terminal-run-value'
tty = os.open('/dev/tty', os.O_RDWR)
assert os.getpid() == os.getsid(0) == os.getpgrp() == os.tcgetpgrp(tty)
assert os.isatty(1) and os.isatty(2)
assert os.fstat(1).st_rdev == os.fstat(2).st_rdev
assert os.tcgetpgrp(1) == os.tcgetpgrp(2) == os.tcgetpgrp(tty)
assert os.isatty(0) == (sys.argv[1] == 'tty')
if sys.argv[1] != 'tty':
    assert os.read(0, 1) == b''
attributes = termios.tcgetattr(tty)
assert attributes[3] & termios.ISIG
assert attributes[6][termios.VINTR] == b'\\x03'
signal.signal(signal.SIGINT, signal.SIG_DFL)
print('native-terminal-ready', flush=True)
signal.pause()
raise RuntimeError('expected terminal SIGINT termination')
"""
    for attached in (False, True):
        code, output = terminal(
            binary,
            env,
            [
                "run",
                "--clean-env",
                "--env",
                "TOKEN=token",
                "--",
                sys.executable,
                "-c",
                target,
                "tty" if attached else "detached",
            ],
            [(b"native-terminal-ready", b"\x03")],
            store,
            stdin_tty=attached,
        )
        assert code == -signal.SIGINT, f"terminal signal status={code}: {output!r}"
        assert value not in output and b"Traceback" not in output
        assert inventory() == before, "native terminal execution changed the store"
print(
    json.dumps(
        {
            "native_terminal": True,
            "foreground_ctrl_c": True,
            "stdin_attached_and_detached": True,
            "store_unchanged": True,
        }
    )
)
