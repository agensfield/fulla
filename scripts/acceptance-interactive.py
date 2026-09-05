"""Isolated controlling-terminal acceptance. Only generated fixture data is used."""

import errno
import json
import os
import pty
import select
import signal
import subprocess
import sys
import tempfile
import termios
import time
from pathlib import Path
from typing import cast

binary = str(Path(sys.argv[1]).resolve())
with tempfile.TemporaryDirectory(prefix="fulla-tty-") as temporary:
    home = Path(temporary).resolve()
    store = home / "store"
    env = {
        **os.environ,
        "HOME": str(home),
        "TMPDIR": str(home),
        "XDG_CONFIG_HOME": str(home / "config"),
        "FULLA_CONFIG": "",
        "FULLA_DIR": "",
        "PA_DIR": "",
    }
    base = [binary, "--store", str(store)]

    def cli(*args: str, data: bytes | None = None) -> bytes:
        return subprocess.run(
            base + list(args), input=data, env=env, capture_output=True, check=True
        ).stdout

    def terminal(
        args: list[str],
        actions: list[tuple[bytes, bytes | int]],
        target: Path | None = None,
    ) -> tuple[int, bytes]:
        pid, fd = pty.fork()
        if pid == 0:
            # Prompts and hidden input must use /dev/tty, not standard input.
            null = os.open(os.devnull, os.O_RDONLY)
            _ = os.dup2(null, 0)
            os.close(null)
            command = base if target is None else [binary, "--store", str(target)]
            os.execve(binary, command + args, env)
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
                        assert os.write(fd, response) == len(response)
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
            assert not pending, (
                f"terminal prompt was missing: exit={result}, output={bytes(output)!r}"
            )
            restored = cast(int, termios.tcgetattr(fd)[3])
            assert restored & termios.ECHO and restored & termios.ICANON, (
                "terminal state was not restored"
            )
            return result, bytes(output)
        finally:
            if result is None:
                os.kill(pid, signal.SIGKILL)
                _ = os.waitpid(pid, 0)
            os.close(fd)

    for flags in (["--json"], ["--non-interactive"]):
        code, output = terminal(["init", *flags], [])
        assert code == 1 and b"Continue?" not in output and not store.exists()
    code, _ = terminal(["init", "--dry-run"], [])
    assert code == 0 and not store.exists()
    code, _ = terminal(["init"], [(b"Continue? [y/N]:", b"\n")])
    assert code == 1 and not store.exists()
    code, _ = terminal(["init"], [(b"Continue? [y/N]:", signal.SIGTERM)])
    assert code == 143 and not store.exists()
    assert not list(home.glob(".fulla-init-*")), "cancelled init left staging material"
    code, output = terminal(["init"], [(b"Continue? [y/N]:", b"yes\n")])
    assert code == 0 and (store / "passwords" / ".git").is_dir()
    assert b"entry names and change times" in output

    untracked = home / "untracked"
    code, output = terminal(
        ["init", "--no-git"], [(b"Continue? [y/N]:", b"y\n")], target=untracked
    )
    assert code == 0 and b"Git history is disabled" in output
    assert not (untracked / "passwords" / ".git").exists()
    identity_before = (untracked / "identities").read_bytes()
    # Turn this disposable native fixture into an unadopted pa-layout fixture.
    # Preserve its metadata elsewhere rather than deleting it.
    _ = (untracked / ".fulla").rename(home / "fixture-metadata")
    code, _ = terminal(
        ["init", "--adopt"], [(b"Continue? [y/N]:", b"n\n")], target=untracked
    )
    assert code == 1 and not (untracked / ".fulla").exists()
    code, output = terminal(
        ["init", "--adopt"], [(b"Continue? [y/N]:", b"y\n")], target=untracked
    )
    assert code == 0 and b"preserving its identity and entries" in output
    assert (untracked / "identities").read_bytes() == identity_before
    code, output = terminal(["init"], [], target=untracked)
    assert code == 1 and b"Continue?" not in output

    code, output = terminal(
        ["add", "typed"],
        [
            (b"[e]ditor:", b"t\n"),
            (b"no newline is added):", b"synthetic-hidden-value\n"),
        ],
    )
    assert code == 0 and b"synthetic-hidden-value" not in output
    assert cli("show", "typed") == b"synthetic-hidden-value"

    code, output = terminal(["add", "generated"], [(b"[e]ditor:", b"g\n")])
    assert code == 0 and len(cli("show", "generated")) == 32

    editor = home / "editor.py"
    receipt = home / "editor-receipt"
    _ = editor.write_text("""import os, pathlib, sys, time, tty
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
    _ = config.write_text(
        "editor = " + json.dumps([sys.executable, str(editor)]) + "\n"
    )
    config.chmod(0o600)
    env["FULLA_FIXTURE_RECEIPT"] = str(receipt)
    env["FULLA_FIXTURE_NEW"] = "1"
    code, _ = terminal(
        ["--config", str(config), "add", "editor-added"],
        [
            (b"[e]ditor:", b"e\n"),
        ],
    )
    assert code == 0 and cli("show", "editor-added") == b"\xff\x00edited\n\n"
    assert not Path(receipt.read_text()).parent.exists()
    del env["FULLA_FIXTURE_NEW"]
    _ = cli("add", "edited", "--stdin", data=b"\x00\xff\n\n")
    code, _ = terminal(["--config", str(config), "edit", "edited"], [])
    assert code == 0 and cli("show", "edited") == b"\xff\x00edited\n\n"
    assert not Path(receipt.read_text()).parent.exists(), "editor files retained"

    _ = cli("edit", "edited", "--stdin", data=b"\x00\xff\n\n")
    env["FULLA_FIXTURE_WAIT"] = "1"
    code, _ = terminal(
        ["--config", str(config), "edit", "edited"],
        [
            (b"fixture-editor-ready", signal.SIGTERM),
        ],
    )
    assert code == 143, f"wrong signal status: {code}"
    assert cli("show", "edited") == b"\x00\xff\n\n"
    assert not Path(receipt.read_text()).parent.exists(), (
        "interrupted editor files retained"
    )
    assert not (store / "lock").exists(), "interrupted editor left lock"

    code, _ = terminal(
        ["add", "cancelled"],
        [
            (b"[e]ditor:", b"t\n"),
            (b"no newline is added):", b"\x03"),
        ],
    )
    assert code == 130
    assert not (store / "passwords" / "cancelled.age").exists()
    assert not (store / "lock").exists()
    historical = b"synthetic-historical-secret\x00\xff"
    replacement = b"synthetic-current-secret"
    _ = cli("add", "restore-target", "--stdin", data=historical)
    log = cast(
        dict[str, object],
        json.loads(cli("history", "list", "restore-target", "--json")),
    )
    log_data = cast(dict[str, object], log["data"])
    entries = cast(list[dict[str, str]], log_data["entries"])
    revision = entries[0]["commit"]
    _ = cli("edit", "restore-target", "--stdin", data=replacement)
    for flags in (["--json"], ["--non-interactive"]):
        code, output = terminal(
            ["history", "restore", revision, "restore-target", *flags], []
        )
        assert code == 1 and b"Continue?" not in output
        assert cli("show", "restore-target") == replacement
    code, output = terminal(
        ["history", "restore", revision, "restore-target"],
        [(b"Continue? [y/N]:", b"n\n")],
    )
    assert code == 1 and cli("show", "restore-target") == replacement
    assert b"synthetic-historical-secret" not in output and replacement not in output
    assert not (store / "lock").exists()
    code, _ = terminal(
        ["history", "restore", revision, "restore-target"],
        [(b"Continue? [y/N]:", signal.SIGTERM)],
    )
    assert code == 143 and cli("show", "restore-target") == replacement
    assert not (store / "lock").exists()
    code, output = terminal(
        ["history", "restore", revision, "restore-target"],
        [(b"Continue? [y/N]:", b"y\n")],
    )
    assert code == 0 and cli("show", "restore-target") == historical
    assert b"Replace the current entry" in output
    assert b"synthetic-historical-secret" not in output and replacement not in output
    _ = cli("remove", "restore-target", "--yes")
    code, output = terminal(
        ["history", "restore", revision, "restore-target"],
        [(b"Continue? [y/N]:", b"y\n")],
    )
    assert code == 0 and b"Recreate the missing entry" in output
    assert cli("show", "restore-target") == historical

    snapshot_bytes = b"synthetic-snapshot-value\x00\xff"
    snapshot_result = cast(
        dict[str, object],
        json.loads(
            cli("add", "snapshot-base", "--stdin", "--json", data=snapshot_bytes)
        ),
    )
    snapshot_data = cast(dict[str, str], snapshot_result["data"])
    snapshot_id = snapshot_data["transaction"]
    _ = cli("edit", "snapshot-base", "--stdin", data=b"changed-snapshot-value")
    _ = cli("add", "snapshot-extra", "--stdin", data=b"extra-snapshot-value")
    for flags in (["--json"], ["--non-interactive"]):
        code, output = terminal(
            ["backup", "restore", snapshot_id, "--phase", "after", *flags], []
        )
        assert code == 1 and b"Continue?" not in output
    code, output = terminal(
        ["backup", "restore", snapshot_id, "--phase", "after"],
        [(b"Continue? [y/N]:", b"n\n")],
    )
    assert code == 1 and b'Remove: ["snapshot-extra"]' in output
    assert cli("show", "snapshot-base") == b"changed-snapshot-value"
    assert cli("show", "snapshot-extra") == b"extra-snapshot-value"
    assert b"synthetic-snapshot-value" not in output
    assert not (store / "lock").exists()
    code, _ = terminal(
        ["backup", "restore", snapshot_id, "--phase", "after"],
        [(b"Continue? [y/N]:", signal.SIGTERM)],
    )
    assert code == 143 and cli("show", "snapshot-extra") == b"extra-snapshot-value"
    assert not (store / "lock").exists()
    code, output = terminal(
        ["backup", "restore", snapshot_id, "--phase", "after"],
        [(b"Continue? [y/N]:", b"y\n")],
    )
    assert code == 0 and cli("show", "snapshot-base") == snapshot_bytes
    assert not (store / "passwords" / "snapshot-extra.age").exists()
    assert b"synthetic-snapshot-value" not in output

    plugin_binary = str(Path(binary).parent / "age-plugin-fullafixture")
    plugin_store = home / "plugin-store"
    env["PATH"] = str(Path(plugin_binary).parent) + os.pathsep + env["PATH"]

    def plugin_cli(*args: str, data: bytes = b"") -> subprocess.CompletedProcess[bytes]:
        return subprocess.run(
            [binary, "--store", str(plugin_store), *args],
            input=data,
            env=env,
            capture_output=True,
            check=False,
            timeout=15,
        )

    assert plugin_cli("init", "--no-git", "--yes").returncode == 0
    keys = cast(
        dict[str, str],
        json.loads(
            subprocess.run(
                [plugin_binary, "fixture-keys"],
                env=env,
                capture_output=True,
                check=True,
                timeout=15,
            ).stdout
        ),
    )
    _ = (plugin_store / "identities").write_text(keys["identity"] + "\n")
    _ = (plugin_store / "recipients").write_text(keys["recipient"] + "\n")
    env["FULLA_PLUGIN_FIXTURE_REQUEST"] = ""
    assert (
        plugin_cli("add", "entry", "--stdin", data=b"plugin-fixture-value").returncode
        == 0
    )
    env["FULLA_PLUGIN_FIXTURE_REQUEST"] = "1"
    code, output = terminal(
        ["show", "entry"], [(b"value (hidden):", b"fixture-pin\n")], target=plugin_store
    )
    assert (
        code == 0 and b"plugin-fixture-value" in output and b"fixture-pin" not in output
    )
    for flags in (["--json"], ["--non-interactive"]):
        code, output = terminal(["show", "entry", *flags], [], target=plugin_store)
        assert (
            code == 1
            and b"interaction.required" in output
            and b"value (hidden):" not in output
        )
    previous = (plugin_store / "passwords/entry.age").read_bytes()
    code, output = terminal(
        ["edit", "entry", "--generate"],
        [(b"value (hidden):", signal.SIGTERM)],
        target=plugin_store,
    )
    assert code == 143 and b"plugin-fixture-value" not in output
    assert not (plugin_store / "lock").exists()
    assert (plugin_store / "passwords/entry.age").read_bytes() == previous
    env["FULLA_PLUGIN_FIXTURE_REQUEST"] = "confirm"
    code, output = terminal(
        ["add", "confirmed", "--generate", "--yes"],
        [(b"allow? [y/N]:", b"n\n")],
        target=plugin_store,
    )
    assert code == 1 and not (plugin_store / "passwords/confirmed.age").exists()
    code, output = terminal(
        ["add", "confirmed", "--generate", "--yes"],
        [(b"allow? [y/N]:", b"y\n")],
        target=plugin_store,
    )
    assert code == 0 and (plugin_store / "passwords/confirmed.age").exists()
    assert b"\x1b[31m" not in output and b"fixture message" in output
    env["FULLA_PLUGIN_FIXTURE_REQUEST"] = ""

print(
    json.dumps(
        {
            "plugin_terminal_interaction": True,
            "snapshot_restore_confirmation": True,
            "history_restore_confirmation": True,
            "initialization_confirmation": True,
            "adoption_identity_preserved": True,
            "noninteractive_no_prompt": True,
            "hidden_input": True,
            "generation": True,
            "editor_exact_bytes": True,
            "editor_signal_cleanup": True,
            "stdin_independent": True,
        }
    )
)
