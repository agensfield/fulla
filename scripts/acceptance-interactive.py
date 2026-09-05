#!/usr/bin/env python3
"""Isolated controlling-terminal acceptance. Only generated fixture data is used."""
import errno
import json
import os
from pathlib import Path
import pty
import select
import signal
import subprocess
import sys
import tempfile
import termios
import time

binary = str(Path(sys.argv[1]).resolve())
with tempfile.TemporaryDirectory(prefix="fulla-tty-") as temporary:
    home = Path(temporary).resolve()
    store = home / "store"
    env = {**os.environ, "HOME": str(home), "TMPDIR": str(home),
           "XDG_CONFIG_HOME": str(home / "config"), "FULLA_CONFIG": "",
           "FULLA_DIR": "", "PA_DIR": ""}
    base = [binary, "--store", str(store)]

    def cli(*args, data=None):
        return subprocess.run(base + list(args), input=data, env=env,
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True).stdout

    def terminal(args, actions):
        pid, fd = pty.fork()
        if pid == 0:
            # Prompts and hidden input must use /dev/tty, not standard input.
            null = os.open(os.devnull, os.O_RDONLY)
            os.dup2(null, 0)
            os.close(null)
            os.execve(binary, base + args, env)
        output = bytearray()
        pending = list(actions)
        deadline = time.monotonic() + 15
        result = None
        try:
            while time.monotonic() < deadline:
                if pending and pending[0][0] in output:
                    _, response = pending.pop(0)
                    if isinstance(response, int):
                        os.kill(pid, response)
                    else:
                        os.write(fd, response)
                ready, _, _ = select.select([fd], [], [], 0.05)
                if ready:
                    try:
                        chunk = os.read(fd, 65536)
                    except OSError as error:
                        if error.errno != errno.EIO:
                            raise
                        chunk = b""
                    output.extend(chunk)
                waited, status = os.waitpid(pid, os.WNOHANG)
                if waited:
                    result = os.waitstatus_to_exitcode(status)
                    break
            assert result is not None, "terminal process timed out"
            assert not pending, f"terminal prompt was missing: exit={result}, output={bytes(output)!r}"
            restored = termios.tcgetattr(fd)[3]
            assert restored & termios.ECHO and restored & termios.ICANON, "terminal state was not restored"
            return result, bytes(output)
        finally:
            if result is None:
                os.kill(pid, signal.SIGKILL)
                os.waitpid(pid, 0)
            os.close(fd)

    cli("init", "--yes", "--json")
    code, output = terminal(["add", "typed"], [
        (b"[e]ditor:", b"t\n"),
        (b"no newline is added):", b"synthetic-hidden-value\n"),
    ])
    assert code == 0 and b"synthetic-hidden-value" not in output
    assert cli("show", "typed") == b"synthetic-hidden-value"

    code, output = terminal(["add", "generated"], [(b"[e]ditor:", b"g\n")])
    assert code == 0 and len(cli("show", "generated")) == 32

    editor = home / "editor.py"
    receipt = home / "editor-receipt"
    editor.write_text("""import os, pathlib, sys, time, tty
file = pathlib.Path(sys.argv[1])
assert file.stat().st_mode & 0o777 == 0o600
assert file.parent.stat().st_mode & 0o777 == 0o700
assert file.read_bytes() == (b'' if os.environ.get('FULLA_FIXTURE_NEW') else b'\\x00\\xff\\n\\n')
pathlib.Path(os.environ['FULLA_FIXTURE_RECEIPT']).write_text(str(file))
(file.parent / 'editor-backup').write_bytes(file.read_bytes())
if os.environ.get('FULLA_FIXTURE_WAIT'):
    tty.setraw(sys.stdin.fileno())
    print('fixture-editor-ready', flush=True)
    time.sleep(60)
file.write_bytes(b'\\xff\\x00edited\\n\\n')
""")
    config = home / "config.toml"
    config.write_text("editor = " + json.dumps([sys.executable, str(editor)]) + "\n")
    config.chmod(0o600)
    env["FULLA_FIXTURE_RECEIPT"] = str(receipt)
    env["FULLA_FIXTURE_NEW"] = "1"
    code, _ = terminal(["--config", str(config), "add", "editor-added"], [
        (b"[e]ditor:", b"e\n"),
    ])
    assert code == 0 and cli("show", "editor-added") == b"\xff\x00edited\n\n"
    assert not Path(receipt.read_text()).parent.exists()
    del env["FULLA_FIXTURE_NEW"]
    cli("add", "edited", "--stdin", data=b"\x00\xff\n\n")
    code, _ = terminal(["--config", str(config), "edit", "edited"], [])
    assert code == 0 and cli("show", "edited") == b"\xff\x00edited\n\n"
    assert not Path(receipt.read_text()).parent.exists(), "editor files retained"

    cli("edit", "edited", "--stdin", data=b"\x00\xff\n\n")
    env["FULLA_FIXTURE_WAIT"] = "1"
    code, _ = terminal(["--config", str(config), "edit", "edited"], [
        (b"fixture-editor-ready", signal.SIGTERM),
    ])
    assert code == 143, f"wrong signal status: {code}"
    assert cli("show", "edited") == b"\x00\xff\n\n"
    assert not Path(receipt.read_text()).parent.exists(), "interrupted editor files retained"
    assert not (store / "lock").exists(), "interrupted editor left lock"

    code, _ = terminal(["add", "cancelled"], [
        (b"[e]ditor:", b"t\n"), (b"no newline is added):", b"\x03"),
    ])
    assert code == 130
    assert not (store / "passwords" / "cancelled.age").exists()
    assert not (store / "lock").exists()
print(json.dumps({"hidden_input": True, "generation": True, "editor_exact_bytes": True,
                  "editor_signal_cleanup": True, "stdin_independent": True}))
