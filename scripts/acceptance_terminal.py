"""Controlling-terminal driver shared by isolated Fulla acceptance journeys."""

import errno
import os
import pty
import select
import signal
import termios
import time
from collections.abc import Sequence
from pathlib import Path
from typing import cast


def terminal(
    binary: str,
    env: dict[str, str],
    args: list[str],
    actions: Sequence[tuple[bytes, bytes | int]],
    target: Path,
    *,
    stdin_tty: bool = False,
) -> tuple[int, bytes]:
    pid, fd = pty.fork()
    if pid == 0:
        if not stdin_tty:
            # Prompts and hidden input must use /dev/tty, not standard input.
            null = os.open(os.devnull, os.O_RDONLY)
            _ = os.dup2(null, 0)
            os.close(null)
        command = [binary, "--store", str(target)]
        os.execve(binary, command + args, env)
    output = bytearray()
    pending = list(actions)
    consumed = 0
    deadline = time.monotonic() + 15
    result = None
    try:
        while time.monotonic() < deadline:
            if pending and pending[0][0] in output[consumed:]:
                marker, response = pending.pop(0)
                consumed = output.index(marker, consumed) + len(marker)
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
