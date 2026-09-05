"""Exercise actual historical/current Fulla binaries on disposable pa-v1 stores."""

import hashlib
import json
import os
import select
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import cast

PREDECESSOR = "831caf68655b41b4ca5b064b7693af35df68a6e6"
old, current, fixture = [str(Path(arg).resolve()) for arg in sys.argv[1:4]]
_ = os.umask(0o077)


def obj(data: bytes) -> dict[str, object]:
    value = cast(object, json.loads(data))
    assert isinstance(value, dict)
    return cast(dict[str, object], value)


def files(root: Path) -> dict[str, str]:
    return {
        str(p.relative_to(root)): hashlib.sha256(p.read_bytes()).hexdigest()
        for p in root.rglob("*")
        if p.is_file()
    }


def acceptance(no_git: bool) -> None:
    with tempfile.TemporaryDirectory(prefix="fulla-upgrade-") as temporary:
        home = Path(temporary).resolve()
        store = home / "store"
        env = {
            "PATH": "/usr/bin:/bin",
            "HOME": str(home),
            "TMPDIR": str(home),
            "XDG_CONFIG_HOME": str(home / "config"),
            "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_CONFIG_GLOBAL": os.devnull,
            "GIT_AUTHOR_NAME": "Fixture",
            "GIT_AUTHOR_EMAIL": "fixture@localhost",
            "GIT_COMMITTER_NAME": "Fixture",
            "GIT_COMMITTER_EMAIL": "fixture@localhost",
            # Keep unrelated Git background processes out of the historical
            # binary fixture. This does not claim the old build fixed that bug.
            "GIT_CONFIG_COUNT": "2",
            "GIT_CONFIG_KEY_0": "maintenance.auto",
            "GIT_CONFIG_VALUE_0": "false",
            "GIT_CONFIG_KEY_1": "gc.auto",
            "GIT_CONFIG_VALUE_1": "0",
        }

        def run(binary: str, *args: str, data: bytes = b"") -> dict[str, object]:
            result = subprocess.run(
                [binary, "--store", str(store), "--json", *args],
                input=data,
                env=env,
                capture_output=True,
                timeout=60,
                check=False,
            )
            assert result.returncode == 0, (
                args,
                result.returncode,
                result.stdout,
                result.stderr,
            )
            envelope = obj(result.stdout)
            assert envelope["schema"] == "fulla.cli/v1" and envelope["ok"] is True
            value = envelope["data"]
            assert isinstance(value, dict)
            return cast(dict[str, object], value)

        def exact(binary: str, name: str, value: bytes) -> None:
            result = subprocess.run(
                [binary, "--store", str(store), "show", name],
                input=b"",
                env=env,
                capture_output=True,
                timeout=30,
                check=False,
            )
            assert result.returncode == 0 and result.stdout == value

        _ = run(old, "init", "--yes", *(["--no-git"] if no_git else []))
        original = b"\x00\xfforiginal\n\n"
        changed = b"\x80replacement\x00"
        _ = run(old, "add", "nested/value", "--stdin", data=original)
        edit = run(old, "edit", "nested/value", "--stdin", data=changed)
        backup = edit["transaction"]
        assert isinstance(backup, str)
        history_commit = ""
        if not no_git:
            entries = run(old, "history", "list", "nested/value")["entries"]
            assert isinstance(entries, list)
            entries = cast(list[object], entries)
            assert len(entries) == 2
            entry = cast(dict[str, object], entries[1])
            history_commit = str(entry["commit"])
        _ = run(old, "identity", "rotate", "--yes")
        manifest = (store / ".fulla/store.json").read_bytes()
        identity = (store / "identities").read_bytes()
        recipients = (store / "recipients").read_bytes()
        before = files(store)
        _ = run(current, "doctor", "--deep")
        exact(current, "nested/value", changed)
        _ = run(current, "backup", "show", backup)
        assert files(store) == before, "read-only upgrade rewrote historical state"

        # A snapshot created by the historical binary remains restorable after
        # its identity rotation, using the sealed historical identity chain.
        _ = run(current, "backup", "restore", backup, "--phase", "before", "--yes")
        exact(current, "nested/value", original)
        _ = run(old, "doctor", "--deep")
        exact(old, "nested/value", original)
        if not no_git:
            _ = run(
                current, "history", "restore", history_commit, "nested/value", "--yes"
            )
            exact(old, "nested/value", original)

        # Basic rollback alternates actual binaries, keeping Fulla metadata.
        _ = run(old, "add", "rollback", "--stdin", data=b"old writer\x00\xff")
        exact(current, "rollback", b"old writer\x00\xff")
        _ = run(current, "edit", "rollback", "--stdin", data=b"current writer\n")
        exact(old, "rollback", b"current writer\n")
        _ = run(old, "move", "rollback", "renamed")
        exact(current, "renamed", b"current writer\n")
        _ = run(
            current, "remove", "renamed", *(["--permanent-delete"] if no_git else [])
        )
        _ = run(old, "doctor", "--deep")
        assert (store / ".fulla/store.json").read_bytes() == manifest
        assert (store / "identities").read_bytes() == identity
        assert (store / "recipients").read_bytes() == recipients
        assert (store / "passwords/.git").exists() is not no_git

        # Current identity rotation must also preserve the old binary's basic
        # reads and its ability to restore its own encrypted historical snapshot.
        _ = run(current, "identity", "rotate", "--yes")
        exact(old, "nested/value", original)
        _ = run(old, "backup", "restore", backup, "--phase", "after", "--yes")
        exact(current, "nested/value", changed)
        _ = run(current, "doctor", "--deep")
        # New transaction snapshot metadata must make an actual old recovery
        # binary refuse, preserving the journal for a supporting binary.
        _ = run(current, "add", "a", "--stdin", data=b"original")
        manifest_path = store / ".fulla/store.json"
        saved_manifest = manifest_path.read_bytes()
        future = obj(saved_manifest)
        domains = cast(dict[str, object], future["domains"])
        domains["backup"] = 2
        _ = manifest_path.write_text(json.dumps(future))
        child: subprocess.Popen[bytes] = subprocess.Popen(
            [fixture, "-test.run=^TestTransactionCrashHelper$"],
            env={
                **env,
                "FULLA_TRANSACTION_FIXTURE": str(store),
                "FULLA_TRANSACTION_PHASE": "published:a",
            },
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        try:
            assert child.stdout is not None
            ready, _, _ = select.select([child.stdout], [], [], 20)
            assert ready
            line = cast(bytes, child.stdout.readline())
            assert line.strip() == b"transaction-ready"
            child.kill()
            _ = child.wait(timeout=10)
        finally:
            if child.poll() is None:
                child.kill()
                _ = child.wait(timeout=10)
            if child.stdout is not None:
                child.stdout.close()
            if child.stderr is not None:
                child.stderr.close()
        before_old = {
            k: v for k, v in files(store).items() if not k.startswith("lock/")
        }
        owner = (store / "lock/owner").read_text().strip()
        refused = subprocess.run(
            [old, "--store", str(store), "--json", "doctor", "--recover-lock", owner],
            input=b"",
            env=env,
            capture_output=True,
            timeout=30,
            check=False,
        )
        assert refused.returncode == 1
        failure = obj(refused.stdout)
        assert cast(dict[str, object], failure["error"])["code"] == "metadata.invalid"
        assert before_old == {
            k: v for k, v in files(store).items() if not k.startswith("lock/")
        }
        # The historical recovery claims a new owner before parsing the journal.
        # Read that new token; never retry with stale authority.
        owner = (store / "lock/owner").read_text().strip()
        recovered = run(current, "doctor", "--recover-lock", owner)
        assert recovered["recovered"] is True and recovered["lock_released"] is True
        exact(current, "b", b"\x00\xff\n")
        _ = manifest_path.write_bytes(saved_manifest)  # Undo the fixture version only.
        snapshot = run(current, "backup", "show", str(recovered["transaction"]))
        assert snapshot["snapshot_domain"] == "transactions"
        print(
            json.dumps({"predecessor": PREDECESSOR, "no_git": no_git, "accepted": True})
        )


for no_git in (False, True):
    acceptance(no_git)
