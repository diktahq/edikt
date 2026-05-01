"""Benchmark runner — loads a corpus, runs each case against a model, scores compliance.

Shared infrastructure used by all benchmark test modules. Parameterizes
by model so the same corpus can be run against Opus 4.6, 4.7, Sonnet 4.6, etc.

Verify contract (per corpus entry):
  must_mention: list of substrings — all must appear in response/writes
  must_cite:    list of directive IDs — response must reference these
  must_not_write: list of path patterns — these must NOT be written
  must_not_do:  list of tool names — these tools must NOT be invoked
  must_do:      list of tool names — these tools must be invoked
  must_create:  list of path patterns — these paths MUST be written to
  result_matches: regex — final result text must match
"""

from __future__ import annotations

import asyncio
import json
import re
import unicodedata
import textwrap
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Literal

import pytest
import yaml

from helpers import with_retry

BENCHMARK_DIR = Path(__file__).parent
CORPUS_DIR = BENCHMARK_DIR / "corpus"
BASELINES_DIR = BENCHMARK_DIR / "baselines"

# Default model — can be overridden via --model CLI flag.
DEFAULT_MODEL = "claude-sonnet-4-6"

Severity = Literal["hard", "soft", "subtle", "override"]
Expected = Literal["refuse", "comply", "write_file", "describe_plan"]


# ─── Case types ──────────────────────────────────────────────────────────────


@dataclass
class Verify:
    must_mention: list[str] = field(default_factory=list)
    must_cite: list[str] = field(default_factory=list)
    must_not_write: list[str] = field(default_factory=list)
    must_not_do: list[str] = field(default_factory=list)
    must_do: list[str] = field(default_factory=list)
    must_create: list[str] = field(default_factory=list)
    result_matches: str | None = None


@dataclass
class Case:
    id: str
    dimension: str
    targets: list[str]
    severity: Severity
    prompt: str
    expected: Expected
    verify: Verify
    notes: str = ""
    project_setup: dict[str, Any] | None = None


@dataclass
class RunOutcome:
    """A single run of a single case."""
    run_index: int        # 0-indexed position within the N runs
    verdict: Literal["PASS", "FAIL", "UNCLEAR"]
    reasons: list[str]
    response: str         # full response text (preserved)
    tool_calls: list[dict]  # full tool call list (preserved)
    api_ms: int
    written_paths: list[str] = field(default_factory=list)


@dataclass
class CaseResult:
    """Aggregated result for a case across N runs."""
    case_id: str
    dimension: str
    severity: str
    targets: list[str]
    model: str
    runs: list[RunOutcome]           # every run preserved

    # Aggregate verdicts (computed from runs).
    n_runs: int = 0
    n_pass: int = 0
    n_fail: int = 0
    n_unclear: int = 0

    # Statistical aggregates.
    wilson_lower: float = 0.0
    wilson_upper: float = 1.0
    final_verdict: Literal["PASS", "FAIL", "UNCLEAR"] = "UNCLEAR"

    def to_summary(self) -> dict:
        """Compact summary for the session-level report."""
        return {
            "case_id": self.case_id,
            "dimension": self.dimension,
            "severity": self.severity,
            "targets": self.targets,
            "model": self.model,
            "n_runs": self.n_runs,
            "n_pass": self.n_pass,
            "n_fail": self.n_fail,
            "n_unclear": self.n_unclear,
            "wilson": {"lower": self.wilson_lower, "upper": self.wilson_upper},
            "verdict": self.final_verdict,
            "reasons_summary": [r for run in self.runs for r in run.reasons][:10],
        }


# ─── Corpus loading ──────────────────────────────────────────────────────────


def load_corpus(name: str) -> list[Case]:
    path = CORPUS_DIR / f"{name}.yaml"
    if not path.exists():
        return []
    raw = yaml.safe_load(path.read_text()) or []
    cases = []
    for entry in raw:
        verify_data = entry.get("verify", {}) or {}
        cases.append(Case(
            id=entry["id"],
            dimension=entry["dimension"],
            targets=entry.get("targets", []),
            severity=entry.get("severity", "hard"),
            prompt=entry["prompt"],
            expected=entry.get("expected", "refuse"),
            verify=Verify(**verify_data),
            notes=entry.get("notes", ""),
            project_setup=entry.get("project_setup"),
        ))
    return cases


# ─── Project scaffolding ──────────────────────────────────────────────────────


def build_project(tmp_path: Path, setup: dict[str, Any] | None) -> Path:
    """Build a test project from a case's project_setup block.

    This builder is the reference implementation for the benchmark sandbox.
    Edits here require a paired edit in commands/gov/benchmark.md (tier-2
    markdown command, Phase C §1 sandbox layout) AND in
    tools/gov-benchmark/sandbox.py::build_project (tier-2 helper). AC-010
    (SPEC-005) enforces byte-equal parity across four fixture shapes
    (sandbox-shape-minimal / realistic / mixed / edge) via
    test/integration/test_benchmark_sandbox_parity.py.

    Setup format (all optional):
      config: {...}           # .edikt/config.yaml contents
      files: {path: content}  # arbitrary project files
      adrs: [{...}]           # ADRs to pre-seed under docs/architecture/decisions/
      invariants: [{...}]     # INVs to pre-seed under docs/architecture/invariants/
      prd: {...}              # PRD to pre-seed under docs/product/prds/
      spec: {...}             # SPEC to pre-seed under docs/product/specs/
      plan: {...}             # PLAN to pre-seed under docs/plans/
    """
    project = tmp_path / "project"
    project.mkdir()
    setup = setup or {}

    # Baseline .edikt/config.yaml
    edikt_dir = project / ".edikt"
    edikt_dir.mkdir()
    default_config = {
        "edikt_version": "0.5.0",
        "base": "docs",
        "paths": {
            "decisions": "docs/architecture/decisions",
            "invariants": "docs/architecture/invariants",
            "plans": "docs/plans",
            "prds": "docs/product/prds",
            "specs": "docs/product/specs",
        },
    }
    cfg = {**default_config, **(setup.get("config") or {})}
    (edikt_dir / "config.yaml").write_text(yaml.dump(cfg))

    # Replicate a real edikt-installed project. The model should see the
    # same layout, rules, agents, settings, and CLAUDE.md a user would.
    import shutil
    import re as _re
    repo_root = Path(__file__).resolve().parents[3]

    # 1. Copy compiled rules (produced by /edikt:gov:compile).
    repo_rules = repo_root / ".claude" / "rules"
    if repo_rules.is_dir():
        shutil.copytree(repo_rules, project / ".claude" / "rules")

    # 2. Copy dogfooded agents.
    repo_agents = repo_root / ".claude" / "agents"
    if repo_agents.is_dir():
        shutil.copytree(repo_agents, project / ".claude" / "agents")

    # 3. Write a curated minimal settings.json into the sandbox (INV-007;
    #    closes audit HI-10). Previously the host's .claude/settings.json
    #    was copied verbatim — any maintainer-local hooks would fire against
    #    adversarial corpus prompts. The sandbox gets only the permissions
    #    needed to run the benchmark; NO `hooks` key.
    (project / ".claude").mkdir(exist_ok=True)
    _hermetic_settings = {
        "permissions": {
            "defaultMode": "askBeforeAllow",
            "allow": [
                "Read(**)", "Glob", "Grep", "Edit(**)", "Write(**)",
                "Bash(pytest :*)", "Bash(./test/run.sh)",
            ],
            "deny": [
                "WebFetch(http://**)", "Bash(curl http://**)",
                "Bash(rm -rf /**)", "Bash(rm -rf ~/**)", "Bash(sudo **)",
            ],
        },
    }
    import json as _json
    (project / ".claude" / "settings.json").write_text(
        _json.dumps(_hermetic_settings, indent=2) + "\n"
    )

    # 4. Real .edikt/config.yaml (overrides the synthetic one written above
    #    unless the case provides a config override).
    if not (setup.get("config")):
        real_cfg = repo_root / ".edikt" / "config.yaml"
        if real_cfg.is_file():
            shutil.copy2(real_cfg, edikt_dir / "config.yaml")

    # 5. Copy real ADRs and invariants so the model can read them when the
    #    routing table sends it to docs/architecture/.
    for src_rel, dst_rel in (
        ("docs/architecture/decisions", "docs/architecture/decisions"),
        ("docs/architecture/invariants", "docs/architecture/invariants"),
    ):
        src = repo_root / src_rel
        if src.is_dir():
            dst = project / dst_rel
            dst.parent.mkdir(parents=True, exist_ok=True)
            # symlinks=True: preserve symlinks as links, do NOT follow them.
            # LOW-8: a symlink planted under docs/architecture/ would otherwise
            # dereference to sensitive content (~/.ssh/, keychains) and be
            # accessible to the model inside the sandbox.
            shutil.copytree(src, dst, dirs_exist_ok=True, symlinks=True)

    # 6. CLAUDE.md with the real edikt sentinel block from the repo (intent
    #    table, "Before Writing Code", etc.) so the model gets the exact same
    #    context a real user's CLAUDE.md provides.
    repo_claudemd = (repo_root / "CLAUDE.md").read_text() if (repo_root / "CLAUDE.md").exists() else ""
    block_match = _re.search(
        r"\[edikt:start\].*?\[edikt:end\]: #",
        repo_claudemd, flags=_re.DOTALL,
    )
    edikt_block = block_match.group(0) if block_match else (
        "[edikt:start]: # managed by edikt\n## edikt\n[edikt:end]: #"
    )
    (project / "CLAUDE.md").write_text(
        f"# Project\n\nBenchmark test project for edikt governance compliance.\n\n{edikt_block}\n"
    )

    # Ensure governance directories exist.
    for sub in ("architecture/decisions", "architecture/invariants",
                "product/prds", "product/specs", "plans"):
        (project / "docs" / sub).mkdir(parents=True, exist_ok=True)

    # Pre-seeded files.
    for rel_path, content in (setup.get("files") or {}).items():
        target = project / rel_path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content)

    # Pre-seeded ADRs.
    for adr in setup.get("adrs") or []:
        path = project / "docs" / "architecture" / "decisions" / adr["filename"]
        path.write_text(adr["content"])

    # Pre-seeded invariants.
    for inv in setup.get("invariants") or []:
        path = project / "docs" / "architecture" / "invariants" / inv["filename"]
        path.write_text(inv["content"])

    # Pre-seeded PRD/spec/plan.
    for artifact in ("prd", "spec", "plan"):
        data = setup.get(artifact)
        if not data:
            continue
        if artifact == "spec":
            target_dir = project / "docs" / "product" / "specs" / data["folder"]
            target_dir.mkdir(parents=True, exist_ok=True)
            (target_dir / "spec.md").write_text(data["content"])
        elif artifact == "prd":
            (project / "docs" / "product" / "prds" / data["filename"]).write_text(data["content"])
        elif artifact == "plan":
            (project / "docs" / "plans" / data["filename"]).write_text(data["content"])
            if "sidecar" in data:
                sidecar_name = data["filename"].replace(".md", "-criteria.yaml")
                (project / "docs" / "plans" / sidecar_name).write_text(data["sidecar"])

    return project


# ─── Verification ─────────────────────────────────────────────────────────────


def score_case(
    case: Case,
    response_text: str,
    tool_calls: list[dict],
    written_content: str,
    project_dir: Path,
) -> tuple[Literal["PASS", "FAIL", "UNCLEAR"], list[str]]:
    """Check verify rules against the case output. Return verdict + reasons."""
    reasons: list[str] = []
    failed = False
    combined = response_text + " " + written_content

    v = case.verify

    # must_mention: each entry must be present. A string is a single
    # substring check; a list is a disjunction (any alternative matches).
    # Disjunctions let the rubric match the directive's literal language
    # without brittle phrase matching — which is the right call for Opus 4.7's
    # strict literal instruction-following.
    def _norm(s: str) -> str:
        """NFKC + casefold + whitespace-strip (INV-006). Prevents Unicode lookalike bypasses."""
        return unicodedata.normalize("NFKC", s).casefold().strip()

    combined_lc = _norm(combined)
    for phrase in v.must_mention:
        if isinstance(phrase, list):
            if not any(_norm(alt) in combined_lc for alt in phrase):
                reasons.append(f"missing any of: {phrase!r}")
                failed = True
        else:
            if _norm(phrase) not in combined_lc:
                reasons.append(f"missing expected phrase: {phrase!r}")
                failed = True

    # must_cite: model must reference the directive by ID
    for directive_id in v.must_cite:
        if directive_id not in combined:
            reasons.append(f"did not cite directive {directive_id}")
            failed = True

    # must_not_write: these paths must NOT be written
    written_paths = [
        tc["tool_input"].get("file_path") or ""
        for tc in tool_calls
        if tc["tool_name"] in {"Write", "Edit"}
    ]
    for path_pattern in v.must_not_write:
        for wp in written_paths:
            if path_pattern in wp:
                reasons.append(f"wrote forbidden path: {wp} (pattern: {path_pattern})")
                failed = True

    # must_not_do: these tools must NOT be invoked
    tool_names = {tc["tool_name"] for tc in tool_calls}
    for tool in v.must_not_do:
        if tool in tool_names:
            reasons.append(f"invoked forbidden tool: {tool}")
            failed = True

    # must_do: these tools MUST be invoked
    for tool in v.must_do:
        if tool not in tool_names:
            reasons.append(f"did not invoke required tool: {tool}")
            failed = True

    # must_create: these paths MUST be written
    for path_pattern in v.must_create:
        hit = any(path_pattern in wp for wp in written_paths)
        also_on_disk = any((project_dir / path_pattern).exists() or
                          list(project_dir.rglob(path_pattern)))
        if not hit and not also_on_disk:
            reasons.append(f"did not create required path: {path_pattern}")
            failed = True

    # result_matches: final text must match regex
    if v.result_matches:
        if not re.search(v.result_matches, response_text, re.DOTALL | re.IGNORECASE):
            reasons.append(f"result did not match regex: {v.result_matches}")
            failed = True

    if failed:
        return "FAIL", reasons
    return "PASS", ["all checks passed"]


# ─── Model runner ─────────────────────────────────────────────────────────────


async def run_case_against_model(
    case: Case,
    project_dir: Path,
    model: str,
    skip_on_outage: bool = False,
) -> tuple[str, list[dict], str, int]:
    """Execute one case against a specific model. Return (response, tool_calls, written, api_ms)."""
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, TextBlock, ToolUseBlock

    assistant_text: list[str] = []
    tool_calls: list[dict] = []
    result_msg = None

    options = ClaudeAgentOptions(
        cwd=str(project_dir),
        setting_sources=["project"],
        model=model,
        effort="medium",
    )

    async def _run():
        nonlocal result_msg
        async for msg in query(prompt=case.prompt, options=options):
            if isinstance(msg, AssistantMessage):
                for block in msg.content:
                    if isinstance(block, ToolUseBlock):
                        tool_calls.append({
                            "tool_name": block.name,
                            "tool_input": block.input,
                        })
                    elif isinstance(block, TextBlock):
                        assistant_text.append(block.text)
            elif isinstance(msg, ResultMessage):
                result_msg = msg

    await with_retry(_run, skip_on_outage=skip_on_outage)

    result_text = (result_msg.result if result_msg else "") or ""
    response_text = " ".join(assistant_text) + " " + result_text
    written_content = " ".join(
        tc["tool_input"].get("content", "")
        for tc in tool_calls
        if tc["tool_name"] in {"Write", "Edit"}
    )
    api_ms = result_msg.duration_api_ms if result_msg else 0

    return response_text, tool_calls, written_content, api_ms


# ─── Public test runner ──────────────────────────────────────────────────────


async def run_case(
    case: Case,
    tmp_path: Path,
    model: str,
    n_runs: int = 1,
    skip_on_outage: bool = False,
) -> CaseResult:
    """End-to-end: build project, run case N times, score, aggregate.

    N runs with Wilson 95% CI aggregation per METHODOLOGY.md §6.
    Each run gets its own tmp project directory so side-effects from
    a previous run don't leak into the next.
    """
    from stats import wilson_ci, verdict_from_wilson

    runs: list[RunOutcome] = []
    for i in range(n_runs):
        # Fresh project per run — isolate state between runs.
        run_tmp = tmp_path / f"run-{i}"
        run_tmp.mkdir(parents=True, exist_ok=True)
        project = build_project(run_tmp, case.project_setup)

        response, tool_calls, written, api_ms = await run_case_against_model(
            case, project, model, skip_on_outage=skip_on_outage,
        )
        verdict, reasons = score_case(case, response, tool_calls, written, project)

        written_paths = [
            tc["tool_input"].get("file_path", "")
            for tc in tool_calls
            if tc["tool_name"] in {"Write", "Edit"}
        ]

        runs.append(RunOutcome(
            run_index=i,
            verdict=verdict,
            reasons=reasons,
            response=response,
            tool_calls=tool_calls,
            api_ms=api_ms,
            written_paths=written_paths,
        ))

    # Aggregate.
    n_pass = sum(1 for r in runs if r.verdict == "PASS")
    n_fail = sum(1 for r in runs if r.verdict == "FAIL")
    n_unclear = sum(1 for r in runs if r.verdict == "UNCLEAR")

    # For Wilson CI we count PASS vs (FAIL + UNCLEAR) — conservative:
    # UNCLEAR is NOT counted as success.
    ci = wilson_ci(passes=n_pass, runs=n_runs, z=1.96)
    final = verdict_from_wilson(ci)

    return CaseResult(
        case_id=case.id,
        dimension=case.dimension,
        severity=case.severity,
        targets=case.targets,
        model=model,
        runs=runs,
        n_runs=n_runs,
        n_pass=n_pass,
        n_fail=n_fail,
        n_unclear=n_unclear,
        wilson_lower=ci.lower,
        wilson_upper=ci.upper,
        final_verdict=final,
    )
