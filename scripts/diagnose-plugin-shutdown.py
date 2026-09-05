"""Bounded reproducer for age plugin shutdown; synthetic disposable store only."""

import json
import os
import signal
import subprocess
import sys
import tempfile
import time
from pathlib import Path
from typing import cast

binary, key_fixture = (str(Path(arg).resolve()) for arg in sys.argv[1:])
with tempfile.TemporaryDirectory(prefix="fulla-plugin-shutdown-") as temporary:
    home = Path(temporary).resolve()
    store = home / "store"
    marker = home / "interrupted"
    env = {
        "HOME": str(home),
        "TMPDIR": str(home),
        "PATH": str(home) + ":/usr/bin:/bin",
        "XDG_CONFIG_HOME": str(home / "config"),
    }
    _ = subprocess.run(
        [binary, "--store", str(store), "--json", "init", "--no-git", "--yes"],
        env=env,
        input=b"",
        capture_output=True,
        timeout=30,
        check=True,
    )
    keys = cast(
        dict[str, object],
        json.loads(
            subprocess.check_output([key_fixture, "fixture-keys"], env=env, timeout=30)
        ),
    )
    recipient = keys["recipient"]
    assert isinstance(recipient, str)
    _ = (store / "recipients").write_text(recipient + "\n")
    plugin = home / "age-plugin-fullafixture"
    _ = plugin.write_text(
        f"#!{sys.executable}\n"
        + "import os, signal, time\n"
        + f"signal.signal(signal.SIGINT, lambda *_: open({str(marker)!r}, 'w').close())\n"
        + "os.write(1, b'invalid age plugin stanza\\n')\n"
        + "os.close(1)\n"
        + "while True: time.sleep(1)\n"
    )
    plugin.chmod(0o700)
    child = subprocess.Popen(
        [binary, "--store", str(store), "--json", "add", "fixture", "--stdin"],
        env=env,
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        start_new_session=True,
    )
    try:
        try:
            _ = child.communicate(b"synthetic fixture value", timeout=3)
        except subprocess.TimeoutExpired:
            pass
        # SIGINT receipt proves age reached Close; mere operation slowness does not.
        deadline = time.monotonic() + 5
        while (
            not marker.exists() and child.poll() is None and time.monotonic() < deadline
        ):
            time.sleep(0.05)
        received_interrupt = marker.exists()
        if received_interrupt:
            time.sleep(1)
        hung_after_interrupt = received_interrupt and child.poll() is None
        print(
            json.dumps(
                {
                    "plugin_received_sigint": received_interrupt,
                    "operation_still_running_after_sigint": hung_after_interrupt,
                    "known_shutdown_gap_reproduced": hung_after_interrupt,
                }
            )
        )
        if not received_interrupt:
            raise RuntimeError("fixture did not reach plugin shutdown")
    finally:
        # Only the process group created by this fixture; includes the plugin.
        try:
            os.killpg(child.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        _ = child.communicate(timeout=10)
