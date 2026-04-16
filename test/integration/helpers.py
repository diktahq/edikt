"""Shared helpers for Layer 2 SDK integration tests.

Extracted from conftest.py so tests can import by a stable, unambiguous name.
`from conftest import with_retry` breaks when pytest adds project-mode/ to
sys.path (testpaths collision) — `from helpers import with_retry` is stable.
"""

from __future__ import annotations

import asyncio
import json
import random
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Callable, Coroutine

import pytest

_HERE = Path(__file__).parent
FAILURES_DIR = _HERE / "failures"


def _is_5xx(exc: Exception) -> bool:
    """Return True if exc looks like an Anthropic API 5xx error."""
    msg = str(exc).lower()
    return "500" in msg or "502" in msg or "503" in msg or "504" in msg or "overloaded" in msg


def _log_outage(exc: Exception) -> None:
    FAILURES_DIR.mkdir(parents=True, exist_ok=True)
    log_path = FAILURES_DIR / "outages.jsonl"
    record = {
        "at": datetime.now(timezone.utc).isoformat(),
        "error": str(exc),
    }
    with log_path.open("a") as fh:
        fh.write(json.dumps(record) + "\n")


async def with_retry(
    func: Callable[[], Coroutine[Any, Any, Any]],
    attempts: int = 3,
    *,
    skip_on_outage: bool = False,
) -> Any:
    """Jittered exponential backoff wrapper for SDK query() calls.

    Attempt 1: immediate.
    Attempt 2: 1s base + U(0, 1)s jitter.
    Attempt 3: 2s base + U(0, 2)s jitter.

    If ``skip_on_outage`` is True and the final attempt raises a 5xx-like
    error, writes an outage event and calls pytest.skip() instead of re-raising.
    """
    base_delays = [0.0, 1.0, 2.0]
    last_exc: Exception | None = None

    for i in range(attempts):
        try:
            return await func()
        except Exception as exc:  # noqa: BLE001
            last_exc = exc
            is_5xx = _is_5xx(exc)

            if i < attempts - 1:
                base = base_delays[min(i + 1, len(base_delays) - 1)]
                jitter = random.uniform(0, base)
                await asyncio.sleep(base + jitter)
            elif is_5xx and skip_on_outage:
                _log_outage(exc)
                pytest.skip(f"Upstream outage after {attempts} retries — {exc}")

    assert last_exc is not None
    raise last_exc
