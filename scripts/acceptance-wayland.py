"""Own a disposable headless compositor for real Wayland clipboard acceptance."""

import os
import signal
import subprocess
import sys
import tempfile
import time
from pathlib import Path

if sys.platform != "linux" or os.environ.get("GITHUB_ACTIONS") != "true":
    raise RuntimeError(
        "headless clipboard acceptance requires a disposable Linux runner"
    )

binary = str(Path(sys.argv[1]).resolve())
harness = str(Path(__file__).with_name("acceptance-clipboard.py").resolve())
_ = os.umask(0o077)
with tempfile.TemporaryDirectory(prefix="fulla-wayland-") as temporary:
    home = Path(temporary)
    runtime = home / "runtime"
    runtime.mkdir(mode=0o700)
    config = home / "sway.conf"
    _ = config.write_text("xwayland disable\nseat seat0 fallback true\n")
    env = {
        "PATH": os.environ["PATH"],
        "HOME": str(home),
        "XDG_RUNTIME_DIR": str(runtime),
        "WLR_BACKENDS": "headless",
        "WLR_RENDERER": "pixman",
        "WLR_LIBINPUT_NO_DEVICES": "1",
        "GITHUB_ACTIONS": "true",
    }
    # No desktop or ambient socket variables are inherited. The compositor and
    # its descendants belong to a fresh process group and private runtime tree.
    with (home / "sway.log").open("wb") as log:
        process = subprocess.Popen(
            ["sway", "--config", str(config)],
            env=env,
            stdin=subprocess.DEVNULL,
            stdout=log,
            stderr=log,
            start_new_session=True,
        )
        try:
            deadline = time.monotonic() + 15
            while True:
                if process.poll() is not None:
                    raise RuntimeError("headless compositor exited before readiness")
                sockets = [p for p in runtime.glob("wayland-*") if p.is_socket()]
                if len(sockets) == 1:
                    env["WAYLAND_DISPLAY"] = sockets[0].name
                    break
                if time.monotonic() >= deadline:
                    raise RuntimeError("headless compositor socket deadline exceeded")
                time.sleep(0.05)
            _ = subprocess.run(
                [sys.executable, harness, binary, "--real-clipboard", "--wayland"],
                env=env,
                check=True,
                timeout=120,
            )
        except BaseException:
            # Only compositor diagnostics, never clipboard payloads.
            _ = sys.stderr.write((home / "sway.log").read_text(errors="replace"))
            raise
        finally:
            if process.poll() is None:
                os.killpg(process.pid, signal.SIGTERM)
                try:
                    _ = process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    os.killpg(process.pid, signal.SIGKILL)
                    _ = process.wait(timeout=5)
