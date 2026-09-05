"""Real pinned shell-pa compatibility; generated disposable stores only."""

import hashlib
import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path

PREDECESSOR = "f75734b8775f72d5d2f9630c08c2b48bdb6d8104"
binary, repository, age_tools = [str(Path(arg).resolve()) for arg in sys.argv[1:4]]
source = subprocess.run(
    ["git", "-C", repository, "show", f"{PREDECESSOR}:pa"],
    capture_output=True,
    check=True,
).stdout
_ = os.umask(0o077)


def acceptance(no_git: bool) -> None:
    with tempfile.TemporaryDirectory(prefix="fulla-pa-") as temporary:
        home = Path(temporary).resolve()
        store = home / "store"
        pa = home / "pa"
        _ = pa.write_bytes(source)
        env = {
            "PATH": age_tools + ":/usr/bin:/bin",
            "HOME": str(home),
            "TMPDIR": str(home),
            "XDG_CONFIG_HOME": str(home / "config"),
            "PA_DIR": str(store),
            "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_CONFIG_GLOBAL": os.devnull,
            "GIT_AUTHOR_NAME": "Fixture",
            "GIT_AUTHOR_EMAIL": "fixture@localhost",
            "GIT_COMMITTER_NAME": "Fixture",
            "GIT_COMMITTER_EMAIL": "fixture@localhost",
            "GIT_CONFIG_COUNT": "2",
            "GIT_CONFIG_KEY_0": "maintenance.auto",
            "GIT_CONFIG_VALUE_0": "false",
            "GIT_CONFIG_KEY_1": "gc.auto",
            "GIT_CONFIG_VALUE_1": "0",
        }

        if no_git:
            env["PA_NOGIT"] = ""

        def run(command: list[str], data: bytes = b"", expected: int = 0) -> bytes:
            result = subprocess.run(
                command, input=data, env=env, capture_output=True, timeout=30, check=False
            )
            assert result.returncode == expected, (
                f"fixture command {command[0]} failed: status={result.returncode}; "
                f"stderr={result.stderr!r}; stdout={result.stdout!r}"
            )
            return result.stdout

        def shell(*args: str, data: bytes = b"", expected: int | None = None) -> bytes:
            # This pinned predecessor ends these functions in
            # `$git_enabled && git_add_and_commit`, returning 1 without Git even
            # after successful publication. Check resulting state independently;
            # do not normalize status for Fulla or other predecessor commands.
            if expected is None:
                expected = int(no_git and args[0] in {"add", "edit", "delete"})
            return run(["/bin/sh", str(pa), *args], data, expected)

        def fulla(*args: str, data: bytes = b"", expected: int = 0) -> bytes:
            return run([binary, "--store", str(store), *args], data, expected)

        def snapshot() -> dict[str, str]:
            return {
                str(item.relative_to(store)): hashlib.sha256(item.read_bytes()).hexdigest()
                for item in store.rglob("*")
                if item.is_file()
            }

        values = {
            "empty": b"",
            "nested/newline": b"fixture\n",
            "binary": b"\x00\xff\n",
            "large": bytes(range(256)) * 1024,
        }
        for name, value in values.items():
            _ = shell("add", "--stdin", name, data=value)
            assert shell("show", name) == value
        assert (store / "passwords/.git").is_dir() is (not no_git)
        original = snapshot()
        _ = fulla("init", "--adopt", "--dry-run", "--json")
        assert snapshot() == original, "adoption preview mutated the pa store"
        _ = fulla("init", "--adopt", "--yes", "--json")
        assert (store / "passwords/.git").is_dir() is (not no_git)
        adopted = snapshot()
        assert all(adopted.get(name) == digest for name, digest in original.items()), (
            "adoption changed predecessor files"
        )
        for name, value in values.items():
            assert fulla("show", name) == value
            _ = fulla("edit", name, "--stdin", data=value + b"\x00")
            assert shell("show", name) == value + b"\x00"
            _ = shell("edit", "--stdin", name, data=value)
            assert fulla("show", name) == value
        _ = fulla("move", "binary", "nested/moved")
        assert shell("show", "nested/moved") == values["binary"]
        _ = shell("move", "nested/moved", "binary")
        assert fulla("show", "binary") == values["binary"]
        if no_git:
            before_refusal = snapshot()
            _ = fulla("remove", "binary", expected=1)
            assert snapshot() == before_refusal
            assert fulla("show", "binary") == values["binary"]
            _ = fulla("remove", "binary", "--permanent-delete")
        else:
            _ = fulla("remove", "binary")
        _ = shell("add", "--stdin", "binary", data=values["binary"])
        assert fulla("show", "binary") == values["binary"]
        # Either implementation must honor the same lock directory before writing.
        lock = store / "lock"
        lock.mkdir(mode=0o700)
        _ = (lock / "owner").write_text("fixture-owner\n")
        _ = (lock / "info").write_text(f"pid={os.getpid()} host=fixture operation=test\n")
        locked = snapshot()
        _ = shell("edit", "--stdin", "binary", data=b"forbidden", expected=1)
        _ = fulla("edit", "binary", "--stdin", data=b"forbidden", expected=1)
        assert snapshot() == locked, "locked writer changed store"
        (lock / "owner").unlink()
        (lock / "info").unlink()
        lock.rmdir()
        # Basic rollback means cease invoking Fulla; no identity or live-format rewrite.
        _ = shell("add", "--stdin", "rollback", data=b"rollback\x00\xff\n")
        _ = shell("edit", "--stdin", "rollback", data=b"edited\n")
        _ = shell("move", "rollback", "rollback-moved")
        assert shell("show", "rollback-moved") == b"edited\n"
        _ = shell("delete", "rollback-moved", data=b"y\n")
        assert not (store / "passwords/rollback-moved.age").exists()
        assert (
            hashlib.sha256((store / "identities").read_bytes()).hexdigest()
            == original["identities"]
        )
        assert (
            hashlib.sha256((store / "recipients").read_bytes()).hexdigest()
            == original["recipients"]
        )
        assert (store / "passwords/.git").is_dir() is (not no_git)
        print(
            json.dumps(
                {
                    "pa_commit": PREDECESSOR,
                    "git": not no_git,
                    "pa_write_exit": 1 if no_git else 0,
                    "adoption": True,
                    "exact_bytes": True,
                    "alternating_crud": True,
                    "shared_lock": True,
                    "basic_rollback": True,
                    "sync_cutover": "separate acceptance required",
                }
            )
        )


for no_git in (False, True):
    acceptance(no_git)
