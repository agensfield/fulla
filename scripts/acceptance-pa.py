"""Real pinned shell-pa compatibility; generated disposable stores only."""

import contextlib
import hashlib
import json
import os
import select
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import cast

PREDECESSOR = "f75734b8775f72d5d2f9630c08c2b48bdb6d8104"
binary, repository, age_tools, sshd = [
    str(Path(arg).resolve()) for arg in sys.argv[1:5]
]
source = subprocess.run(
    ["git", "-C", repository, "show", f"{PREDECESSOR}:pa"],
    capture_output=True,
    check=True,
).stdout
_ = os.umask(0o077)


def object_json(data: bytes | str) -> dict[str, object]:
    value = cast(object, json.loads(data))
    assert isinstance(value, dict)
    return cast(dict[str, object], value)


def result_json(data: bytes) -> dict[str, object]:
    envelope = object_json(data)
    assert envelope["schema"] == "fulla.cli/v1" and envelope["ok"] is True
    result = envelope["data"]
    assert isinstance(result, dict)
    return cast(dict[str, object], result)


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
                command,
                input=data,
                env=env,
                capture_output=True,
                timeout=30,
                check=False,
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
                str(item.relative_to(store)): hashlib.sha256(
                    item.read_bytes()
                ).hexdigest()
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
        _ = (lock / "info").write_text(
            f"pid={os.getpid()} host=fixture operation=test\n"
        )
        locked = snapshot()
        _ = shell("edit", "--stdin", "binary", data=b"forbidden", expected=1)
        _ = fulla("edit", "binary", "--stdin", data=b"forbidden", expected=1)
        assert snapshot() == locked, "locked writer changed store"
        (lock / "owner").unlink()
        (lock / "info").unlink()
        lock.rmdir()
        # Use real OpenSSH against two disposable loopback servers. Each server
        # launches only the configured Fulla binary/store, never arbitrary exec.
        with contextlib.ExitStack() as cleanup:
            other = home / "other"
            _ = run(
                [binary, "--store", str(other), "init", "--no-git", "--yes", "--json"]
            )
            remote_value = b"remote-fixture\x00\xff\n"
            _ = run(
                [binary, "--store", str(other), "add", "remote-only", "--stdin"],
                remote_value,
            )
            _ = run(
                [binary, "--store", str(other), "add", "binary", "--stdin"],
                b"shared-remote",
            )

            def stop(process: subprocess.Popen[bytes]) -> None:
                if process.poll() is None:
                    process.terminate()
                    try:
                        _ = process.wait(timeout=10)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        _ = process.wait(timeout=10)

            def endpoint(target: Path, label: str) -> list[str]:
                log = cleanup.enter_context((home / f"{label}.log").open("wb"))
                process: subprocess.Popen[bytes] = subprocess.Popen(
                    [
                        sshd,
                        "--dir",
                        str(home / label),
                        "--binary",
                        binary,
                        "--store",
                        str(target),
                        "--remote-port",
                        "0",
                    ],
                    env=env,
                    stdout=subprocess.PIPE,
                    stderr=log,
                )
                _ = cleanup.callback(stop, process)
                assert process.stdout is not None
                _ = cleanup.callback(process.stdout.close)
                ready, _, _ = select.select([process.stdout], [], [], 15)
                assert ready, "fixture SSH listener did not start"
                info = object_json(cast(bytes, process.stdout.readline()))
                address = info["address"]
                assert isinstance(address, str)
                port = address.rsplit(":", 1)[1]
                options = ["--host", "fulla-fixture@127.0.0.1"]
                for option in (
                    f"Port={port}",
                    f"IdentityFile={info['client_key']}",
                    f"UserKnownHostsFile={info['known_hosts']}",
                    f"GlobalKnownHostsFile={os.devnull}",
                    "StrictHostKeyChecking=yes",
                    "IdentitiesOnly=yes",
                ):
                    options.extend(["--ssh-option", option])
                return options

            _ = result_json(fulla("doctor", "--deep", "--json"))
            _ = result_json(
                run([binary, "--store", str(other), "doctor", "--deep", "--json"])
            )
            local_endpoint = endpoint(store, "local-sshd")
            remote_endpoint = endpoint(other, "remote-sshd")
            local_identity = result_json(fulla("identity", "show", "--json"))
            remote_identity = result_json(
                run([binary, "--store", str(other), "identity", "show", "--json"])
            )
            local_fingerprint = local_identity["fingerprint"]
            remote_fingerprint = remote_identity["fingerprint"]
            assert isinstance(local_fingerprint, str) and isinstance(
                remote_fingerprint, str
            )
            _ = fulla(
                "peer",
                "add",
                "other",
                *remote_endpoint,
                "--expect-fingerprint",
                remote_fingerprint,
                "--json",
            )
            _ = run(
                [
                    binary,
                    "--store",
                    str(other),
                    "peer",
                    "add",
                    "other",
                    *local_endpoint,
                    "--expect-fingerprint",
                    local_fingerprint,
                    "--json",
                ]
            )
            before_refusal = snapshot()
            refused = object_json(fulla("sync", "other", "--json", expected=1))
            assert snapshot() == before_refusal
            error = refused["error"]
            assert isinstance(error, dict) and error["code"] == "sync.dry_run_required"
            before_sync = snapshot()
            preview = result_json(fulla("sync", "other", "--dry-run", "--json"))
            assert preview["pull"] == ["remote-only"] and preview["skipped"] == [
                "binary"
            ]
            # Dry-run records trust evidence but preserves every live ciphertext.
            after_preview = snapshot()
            assert all(
                after_preview[name] == digest
                for name, digest in before_sync.items()
                if not name.startswith(".fulla/")
            )
            applied = result_json(fulla("sync", "other", "--json"))
            assert applied["activated"] and applied["pushed"] and applied["pulled"]
            receipt_name = applied["receipt"]
            assert isinstance(receipt_name, str)
            receipt = object_json((store / receipt_name).read_text())
            assert receipt["pa_xfer_retired"] is True
            assert (
                result_json(fulla("peer", "show", "other", "--json"))["activated"]
                is True
            )
            assert (
                result_json(
                    run(
                        [
                            binary,
                            "--store",
                            str(other),
                            "peer",
                            "show",
                            "other",
                            "--json",
                        ]
                    )
                )["activated"]
                is True
            )
            assert shell("show", "remote-only") == remote_value
            assert shell("show", "binary") == values["binary"]
            for name, value in values.items():
                expected_value = b"shared-remote" if name == "binary" else value
                assert (
                    run([binary, "--store", str(other), "show", name]) == expected_value
                )
            retry = result_json(fulla("sync", "other", "--json"))
            assert retry["push"] == [] and retry["pull"] == []
            assert not retry["pushed"] and not retry["pulled"]
        preserved_metadata = {
            name: digest
            for name, digest in snapshot().items()
            if name.startswith(".fulla/")
        }
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
        assert {
            name: digest
            for name, digest in snapshot().items()
            if name.startswith(".fulla/")
        } == preserved_metadata
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
                    "sync_cutover": True,
                    "transport": "real OpenSSH, two loopback fixture stores",
                }
            )
        )


for no_git in (False, True):
    acceptance(no_git)
