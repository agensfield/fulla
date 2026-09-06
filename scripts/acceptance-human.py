"""Ordered human journeys on disposable Git/no-Git stores and clipboard tools."""

import json
import os
import subprocess
import sys
import tempfile
import time
from pathlib import Path
from typing import cast

from acceptance_terminal import terminal

binary = str(Path(sys.argv[1]).resolve())


def journey(no_git: bool) -> dict[str, object]:
    with tempfile.TemporaryDirectory(prefix="fulla-human-") as temporary:
        home = Path(temporary).resolve()
        store = home / "store"
        bindir = home / "bin"
        bindir.mkdir()
        clipboard = home / "clipboard"
        helper = f"""#!{sys.executable}
import pathlib, sys
state = pathlib.Path({str(clipboard)!r})
if pathlib.Path(sys.argv[0]).name in ('pbcopy', 'wl-copy'):
    state.write_bytes(sys.stdin.buffer.read())
else:
    sys.stdout.buffer.write(state.read_bytes())
"""
        for name in ("pbcopy", "pbpaste", "wl-copy", "wl-paste"):
            tool = bindir / name
            _ = tool.write_text(helper)
            tool.chmod(0o700)
        expected = home / "editor-before"
        payload = home / "editor-after"
        edited_path = home / "editor-path"
        editor = home / "editor.py"
        _ = editor.write_text(f"""import pathlib, sys
file = pathlib.Path(sys.argv[1])
assert file.stat().st_mode & 0o777 == 0o600
assert file.parent.stat().st_mode & 0o777 == 0o700
assert file.read_bytes() == pathlib.Path({str(expected)!r}).read_bytes()
pathlib.Path({str(edited_path)!r}).write_text(str(file))
file.write_bytes(pathlib.Path({str(payload)!r}).read_bytes())
""")
        config = home / "config.toml"
        _ = config.write_text(
            "editor = " + json.dumps([sys.executable, str(editor)]) + "\n"
        )
        config.chmod(0o600)
        env = {
            **os.environ,
            "HOME": str(home),
            "TMPDIR": str(home),
            "XDG_CONFIG_HOME": str(home / "config"),
            "FULLA_CONFIG": "",
            "FULLA_DIR": "",
            "PA_DIR": "",
            "WAYLAND_DISPLAY": "fixture-wayland",
            "PATH": str(bindir) + os.pathsep + os.environ["PATH"],
        }
        base = [binary, "--config", str(config), "--store", str(store)]
        steps: list[str] = []
        values = {
            "nested/entry": b"synthetic-human-hidden-value",
            "empty": b"",
            "binary": b"\x00\xffmulti\nline\n\n",
        }

        def human(
            args: list[str],
            actions: list[tuple[bytes, bytes | int]] | None = None,
            status: int = 0,
        ) -> bytes:
            code, output = terminal(
                binary, env, ["--config", str(config), *args], actions or [], store
            )
            assert code == status, f"human {args}: exit={code}, output={output!r}"
            for value in values.values():
                if value:
                    assert value not in output, (
                        "secret echoed by metadata/prompt operation"
                    )
            steps.append(" ".join(args[:2]))
            return output

        def observe(*args: str) -> bytes:
            return subprocess.run(
                base + list(args),
                stdin=subprocess.DEVNULL,
                env=env,
                capture_output=True,
                check=True,
                timeout=20,
            ).stdout

        def metadata(*args: str) -> dict[str, object]:
            # Machine observation selects opaque IDs without coupling the
            # journey to the presentation format of human-readable metadata.
            result = cast(dict[str, object], json.loads(observe(*args, "--json")))
            assert result["schema"] == "fulla.cli/v1" and result["ok"]
            return cast(dict[str, object], result["data"])

        _ = human(
            ["init", *(["--no-git"] if no_git else [])], [(b"Continue? [y/N]:", b"y\n")]
        )
        assert (store / "passwords/.git").is_dir() == (not no_git)
        for name in ("nested/entry", "empty"):
            _ = human(
                ["add", name],
                [
                    (b"[e]ditor:", b"t\n"),
                    (b"no newline is added):", values[name] + b"\n"),
                ],
            )
            assert observe("show", name) == values[name]
        _ = human(["add", "generated"], [(b"[e]ditor:", b"g\n")])
        values["generated"] = observe("show", "generated")
        assert len(values["generated"]) == 32
        _ = expected.write_bytes(b"")
        _ = payload.write_bytes(values["binary"])
        _ = human(["add", "binary"], [(b"[e]ditor:", b"e\n")])
        assert observe("show", "binary") == values["binary"]
        assert not Path(edited_path.read_text()).parent.exists()
        _ = expected.write_bytes(values["nested/entry"])
        values["nested/entry"] = b"synthetic-human-edited-value"
        _ = payload.write_bytes(values["nested/entry"])
        _ = human(["edit", "nested/entry"])
        assert observe("show", "nested/entry") == values["nested/entry"]
        assert not Path(edited_path.read_text()).parent.exists()
        listing = human(["list"])
        assert all(name.encode() in listing for name in values)
        _ = human(["copy", "nested/entry", "--clear-after", "1s"])
        assert clipboard.read_bytes() == values["nested/entry"]
        deadline = time.monotonic() + 5
        while clipboard.read_bytes() and time.monotonic() < deadline:
            time.sleep(0.05)
        assert clipboard.read_bytes() == b"", "scheduled fixture clipboard clear failed"
        _ = human(["move", "nested/entry", "moved"])
        values["moved"] = values.pop("nested/entry")
        assert not (store / "passwords/nested/entry.age").exists()
        _ = human(["backup", "list"])
        revision = ""
        if not no_git:
            _ = human(["history", "list", "moved"])
            history = cast(
                list[dict[str, str]], metadata("history", "list", "moved")["entries"]
            )
            revision = history[0]["commit"]
        else:
            _ = human(["remove", "moved"], status=1)
            assert observe("show", "moved") == values["moved"]
        _ = human(["remove", "moved", *(["--permanent-delete"] if no_git else [])])
        assert not (store / "passwords/moved.age").exists()
        _ = human(["backup", "list"])
        backups = cast(list[dict[str, str]], metadata("backup", "list")["backups"])
        snapshot = backups[0]["id"]
        _ = human(["backup", "show", snapshot])
        restore = (
            ["backup", "restore", snapshot, "--phase", "before"]
            if no_git
            else ["history", "restore", revision, "moved"]
        )
        _ = human(restore, [(b"Continue? [y/N]:", b"n\n")], status=1)
        assert not (store / "passwords/moved.age").exists()
        _ = human(restore, [(b"Continue? [y/N]:", b"y\n")])
        for name, value in values.items():
            assert observe("show", name) == value, "journey changed exact entry bytes"
        assert metadata("list")["names"] == sorted(values), "unexpected final entry set"
        assert metadata("doctor", "--deep")["healthy"]
        assert not (store / "lock").exists()
        assert not list((store / ".fulla/transactions").iterdir())
        return {
            "git": not no_git,
            "steps": steps,
            "exact_values": len(values),
            "deletion_recovered": True,
            "clipboard": "isolated fixture",
        }


print(json.dumps({"human_journeys": [journey(False), journey(True)]}))
