# /edikt:audit

Security audit — OWASP scan, secret detection, auth coverage, and vulnerability patterns.

## Usage

```bash
/edikt:audit                     ← full codebase scan
/edikt:audit api                 ← routes and handlers only
/edikt:audit auth                ← authentication and authorization code
/edikt:audit data                ← data access and storage
/edikt:audit src/payments/       ← specific directory
```

## Arguments

| Argument | Description |
|----------|-------------|
| (none) | Full codebase scan |
| `api` | Routes and handlers only |
| `auth` | Authentication and authorization code only |
| `data` | Data access and storage code only |
| A file path | Scan a specific file or directory |
| `--no-edikt` | Run all checks inline without spawning specialist agents |

## What it does

Routes to the `security` and `sre` agents to audit the specified scope. The `security` agent covers OWASP Top 10, hardcoded secrets, and input validation. The `sre` agent covers deployment configuration, exposed ports, and infrastructure misconfigurations. The audit is deliberate — run it before shipping a security-sensitive feature.

For lighter continuous security checks, edikt also adds a pre-push git hook that greps for obvious patterns (hardcoded credentials, SQL concatenation, unresolved security TODOs) and warns before every push.

## What's checked

**OWASP Top 10:**
- A01 Broken Access Control — auth on all routes, privilege escalation paths
- A02 Cryptographic Failures — hardcoded secrets, weak crypto, unencrypted PII
- A03 Injection — SQL concat, command injection, template injection, XSS
- A04 Insecure Design — missing rate limiting, no input validation
- A05 Security Misconfiguration — debug mode, default credentials
- A07 Auth Failures — session management, token expiry
- A09 Logging Failures — PII in logs, missing audit trail

**Secret detection:**
- Hardcoded API keys, passwords, tokens in source code
- Internal endpoints or IPs that shouldn't be committed

**Input validation:**
- Routes accepting user input — do they validate?
- File uploads — are they sanitized?
- SQL queries — parameterized or concatenated?

## Output

```text
AUDIT REPORT — 2026-03-08
─────────────────────────────────────────────────────
Scope: full codebase

SECURITY

🔴 CRITICAL
  • src/api/users.go:42 — SQL query built with string concat — inject risk

🟡 WARNINGS
  • src/auth/jwt.go:15 — token expiry not enforced on refresh

🟢 CLEAN
  • No hardcoded secrets detected
  • Input validation: present on all public routes

OWASP Checklist:
  A01 Access Control    ✅
  A02 Cryptography      ✅
  A03 Injection         ❌
  A04 Insecure Design   ⚠️
  A05 Misconfiguration  ✅
  A07 Auth Failures     ⚠️
  A09 Logging           ✅
RELIABILITY
  🟢 Error handling present on all public routes
  🟡 No circuit breaker on external API calls

─────────────────────────────────────────────────────
1 critical, 3 warnings. Run /edikt:audit auth to narrow focus.
```

## Proactive suggestions

The `Stop` hook watches for security-sensitive domains — auth, payments, PII, tokens, cryptography. When detected, Claude adds:

```text
🔒 Security-sensitive domain — run `/edikt:audit` before shipping this feature.
```

The pre-push hook also runs a lightweight grep on every push and warns if patterns are found. It never blocks — always exits 0. Disable with `EDIKT_SECURITY_SKIP=1 git push` or permanently via `.edikt/config.yaml`.

## Natural language triggers

- "audit security"
- "check for vulnerabilities"
- "scan for secrets"
- "security review"
