# Security Workflow

edikt provides security coverage at three points in your workflow: while you build (Stop hook signals), before you push (pre-push hook), and on demand (explicit audit). This guide explains each layer and when to use it.

## The three layers

```text
Building → Stop hook flags security domains
Pushing  → Pre-push hook scans for obvious patterns
On demand → /edikt:audit for deliberate security passes
```

These layers are complementary — lightweight and continuous early, thorough and deliberate before shipping.

---

## Layer 1 — Stop hook signals (while building)

The `Stop` hook watches every Claude response for security-sensitive domains. When it detects one, Claude ends its response with:

```text
🔒 Security-sensitive domain — run `/edikt:audit` before shipping this feature.
```

This fires when the response involves: authentication, authorization, payment processing, PII handling, cryptography, token management, or access control.

It's a nudge, not a block. The signal means "this deserves a deliberate security review before it ships" — not that something is necessarily wrong.

---

## Layer 2 — Pre-push hook (before every push)

The pre-push git hook runs a lightweight grep scan on every `git push`. It checks the diff for:

- **Hardcoded secrets** — API keys, passwords, tokens assigned in code
- **SQL string concatenation** — potential injection vectors
- **Unresolved security TODOs** — `TODO: validate`, `TODO: sanitize`, etc.

If patterns are found, it warns before the push completes:

```text
🔒 edikt: security patterns detected (2):
   • api_key = "sk-..." — src/config.go
   • TODO: validate input — src/api/users.go

   Run /edikt:audit to review. Pushing anyway.
   To skip: EDIKT_SECURITY_SKIP=1 git push
```

**It never blocks.** Always exits 0. You decide whether to fix before pushing.

### Disable for one push

```bash
EDIKT_SECURITY_SKIP=1 git push
```

To also skip the invariant check in the same push:

```bash
EDIKT_INVARIANT_SKIP=1 git push
```

Both flags can be combined:

```bash
EDIKT_SECURITY_SKIP=1 EDIKT_INVARIANT_SKIP=1 git push
```

### Disable permanently

```yaml
# .edikt/config.yaml
hooks:
  pre-push-security: false
```

---

## Layer 3 — /edikt:audit (deliberate security pass)

For features that touch security-sensitive domains, run a full audit before shipping:

```bash
/edikt:audit              ← full codebase
/edikt:audit api          ← routes and handlers only
/edikt:audit auth         ← authentication and authorization code
/edikt:audit src/payments/ ← specific directory
```

This invokes the `security` agent for a thorough review:

**OWASP Top 10 coverage:**
- A01 Broken Access Control
- A02 Cryptographic Failures
- A03 Injection (SQL, command, template, XSS)
- A04 Insecure Design
- A05 Security Misconfiguration
- A07 Authentication Failures
- A09 Logging Failures (PII in logs)

**Plus:**
- Hardcoded secret detection
- Input validation coverage
- File upload sanitization
- SQL parameterization check

**Output:**
```text
SECURITY AUDIT — 2026-03-08
─────────────────────────────────────────────────────
Scope: src/payments/

🔴 CRITICAL
  • src/payments/handler.go:47 — SQL built with string concat — injection risk

🟡 WARNINGS
  • src/payments/webhook.go:23 — no HMAC signature validation on incoming webhook

🟢 CLEAN
  • No hardcoded secrets detected
  • Input validation present on all public routes

OWASP Checklist:
  A01 Access Control    ✅
  A02 Cryptography      ✅
  A03 Injection         ❌
  A04 Insecure Design   ⚠️
  A05 Misconfiguration  ✅
  A07 Auth Failures     ✅
  A09 Logging           ✅
─────────────────────────────────────────────────────
```

---

## When to run what

| Situation | Action |
|-----------|--------|
| Building auth, payments, or PII features | Watch for 🔒 Stop hook signals |
| Before every push | Pre-push hook runs automatically |
| Finishing a security-sensitive feature | `/edikt:audit auth` or `/edikt:audit api` |
| Full security review before release | `/edikt:audit` (full codebase) |
| Post-implementation review of any feature | `/edikt:review` (includes security domain if relevant files changed) |

---

## Pre-flight security review in plans

When you run `/edikt:plan` and the plan mentions auth, payments, tokens, RBAC, or similar, the `security` agent automatically reviews the plan before execution:

```text
SECURITY
  🟡  JWT secret storage not specified — clarify storage mechanism in phase 2
  🟢  Auth middleware scoping looks correct
```

Catching security issues in the plan is far cheaper than catching them in code review.

---

## The security agent

All security checks in edikt route through the `security` specialist agent. You can invoke it directly for anything security-related — threat modeling, reviewing a specific pattern, checking a library's security posture:

```text
Ask security to review our OAuth implementation
```

The agent is installed automatically on projects where the project-context.md description mentions payments, auth, HIPAA, PCI, compliance, or security.

---

## Environment hardening

### Strip credentials from subprocesses

Set `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB=1` to strip Anthropic and cloud provider credentials from subprocess environments (Bash tool, hooks, MCP servers). This prevents hook scripts and MCP servers from accidentally accessing or leaking API keys.

Add to your shell profile:

```bash
export CLAUDE_CODE_SUBPROCESS_ENV_SCRUB=1
```

### Sandbox enforcement in CI

Set `sandbox.failIfUnavailable: true` in your CI settings to ensure Claude Code exits with an error if the sandbox cannot start, instead of running unsandboxed:

```json
{
  "sandbox": {
    "enabled": true,
    "failIfUnavailable": true
  }
}
```

This prevents governance checks from running in an unprotected environment.
