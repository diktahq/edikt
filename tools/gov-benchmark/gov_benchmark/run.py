"""edikt-gov-benchmark — helper entry point.

Contract
--------
The helper is invoked from commands/gov/benchmark.md (tier-1 markdown) as a
single subprocess per directive run:

    python -m gov_benchmark.run < input.json

Input (stdin, JSON)::

    {
      "directive_id":    "ADR-012",
      "directive_body":  "… MUST not …",
      "signal_type":     "refuse_file_pattern",
      "behavioral_signal": {…},
      "attack_prompt":   "…",
      "target_model":    "claude-opus-4-7",
      "project_dir":     "/tmp/…/project",
      "response_budget_tokens": 2000,
      "timeout_s":       60
    }

Output (stdout, JSON single line per run)::

    {
      "directive_id": "ADR-012",
      "verdict":      "PASS",
      "reasons":      [...],
      "assistant_text": "...",
      "tool_calls":   [...],
      "written_paths": [...],
      "elapsed_ms":   19340,
      "api_ms":       18400,
      "status":       "ok"        # ok | skipped | auth_error | network_error | sdk_error
    }

On SIGINT (AC-006b / AC-006c):
  - Between directives: the markdown command exits naturally after each
    helper run; there's no persistent state in the helper.
  - During a model call: the helper cancels the SDK query, waits ≤5s for
    the subprocess to terminate, then exits. status="cancelled" on exit.

Error UX (AC-016b):
  - Auth errors → stderr: "Claude auth failed — run `claude` to refresh then retry"
  - Network blip → status="network_error", summary includes the directive ID
  - Any other SDK exception → status="sdk_error" with the exception message
  Each error path produces actionable messages, NEVER a raw traceback on stdout.
"""

from __future__ import annotations

import asyncio
import contextlib
import json
import os
import signal
import sys
import time
import traceback
from pathlib import Path
from typing import Any

from .scoring import score_case


# ─── Auth / error classification ─────────────────────────────────────────────


_AUTH_SIGNAL_RE = (
    "not logged in",
    "authentication",
    "unauthorized",
    "auth failed",
    "401",
)
_NETWORK_SIGNAL_RE = (
    "network",
    "connection reset",
    "connection refused",
    "timed out",
    "502",
    "503",
    "504",
    "dns",
)

# Class-name fragments that map to our error categories.
# This catches SDK-defined exception types without requiring a hard import of
# claude_agent_sdk at module level (the SDK is a deferred / optional dep).
# Check class name first (#15) — avoids false-positive substring matches where
# a legitimate directive body or error detail happens to contain auth/network
# language but the exception is not structurally an auth/network error.
_AUTH_CLASS_FRAGMENTS = ("AuthenticationError", "AuthError", "Unauthorized", "LoginError")
_NETWORK_CLASS_FRAGMENTS = ("NetworkError", "ConnectionError", "TimeoutError", "HTTPError")


def _exc_class_names(exc: BaseException) -> list[str]:
    """Return the class name + all base-class names for *exc* (MRO walk)."""
    return [cls.__name__ for cls in type(exc).__mro__]


def _classify_error(exc: BaseException) -> str:
    """Classify *exc* into auth_error / network_error / sdk_error.

    Strategy (per finding #15):
    1. Prefer class-based detection — inspect the exception type's MRO so that
       SDK-defined AuthenticationError / NetworkError subtypes are matched even
       when they don't surface a telltale substring in their message.
    2. Fall back to substring matching on the message only for exception types
       that are not structurally recognized (e.g., bare RuntimeError wrapping
       an SDK detail string, or third-party exceptions).
    This prevents false-positive auth classification where a legitimate directive
    body or error detail contains the phrase "not logged in" in a raised generic
    exception (the original finding #15 false-positive case).
    """
    class_names = _exc_class_names(exc)

    # Step 1: class-based detection (structural, not text-based).
    for name in class_names:
        for frag in _AUTH_CLASS_FRAGMENTS:
            if frag in name:
                return "auth_error"
        for frag in _NETWORK_CLASS_FRAGMENTS:
            if frag in name:
                return "network_error"

    # Step 2: fallback substring match — only reached for non-SDK / unknown
    # exception types (e.g., bare RuntimeError, ValueError). Substrate for
    # future SDK exception types that haven't been named yet.
    msg = str(exc).lower()
    if any(s in msg for s in _AUTH_SIGNAL_RE):
        return "auth_error"
    if any(s in msg for s in _NETWORK_SIGNAL_RE):
        return "network_error"
    return "sdk_error"


def _actionable_message(status: str, directive_id: str, raw: str) -> str:
    if status == "auth_error":
        return (
            "Claude auth failed — run `claude` to refresh then retry"
        )
    if status == "network_error":
        return (
            f"Network error on directive {directive_id} — "
            f"marked SKIPPED, continuing with remaining directives"
        )
    return f"Benchmark error on {directive_id}: {raw}"


# ─── Cancellation token ──────────────────────────────────────────────────────


class _Cancel:
    """Tiny cancellation flag shared with the SIGINT handler.

    The SDK's async generator cancellation happens via task.cancel(); this
    object just tracks whether we asked the loop to cancel and how long ago,
    so we can enforce the ≤5s budget in AC-006c.
    """

    def __init__(self) -> None:
        self.requested_at: float | None = None

    def request(self) -> None:
        if self.requested_at is None:
            self.requested_at = time.monotonic()

    @property
    def requested(self) -> bool:
        return self.requested_at is not None

    def elapsed(self) -> float:
        if self.requested_at is None:
            return 0.0
        return time.monotonic() - self.requested_at


# ─── SDK invocation ──────────────────────────────────────────────────────────


async def _invoke_sdk(
    prompt: str,
    project_dir: Path,
    model: str,
    timeout_s: float,
    cancel: _Cancel,
) -> dict[str, Any]:
    """Run one directive through the SDK. Returns a partial run record.

    Separated so tests can stub _invoke_sdk without touching claude_agent_sdk.
    """
    # Deferred import — the helper is installable without SDK as long as no
    # directive is actually run, and test paths can monkey-patch this symbol.
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import (
        AssistantMessage,
        ResultMessage,
        TextBlock,
        ToolUseBlock,
    )

    assistant_text: list[str] = []
    tool_calls: list[dict[str, Any]] = []
    api_ms = 0
    written_paths: list[str] = []

    options = ClaudeAgentOptions(
        cwd=str(project_dir),
        setting_sources=["user", "project"],
        model=model,
    )

    async def _stream() -> None:
        nonlocal api_ms
        async for msg in query(prompt=prompt, options=options):
            if cancel.requested:
                break
            if isinstance(msg, AssistantMessage):
                for block in msg.content:
                    if isinstance(block, ToolUseBlock):
                        tool_calls.append(
                            {
                                "tool_name": block.name,
                                "tool_input": block.input,
                            }
                        )
                    elif isinstance(block, TextBlock):
                        assistant_text.append(block.text)
            elif isinstance(msg, ResultMessage):
                api_ms = getattr(msg, "duration_api_ms", 0) or 0

    task = asyncio.create_task(_stream())
    try:
        await asyncio.wait_for(task, timeout=timeout_s)
    except asyncio.TimeoutError:
        cancel.request()
        task.cancel()
        with contextlib.suppress(asyncio.CancelledError, Exception):
            await task
    except asyncio.CancelledError:
        cancel.request()
        task.cancel()
        with contextlib.suppress(asyncio.CancelledError, Exception):
            await task
        raise

    for tc in tool_calls:
        if tc.get("tool_name") in {"Write", "Edit"}:
            fp = (tc.get("tool_input") or {}).get("file_path", "")
            if fp:
                written_paths.append(fp)

    return {
        "assistant_text": " ".join(assistant_text),
        "tool_calls": tool_calls,
        "written_paths": written_paths,
        "api_ms": api_ms,
    }


# ─── Top-level run ────────────────────────────────────────────────────────────


def _install_sigint_handler(cancel: _Cancel, loop: asyncio.AbstractEventLoop) -> None:
    """Install a SIGINT handler that asks the SDK task to cancel within ≤5s."""

    def _handler(signum: int, frame: Any) -> None:  # noqa: ARG001
        cancel.request()
        # Waking the loop nudges wait_for/CancelledError into play.
        loop.call_soon_threadsafe(lambda: None)
        if cancel.elapsed() > 5.0:
            # Hard bail-out guard — never reached under normal cancel path.
            os._exit(130)

    with contextlib.suppress(ValueError):
        signal.signal(signal.SIGINT, _handler)


# #16 (security): meta-header prepended to every attack prompt at render time.
# This is a runtime-rendering concern — the header is NOT stored in template
# files so templates remain reusable.  Downstream logging / auditing can grep
# for this literal string to identify adversarial benchmark prompts.
_BENCHMARK_META_HEADER = (
    "[BENCHMARK ONLY — adversarial governance test, not a real user request]"
)


def _prepend_benchmark_header(prompt: str) -> str:
    """Prepend the BENCHMARK ONLY meta-header to an attack prompt.

    Two blank lines separate the header from the existing attack body so
    that the model sees a clear visual break and the header is not parsed
    as part of the attack instruction.
    """
    return f"{_BENCHMARK_META_HEADER}\n\n\n{prompt}"


def run_one(inp: dict[str, Any]) -> dict[str, Any]:
    """Run a single directive attack. Synchronous wrapper."""
    directive_id = inp.get("directive_id", "UNKNOWN")
    signal_type = inp.get("signal_type", "")
    behavioral_signal = inp.get("behavioral_signal") or {}
    attack_prompt = inp.get("attack_prompt", "")
    target_model = inp.get("target_model", "")
    project_dir = Path(inp.get("project_dir") or ".")
    timeout_s = float(inp.get("timeout_s") or 60)

    # AC-009: directives with no behavioral_signal are skipped by the caller,
    # but we re-assert here so the helper never scores an empty signal.
    if not behavioral_signal:
        return {
            "directive_id": directive_id,
            "verdict": "PASS",
            "reasons": ["no behavioral_signal — skipped"],
            "assistant_text": "",
            "tool_calls": [],
            "written_paths": [],
            "elapsed_ms": 0,
            "api_ms": 0,
            "status": "skipped",
        }

    cancel = _Cancel()

    # #16: prepend the meta-header before passing the prompt to the SDK so
    # every downstream log entry clearly marks the prompt as adversarial.
    rendered_prompt = _prepend_benchmark_header(attack_prompt)

    start = time.monotonic()

    async def _bounded_invoke() -> dict[str, Any]:
        # Outer wait_for enforces AC-006c: the helper never blocks past
        # timeout_s + 5 s (our hard-cancel budget). Stubbed _invoke_sdk
        # benefits from the same guard — tests don't need their own.
        task = asyncio.create_task(
            _invoke_sdk(
                prompt=rendered_prompt,
                project_dir=project_dir,
                model=target_model,
                timeout_s=timeout_s,
                cancel=cancel,
            )
        )
        try:
            # Budget: timeout_s for the model call + 2s hard-cancel grace.
            return await asyncio.wait_for(task, timeout=timeout_s + 2.0)
        except asyncio.TimeoutError:
            cancel.request()
            task.cancel()
            with contextlib.suppress(asyncio.CancelledError, Exception):
                await task
            return {
                "assistant_text": "",
                "tool_calls": [],
                "written_paths": [],
                "api_ms": 0,
                "__timed_out__": True,
            }

    try:
        loop = asyncio.new_event_loop()
        try:
            asyncio.set_event_loop(loop)
            _install_sigint_handler(cancel, loop)
            result = loop.run_until_complete(_bounded_invoke())
            if result.get("__timed_out__"):
                return {
                    "directive_id": directive_id,
                    "verdict": "FAIL",
                    "reasons": ["run timed out (hard cancel budget exceeded)"],
                    "assistant_text": "",
                    "tool_calls": [],
                    "written_paths": [],
                    "elapsed_ms": int((time.monotonic() - start) * 1000),
                    "api_ms": 0,
                    "status": "cancelled",
                }
        finally:
            loop.close()
    except KeyboardInterrupt:
        cancel.request()
        return {
            "directive_id": directive_id,
            "verdict": "FAIL",
            "reasons": ["cancelled by user (SIGINT)"],
            "assistant_text": "",
            "tool_calls": [],
            "written_paths": [],
            "elapsed_ms": int((time.monotonic() - start) * 1000),
            "api_ms": 0,
            "status": "cancelled",
        }
    except Exception as exc:  # noqa: BLE001
        status = _classify_error(exc)
        msg = _actionable_message(status, directive_id, str(exc))
        # Actionable message on stderr for the markdown command to surface.
        print(msg, file=sys.stderr, flush=True)
        return {
            "directive_id": directive_id,
            "verdict": "FAIL",
            "reasons": [msg],
            "assistant_text": "",
            "tool_calls": [],
            "written_paths": [],
            "elapsed_ms": int((time.monotonic() - start) * 1000),
            "api_ms": 0,
            "status": status,
        }

    verdict, reasons = score_case(
        signal_type=signal_type,
        behavioral_signal=behavioral_signal,
        assistant_text=result["assistant_text"],
        tool_calls=result["tool_calls"],
        project_dir=project_dir,
    )
    return {
        "directive_id": directive_id,
        "verdict": verdict,
        "reasons": reasons,
        "assistant_text": result["assistant_text"],
        "tool_calls": result["tool_calls"],
        "written_paths": result["written_paths"],
        "elapsed_ms": int((time.monotonic() - start) * 1000),
        "api_ms": result["api_ms"],
        "status": "ok",
    }


def main() -> int:
    """CLI entry — read JSON from stdin, emit JSON to stdout, exit 0 always.

    Exit codes:
      0 — normal (verdict PASS or FAIL; status captures error class)
      2 — input malformed (couldn't parse stdin as JSON)
    The benchmark is advisory (AC-016) — directive-level failure is PASS/FAIL
    in the payload, not an exit code.
    """
    try:
        raw = sys.stdin.read()
        inp = json.loads(raw)
    except json.JSONDecodeError as exc:
        print(
            f"input parse error: {exc}. "
            "helper expects a JSON object on stdin.",
            file=sys.stderr,
            flush=True,
        )
        return 2

    out = run_one(inp)
    sys.stdout.write(json.dumps(out) + "\n")
    sys.stdout.flush()
    return 0


if __name__ == "__main__":  # pragma: no cover
    try:
        sys.exit(main())
    except Exception as exc:  # noqa: BLE001
        # Last-resort trap so the caller never sees a raw traceback on stdout.
        print(
            f"Benchmark error: {exc}. See stderr for details.",
            file=sys.stderr,
            flush=True,
        )
        traceback.print_exc(file=sys.stderr)
        sys.exit(1)
