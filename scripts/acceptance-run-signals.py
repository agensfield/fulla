"""Strict inherited-signal parity gate against direct exec; currently exposes a gap."""

import hashlib
import json
import os
import signal
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import cast

binary = str(Path(sys.argv[1]).resolve())
_ = os.umask(0o077)
launcher = """import os, signal, sys
mode, name = sys.argv[1:3]
sig = getattr(signal, name)
if mode in ('blocked', 'pending'):
    signal.pthread_sigmask(signal.SIG_BLOCK, [sig])
    if mode == 'pending':
        os.kill(os.getpid(), sig)
else:
    signal.signal(sig, signal.SIG_IGN)
os.execv(sys.argv[3], sys.argv[3:])
"""
target = """import json, os, signal, sys
mode, name = sys.argv[1:3]
sig = getattr(signal, name)
if mode == 'blocked':
    preserved = sig in signal.pthread_sigmask(signal.SIG_BLOCK, [])
elif mode == 'pending':
    preserved = sig in signal.pthread_sigmask(signal.SIG_BLOCK, []) and sig in signal.sigpending()
else:
    preserved = signal.getsignal(sig) == signal.SIG_IGN
print(json.dumps({'preserved': preserved, 'pid': os.getpid()}))
"""
results: list[dict[str, object]] = []
with tempfile.TemporaryDirectory(prefix="fulla-run-signals-") as temporary:
    home = Path(temporary).resolve()
    store = home / "store"
    env = {
        "PATH": os.environ["PATH"],
        "HOME": str(home),
        "XDG_CONFIG_HOME": str(home / "config"),
    }
    base = [binary, "--store", str(store)]
    for args, data in [
        (["--json", "init", "--yes", "--no-git"], None),
        (["--json", "add", "token", "--stdin"], b"synthetic-signal-fixture"),
    ]:
        _ = subprocess.run(
            base + args, input=data, env=env, capture_output=True, check=True, timeout=20
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
    for mode, name in [
        ("blocked", "SIGUSR1"),
        ("blocked", "SIGTERM"),
        ("pending", "SIGTERM"),
        ("ignored", "SIGHUP"),
        ("ignored", "SIGINT"),
        ("ignored", "SIGTERM"),
    ]:
        assert hasattr(signal, name)
        observed: dict[str, bool] = {}
        exits: dict[str, int] = {}
        for route in ("direct", "fulla"):
            command = [sys.executable, "-c", target, mode, name]
            if route == "fulla":
                command = base + ["run", "--env", "TOKEN=token", "--"] + command
            process = subprocess.Popen(
                [sys.executable, "-c", launcher, mode, name] + command,
                env=env,
                stdin=subprocess.DEVNULL,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )
            try:
                out, err = process.communicate(timeout=20)
            except subprocess.TimeoutExpired:
                process.kill()
                _ = process.communicate()
                raise
            assert not err, (route, mode, name)
            exits[route] = process.returncode
            if process.returncode == 0:
                report = cast(dict[str, object], json.loads(out))
                assert report["pid"] == process.pid, "process image was not replaced"
                assert isinstance(report["preserved"], bool)
                observed[route] = report["preserved"]
            else:
                assert not out, "failed signal probe emitted unexpected output"
                observed[route] = False
        assert observed["direct"], "direct-exec control did not preserve fixture state"
        results.append({"mode": mode, "signal": name, **observed, "exit": exits})
    assert inventory() == before, "signal probes changed the store"
passed = all(row["fulla"] is True for row in results)
print(json.dumps({"native_signal_parity": passed, "cases": results, "store_unchanged": True}))
raise SystemExit(0 if passed else 1)
