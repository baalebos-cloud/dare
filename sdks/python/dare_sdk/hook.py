"""Optional auto-hook: installs a sys.excepthook that diagnoses any
uncaught exception before the process exits, then prints the diagnosis
and re-raises the original traceback so normal tooling still sees it.
"""
from __future__ import annotations

import sys
from typing import Callable

from .client import CoreNotFoundError, diagnose

_original_excepthook: Callable = sys.excepthook
_installed = False


def _hook(exc_type, exc_value, exc_tb):
    try:
        result = diagnose(exc_value)
        print(f"\n✖ dare diagnosis (via {result.provider}):", file=sys.stderr)
        print(f"  {result.summary}", file=sys.stderr)
        if result.suggested_fix:
            print(f"\n  Suggested fix:\n  {result.suggested_fix}", file=sys.stderr)
    except CoreNotFoundError as e:
        print(f"\n(dare: {e})", file=sys.stderr)
    except Exception:
        # Never let the diagnosis path itself crash the program harder
        # than the original exception already did.
        pass

    _original_excepthook(exc_type, exc_value, exc_tb)


def install() -> None:
    """Install the global uncaught-exception hook. Idempotent."""
    global _installed
    if _installed:
        return
    sys.excepthook = _hook
    _installed = True


def uninstall() -> None:
    global _installed
    sys.excepthook = _original_excepthook
    _installed = False
