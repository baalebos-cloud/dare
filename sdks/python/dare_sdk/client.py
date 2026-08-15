"""Talks to the dare core binary (or its daemon socket) to get a
diagnosis for a captured exception. Kept deliberately thin: all provider
routing/fallback logic lives in the Go core, not duplicated here.
"""
from __future__ import annotations

import json
import os
import platform
import shutil
import subprocess
import traceback
from dataclasses import dataclass


@dataclass
class Diagnosis:
    summary: str
    suggested_fix: str
    provider: str
    raw: str


class CoreNotFoundError(RuntimeError):
    """Raised when the `dare` binary isn't on PATH."""


def _find_core_binary() -> str:
    path = shutil.which("dare")
    if not path:
        raise CoreNotFoundError(
            "The `dare` core binary was not found on PATH. "
            "Install it: curl -fsSL https://dare.dev/install.sh | sh"
        )
    return path


def _build_payload(exc: BaseException) -> str:
    tb_text = "".join(traceback.format_exception(type(exc), exc, exc.__traceback__))
    payload = {
        "language": "python",
        "runtime_version": platform.python_version(),
        "os": platform.system(),
        "traceback": tb_text,
    }
    return json.dumps(payload)


def diagnose(exc: BaseException, timeout: float = 30.0) -> Diagnosis:
    """Send a caught exception's traceback to the core CLI and return
    a structured diagnosis. Raises CoreNotFoundError if the binary is
    missing rather than failing silently.
    """
    binary = _find_core_binary()
    payload = _build_payload(exc)

    # The core binary reads structured JSON from stdin when invoked with
    # --json, reusing the exact same router/provider logic as pipe mode.
    result = subprocess.run(
        [binary, "--json"],
        input=payload,
        capture_output=True,
        text=True,
        timeout=timeout,
    )

    if result.returncode not in (0, 2):
        raise RuntimeError(f"dare core exited unexpectedly: {result.stderr}")

    try:
        data = json.loads(result.stdout)
    except json.JSONDecodeError:
        # Core printed human-readable text (e.g. no --json support yet in
        # this PoC) — surface it directly rather than crashing the SDK.
        return Diagnosis(summary=result.stdout.strip(), suggested_fix="", provider="unknown", raw=result.stdout)

    return Diagnosis(
        summary=data.get("summary", ""),
        suggested_fix=data.get("suggested_fix", ""),
        provider=data.get("provider", "unknown"),
        raw=result.stdout,
    )
