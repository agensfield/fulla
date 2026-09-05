"""Authorized noninteractive CLI journey; generated disposable secrets only."""

import base64
import hashlib
import json
import os
import stat
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import cast

binary = str(Path(sys.argv[1]).resolve())
_ = os.umask(0o077)


def obj(data: bytes) -> dict[str, object]:
    value = cast(object, json.loads(data))
    assert isinstance(value, dict)
    return cast(dict[str, object], value)


def tree(root: Path) -> dict[str, tuple[int, str]]:
    return {
        str(p.relative_to(root)): (
            p.stat().st_mode,
            hashlib.sha256(p.read_bytes()).hexdigest() if p.is_file() else "",
        )
        for p in root.rglob("*")
    }


def acceptance(no_git: bool) -> None:
    with tempfile.TemporaryDirectory(prefix="fulla-agent-") as temporary:
        home = Path(temporary).resolve()
        source, recovery, target = [
            home / name for name in ("source", "recovery", "target")
        ]
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
            "UNSELECTED": "fixture ambient value",
        }
        visited: set[str] = set()

        def raw(
            store: Path, *args: str, data: bytes = b"", inherited: tuple[int, ...] = ()
        ) -> bytes:
            result = subprocess.run(
                [binary, "--store", str(store), *args],
                input=data,
                pass_fds=inherited,
                env=env,
                capture_output=True,
                start_new_session=True,
                timeout=180,
                check=False,
            )
            assert result.returncode == 0, (
                args,
                result.returncode,
                result.stdout,
                result.stderr,
            )
            return result.stdout

        def run(
            store: Path, *args: str, data: bytes = b"", inherited: tuple[int, ...] = ()
        ) -> dict[str, object]:
            result = obj(raw(store, "--json", *args, data=data, inherited=inherited))
            assert result["schema"] == "fulla.cli/v1" and result["ok"] is True
            assert isinstance(result["warnings"], list)
            command = result["command"]
            assert isinstance(command, str)
            visited.add(command)
            value = result["data"]
            assert isinstance(value, dict)
            return cast(dict[str, object], value)

        def exact(store: Path, name: str, expected: bytes) -> None:
            shown = run(store, "show", name)
            assert shown["encoding"] == "base64"
            value = shown["value"]
            assert (
                isinstance(value, str)
                and base64.b64decode(value, validate=True) == expected
            )
            assert raw(store, "--non-interactive", "show", name) == expected

        _ = run(source, "init", "--yes", *(["--no-git"] if no_git else []))
        _ = run(source, "add", "empty", "--stdin")
        original = b"\x00\xffbinary\n\n"
        descriptor, writer = os.pipe()
        try:
            assert os.write(writer, original) == len(original)
        finally:
            os.close(writer)
        try:
            _ = run(
                source,
                "add",
                "binary",
                "--from-fd",
                str(descriptor),
                inherited=(descriptor,),
            )
        finally:
            os.close(descriptor)
        _ = run(
            source,
            "add",
            "generated",
            "--generate",
            "--length",
            "37",
            "--alphabet",
            "ab",
        )
        generated = raw(source, "--non-interactive", "show", "generated")
        assert len(generated) == 37 and set(generated) <= set(b"ab")
        mapped = b"mapped fixture\nvalue\n"
        _ = run(source, "add", "env", "--stdin", data=mapped)
        _ = run(source, "move", "binary", "renamed")
        revision = ""
        if not no_git:
            revision = (
                raw(source, "--non-interactive", "git", "--", "rev-parse", "HEAD")
                .decode()
                .strip()
            )
            visited.add("git")
            _ = run(source, "history", "list", "renamed")
            _ = run(source, "history", "show", revision)
        edit = run(source, "edit", "renamed", "--stdin", data=b"changed\x00\xfe")
        snapshot = edit["transaction"]
        assert isinstance(snapshot, str)
        _ = run(
            source, "remove", "renamed", *(["--permanent-delete"] if no_git else [])
        )
        _ = run(source, "backup", "show", snapshot)
        _ = run(source, "backup", "restore", snapshot, "--phase", "before", "--yes")
        exact(source, "renamed", original)
        exact(source, "empty", b"")
        if not no_git:
            _ = run(source, "history", "restore", revision, "renamed", "--yes")
            exact(source, "renamed", original)

        # Some runtimes add environment keys themselves (macOS Python adds
        # LC_CTYPE and __CF_USER_TEXT_ENCODING). A direct clean launch isolates
        # those additions from Fulla's environment selection.
        runtime_keys = (
            subprocess.check_output(
                [
                    sys.executable,
                    "-c",
                    "import json,os; print(json.dumps(sorted(os.environ)))",
                ],
                env={"TOKEN": "fixture"},
                timeout=30,
            )
            .decode()
            .strip()
        )
        child_input = b"native stdin fixture\x00\xff"
        child_code = (
            "import hashlib,json,os,sys; "
            "assert set(os.environ)==set(json.loads(sys.argv[2])); "
            "assert hashlib.sha256(os.environ['TOKEN'].encode()).hexdigest()==sys.argv[1]; "
            "assert hashlib.sha256(sys.stdin.buffer.read()).hexdigest()==sys.argv[3]; "
            "assert os.getcwd()==sys.argv[4] and sys.argv[5]=='--json'; "
            "assert os.getpgrp()==os.getpid() and os.getsid(0)==os.getpid(); "
            "sys.stderr.write('native stderr fixture\\n'); "
            "print(json.dumps({'pid':os.getpid(),'accepted':True})); sys.exit(23)"
        )
        child = subprocess.Popen(
            [
                binary,
                "--store",
                str(source),
                "--non-interactive",
                "run",
                "--clean-env",
                "--env",
                "TOKEN=env",
                "--",
                sys.executable,
                "-c",
                child_code,
                hashlib.sha256(mapped).hexdigest(),
                runtime_keys,
                hashlib.sha256(child_input).hexdigest(),
                str(home),
                "--json",
            ],
            cwd=home,
            env=env,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            start_new_session=True,
        )
        try:
            stdout, stderr = child.communicate(child_input, timeout=30)
        finally:
            if child.poll() is None:
                child.kill()
                _ = child.wait(timeout=10)
        assert child.returncode == 23 and stderr == b"native stderr fixture\n", stderr
        executed = obj(stdout)
        assert executed["pid"] == child.pid and executed["accepted"] is True
        visited.add("run")

        _ = run(recovery, "init", "--no-git", "--yes")
        recipient = run(recovery, "identity", "show")["recipient"]
        assert isinstance(recipient, str)
        identity = str(recovery / "identities")
        manifest = home / "manifest.json"
        _ = manifest.write_text(json.dumps(["empty", "renamed"]))
        bundle = home / "selected.age"
        exported = run(
            source,
            "transfer",
            "export",
            "--manifest",
            str(manifest),
            "--recipient",
            recipient,
            "--output",
            str(bundle),
        )
        assert sorted(cast(list[str], exported["names"])) == ["empty", "renamed"]
        isolated = home / "never-created"
        verified = run(
            isolated, "transfer", "verify", str(bundle), "--identity", identity
        )
        assert (
            sorted(cast(list[str], verified["names"])) == ["empty", "renamed"]
            and not isolated.exists()
        )
        _ = run(target, "init", "--no-git", "--yes")
        _ = run(target, "add", "empty", "--stdin", data=b"keep local")
        imported = run(
            target, "transfer", "import", str(bundle), "--identity", identity
        )
        assert imported["names"] == ["renamed"] and imported["skipped"] == ["empty"]
        exact(target, "renamed", original)
        exact(target, "empty", b"keep local")
        assert run(target, "list")["names"] == ["empty", "renamed"]

        before_identity = run(source, "identity", "show")["fingerprint"]
        rotated = run(source, "identity", "rotate", "--yes")
        assert (
            rotated["old_fingerprint"] == before_identity
            and rotated["new_fingerprint"] != before_identity
        )
        _ = run(source, "backup", "restore", snapshot, "--phase", "before", "--yes")
        exact(source, "renamed", original)
        full = home / "full.age"
        before_archive = tree(source)
        full_result = run(
            source,
            "backup",
            "export",
            "--full",
            "--recipient",
            recipient,
            "--output",
            str(full),
        )
        after_archive = tree(source)
        assert all(
            after_archive.get(name) == value for name, value in before_archive.items()
        )
        added_receipts = set(after_archive) - set(before_archive)
        assert len(added_receipts) == 1
        receipt_name = added_receipts.pop()
        assert receipt_name.startswith(".fulla/receipts/") and receipt_name.endswith(
            ".json"
        )
        receipt = obj((source / receipt_name).read_bytes())
        assert receipt["command"] == "backup export" and receipt["applied"] is True
        assert receipt["full"] is True and receipt["files"] == full_result["files"]
        # The archive captures the state before its own publication receipt.
        restored = home / "restored"
        _ = run(
            restored,
            "backup",
            "restore",
            str(full),
            "--full",
            "--identity",
            identity,
            "--yes",
        )
        # Archive restoration normalizes private modes, including Git's 0400
        # loose objects, to the documented 0600 files / 0700 directories.
        expected_restore = {
            name: (stat.S_IFMT(mode) | (0o700 if stat.S_ISDIR(mode) else 0o600), digest)
            for name, (mode, digest) in before_archive.items()
        }
        assert tree(restored) == expected_restore
        exact(restored, "renamed", original)
        _ = run(restored, "doctor", "--deep")
        before_prune = tree(source)
        preview = run(source, "backup", "prune", "--keep", "1")
        assert (
            preview["dry_run"] is True
            and preview["candidates"]
            and tree(source) == before_prune
        )
        applied = run(source, "backup", "prune", "--keep", "1", "--yes")
        assert applied["dry_run"] is False and applied["deleted"]
        backups = run(source, "backup", "list")["backups"]
        assert isinstance(backups, list) and len(cast(list[object], backups)) == 1
        exact(source, "renamed", original)
        _ = run(source, "status")
        _ = run(source, "doctor", "--deep")
        _ = run(source, "version")
        assert b"complete -F" in raw(source, "--non-interactive", "completion", "bash")
        visited.add("completion")
        expected_commands = {
            "init",
            "add",
            "show",
            "edit",
            "move",
            "remove",
            "list",
            "run",
            "backup list",
            "backup show",
            "backup restore",
            "backup prune",
            "backup export",
            "identity show",
            "identity rotate",
            "transfer export",
            "transfer verify",
            "transfer import",
            "status",
            "doctor",
            "version",
            "completion",
        }
        if not no_git:
            expected_commands |= {
                "git",
                "history list",
                "history show",
                "history restore",
            }
        assert visited == expected_commands
        print(
            json.dumps(
                {"no_git": no_git, "accepted": True, "commands": sorted(visited)}
            )
        )


for no_git in (False, True):
    acceptance(no_git)
