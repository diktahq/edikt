"""edikt gov-benchmark — tier-2 Python helper for /edikt:gov:benchmark.

This package is installed by `edikt install benchmark` (never by install.sh).
It is the SDK-touching half of a two-part system:

  - commands/gov/benchmark.md — pure-markdown tier-1 command surface
    (Claude Code orchestrates the flow, reads directives, renders reports).
  - tools/gov-benchmark/      — tier-2 Python helper (this package).
    Invokes the Claude Agent SDK to execute attack prompts against the
    user's configured model, handles SIGINT cleanly, scores responses.

Parity between the two halves is enforced by integration tests, NEVER by
shared code crossing the tier-1/tier-2 boundary (ref: ADR-015).

Modules:
  run      — main entry point; JSON stdin → JSON stdout; SIGINT handling.
  sandbox  — build_project(): byte-equal counterpart to
             test/integration/benchmarks/runner.py::build_project (AC-010).
  scoring  — score_case(): behavioral-signal verdict logic matching
             the command's Phase C §4 scoring contract.
"""

__version__ = "0.6.0"
