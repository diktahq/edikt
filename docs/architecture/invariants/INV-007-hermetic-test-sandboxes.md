# INV-007 — Benchmark and test sandboxes are hermetic

**Status:** Active

## Statement

Sandboxes created by the test harness or benchmark runner to execute Claude sessions MUST be hermetic. They MUST NOT copy the host user's `~/.claude/settings.json`, user-global settings, hooks, shell rc files, SSH keys, or any other host credential or configuration into the sandbox. The `setting_sources` list passed to subprocess Claude invocations in tests is restricted to `["project"]`; `"user"` is forbidden. The sandbox receives only a curated minimal `settings.json` written by the test harness.

## Rationale

The v0.5.0 security audit (2026-04-17) found that `test/integration/benchmarks/runner.py` unconditionally copied the maintainer's live `.claude/settings.json` — including any local experimental hooks — into every benchmark sandbox, then ran Claude there against adversarial corpus prompts with `setting_sources=["user", "project"]` (audit finding HI-10). An attack-template `Write` that matched any `PostToolUse` regex in the host settings fired the host hook with attacker-controlled content as input. Any user-global hook also fired. This turned the test harness into an attack surface against the developer running it.

A hermetic sandbox is standard practice for any system that runs untrusted workloads. edikt's threat model includes red-team corpus runs against directives, so this must hold.

## Consequences of violation

- A benchmark corpus run executes host-local hooks against attacker-controlled payloads, possibly including developer-specific shell commands with full user permissions.
- Host secrets (API keys, session tokens, environment variables in `.env`) leak into benchmark result JSONL or are accessible to the model running inside the sandbox.
- CI runs that pick up maintainer-pushed settings inherit those settings — non-reproducible benchmark results, potentially exfiltrating content.

## Implementation

The test harness writes a minimal `settings.json` into each sandbox containing:
- No `hooks` key.
- An explicit `permissions` block with only the tools the benchmark requires (typically `Read(**)`, `Edit(**)`, `Write(**)`, `Bash(./test/run.sh)`, `Bash(pytest :*)`).
- No MCP server configuration.
- No `statusLine`.

Every subprocess Claude invocation in `test/` passes `setting_sources=["project"]` (or the equivalent CLI flag). `"user"` is never used in tests.

Copies of repo content (ADRs, invariants, plans) into the sandbox use `shutil.copytree(..., symlinks=True)` — symlinks are preserved, never dereferenced, and escape checks refuse any source path whose realpath leaves the intended source root.

Result JSONL written by benchmarks redacts `tool_calls[*].tool_input.content` (replace with `<redacted:len=N>`), length-caps `response` at 4096 characters, and aborts the run if it detects credential-pattern regexes in any serialized field. The canonical credential-pattern list is maintained in `test/integration/benchmarks/runner.py` as a module-level constant (single source of truth); updates to the list are code changes in that file, not in this invariant.

## Anti-patterns

Forbidden:
```python
# runner.py copying host settings — becomes attack surface
if (repo_root / ".claude" / "settings.json").is_file():
    shutil.copy2(repo_root / ".claude" / "settings.json", project / ".claude" / "settings.json")
claude_options = ClaudeAgentOptions(..., setting_sources=["user", "project"])
```

Required:
```python
# runner.py writes a curated minimal settings.json
(project / ".claude").mkdir(exist_ok=True)
(project / ".claude" / "settings.json").write_text(json.dumps(MINIMAL_BENCHMARK_SETTINGS))
claude_options = ClaudeAgentOptions(..., setting_sources=["project"])
```

## Enforcement

- Security regression tests under `test/security/sandbox/` assert that, for a representative benchmark run, the sandbox `settings.json` contains no `hooks` key and no entries outside the curated allowlist.
- CI grep: `grep -rn 'setting_sources=\[.*user' test/` MUST return zero matches.
- `test/integration/benchmarks/results/claude-*/` is `.gitignore`d; committed result directories (summaries/baselines) are grepped for credential patterns pre-merge.

## Directives

[edikt:directives:start]: #
source_hash: f55b3d58ddf3f809b241756a10adb8e0caf76769dee3ec66ff4be4f04ef052cd
directives_hash: pending
compiler_version: "0.4.3"
paths:
  - "test/integration/**"
  - "test/unit/**"
scope:
  - implementation
  - review
directives:
  - Test and benchmark sandboxes MUST NOT copy the host's `~/.claude/settings.json`, user-global settings, hooks, or secrets. (ref: INV-007)
  - `setting_sources` for subprocess Claude invocations in tests is `["project"]`. `"user"` is forbidden. (ref: INV-007)
  - Test harness writes a curated minimal `settings.json` into each sandbox; `hooks` key MUST be absent. (ref: INV-007)
  - Repo content copies into sandboxes MUST use `shutil.copytree(..., symlinks=True)` and refuse source paths whose realpath escapes the source root. (ref: INV-007)
  - Benchmark JSONL results MUST redact `tool_calls[*].tool_input.content`, length-cap `response`, and abort on credential-pattern detection. (ref: INV-007)
manual_directives: []
suppressed_directives: []
canonical_phrases:
  - "hermetic sandbox"
  - "setting_sources project only"
  - "INV-007"
behavioral_signal:
  cite:
    - "INV-007"
[edikt:directives:end]: #
