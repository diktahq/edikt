# INV-008 — Release install URLs are tag-pinned, never branch-tracking

**Status:** Active

## Statement

Every user-facing install, upgrade, or documentation URL that resolves to a mutable source — `raw.githubusercontent.com/<org>/<repo>/main/...`, `releases/latest/download/...`, branch refs, or any path that a repo owner can silently update — is forbidden. All such URLs MUST pin to a specific release tag (`releases/download/vX.Y.Z/...` or an equivalent immutable ref). Applies to: README install commands, `bin/edikt install`/`upgrade` URL composition, `install.sh` resolve-latest flows, `.github/workflows/*.yml` user-facing references, `docs/**` snippets users are expected to copy, Homebrew formula URLs, and any MCP server configuration edikt documents.

## Rationale

The v0.5.0 security audit (2026-04-17) rated CRIT-7 on the fact that the README documented `curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash` as the canonical install command. Tracking `main` meant every push to main shipped instantly to every new user, with no time window for detection or rollback. Combined with the lack of signing (ADR-016 addressed separately), a brief repo compromise (push rights or tag-move) compromised every subsequent install.

Tag-pinning removes the branch-tracking attack primitive. A push to `main` no longer affects installs; only a new tagged release does. Combined with Sigstore signing (ADR-016), a supply-chain compromise requires both breaking the tag and forging the signing identity.

## Consequences of violation

- A branch-tracking install URL turns any push-to-main into an immediate installer swap — no review window, no staging.
- A `releases/latest/download/...` URL turns any release publication into an immediate silent rollout — no pinning for downstream consumers who want to audit before upgrading.
- Documentation that instructs users to `curl | bash` a mutable URL is a supply-chain footgun recommending attack-surface expansion.

## Implementation

- README install command uses `https://github.com/diktahq/edikt/releases/download/v<PINNED>/install.sh`. `v<PINNED>` is updated per release and lives under edikt's compile step / release runbook.
- `bin/edikt install` and `bin/edikt upgrade` compose URLs against `$RELEASE_TARBALL_BASE/<specific-tag>/edikt-payload-<tag>.tar.gz`. The tag is either explicitly supplied (`--ref <tag>`) or resolved via the GitHub Releases API and validated against `^v[0-9]+\.[0-9]+\.[0-9]+$` before use.
- `resolve_latest_tag` MAY resolve to the most recent release tag at install time, but the RESULT MUST be a specific tag recorded in subsequent state (installer receipt, `~/.edikt/state/version`), not a floating ref. "Latest" is a one-time resolution, not a persistent subscription.
- Documentation grep: no `raw.githubusercontent.com/.../main/` or `releases/latest/download/` URLs in any file under `docs/`, `README.md`, or `.github/workflows/`.

## Anti-patterns

Forbidden (all three are attacker-friendly):
```
https://raw.githubusercontent.com/diktahq/edikt/main/install.sh
https://github.com/diktahq/edikt/releases/latest/download/edikt-payload.tar.gz
curl -L https://raw.githubusercontent.com/.../HEAD/some-script.sh | bash
```

Required:
```
https://github.com/diktahq/edikt/releases/download/v0.5.0/install.sh
```

## Enforcement

- Pre-release CI check: `grep -rnE '(raw\.githubusercontent\.com/.+/main/|releases/latest/download/)' README.md docs/ .github/workflows/` MUST return zero matches before a release tag is cut.
- Security regression test under `test/security/release/` asserts no branch-tracking URL is present in user-facing files.
- Release runbook includes a step to update the pinned version in README and re-run the grep.

## Directives

[edikt:directives:start]: #
source_hash: pending
directives_hash: pending
compiler_version: "0.4.3"
paths:
  - "README.md"
  - "docs/**"
  - ".github/workflows/**"
  - "install.sh"
  - "bin/edikt"
scope:
  - implementation
  - review
directives:
  - User-facing install and upgrade URLs MUST resolve to a specific release tag. `raw.githubusercontent.com/.../main/`, `releases/latest/download/`, and any branch-tracking URL are forbidden. (ref: INV-008)
  - Pre-release CI MUST grep-fail on any branch-tracking URL in README.md, docs/, or .github/workflows/. (ref: INV-008)
  - `resolve_latest_tag` may perform a one-time resolution at install time; the RESULT must be recorded as a specific tag, never a floating ref. (ref: INV-008)
manual_directives: []
suppressed_directives: []
canonical_phrases:
  - "tag-pinned install URL"
  - "no branch tracking"
  - "INV-008"
behavioral_signal:
  cite:
    - "INV-008"
[edikt:directives:end]: #
