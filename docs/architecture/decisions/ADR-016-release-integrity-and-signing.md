---
type: adr
id: ADR-016
title: Release integrity and signing — Sigstore keyless signing of aggregated SHA256SUMS
status: accepted
decision-makers: [Daniel Gomes]
created_at: 2026-04-17T00:00:00Z
supersedes: ADR-013
references:
  adrs: [ADR-013, ADR-014, ADR-015]
  invariants: [INV-008]
  prds: [PRD-002]
  specs: [SPEC-004]
---

# ADR-016: Release integrity and signing — Sigstore keyless signing of aggregated SHA256SUMS

**Status:** Accepted
**Date:** 2026-04-17
**Decision-makers:** Daniel Gomes

---

## Context and Problem Statement

The v0.5.0 security audit (2026-04-17) found two concurrent failures in edikt's release integrity posture:

1. **The integrity wiring mandated by ADR-013 was never functional end-to-end.** The GitHub Actions release workflow published `SHA256SUMS` over `edikt-payload-v<ver>.tar.gz`, but `bin/edikt install` and `install.sh` downloaded a *different* artifact (the auto-generated `archive/refs/tags/<tag>.tar.gz`) and attempted to verify per-file `.sha256` sidecars that the workflow never published. Verification could not succeed without `EDIKT_INSTALL_INSECURE=1`, which users were silently funnelled toward. Audit finding CRIT-6.
2. **No signing.** ADR-013 explicitly deferred signing. The README install command tracked `main` via `raw.githubusercontent.com`. A brief repo compromise, a moved tag, or a tampered release asset was undetectable end-to-end. Audit finding CRIT-7.

ADR-013's format decision (aggregated SHA256SUMS, no per-file sidecars) was correct on its own terms. The gap was implementation — the launcher fetched the wrong artifact, and there was no cryptographic anchor to the release identity. This ADR makes the implementation match ADR-013's intent and adds signing on top.

## Decision Drivers

- Integrity verification must succeed on the default install path (no opt-in env var required).
- Signing must be feasible in CI without a human key-custodian — edikt is maintained by a small team and manual GPG key handling is a liability.
- The launcher must verify both that the artifact has not been tampered with AND that the artifact was produced by edikt's release workflow (not by an attacker with transient push access).
- The `curl | bash` install path must not require cosign as a hard prerequisite — users without cosign should still get a verified install via `SHA256SUMS` alone, with a loud banner if cosign verification is skipped.
- ADR-013's aggregated `SHA256SUMS` format must remain — `bin/edikt`'s `verify_against_sidecar()` already parses it correctly.

## Considered Options

1. **GPG signing with maintainer-held keys** — classic, requires key custody and rotation procedures.
2. **Sigstore keyless (cosign) via GitHub OIDC** — short-lived certificates issued against the workflow's OIDC identity; no long-lived private keys.
3. **No signing, ship ADR-013 wiring fixes only** — closes CRIT-6 but leaves CRIT-7 open.
4. **npm/PyPI package registry** — delegates signing to a third-party registry — rejected because edikt is a shell-based installer, not a package.

## Decision

We will adopt **Sigstore keyless signing** of an aggregated `SHA256SUMS` file. The full release integrity model:

(a) Publish exactly one canonical payload artifact per release tag: `edikt-payload-<tag>.tar.gz` as a GitHub Release asset (not the auto-generated archive).

(b) Publish `SHA256SUMS` alongside the payload as a Release asset, covering every asset in the release. Format is the standard `sha256sum` format (carries forward from ADR-013).

(c) Sign `SHA256SUMS` with Sigstore keyless: the release workflow requests a short-lived signing certificate from Fulcio via GitHub's OIDC token, signs `SHA256SUMS`, and publishes the combined signature + certificate as a single `SHA256SUMS.sig.bundle` Release asset. No separate `.pem` or `.sig` files — the bundle is self-contained.

(d) The launcher (`install.sh`) and `bin/edikt install`/`upgrade`:
- Download the `edikt-payload-<tag>.tar.gz` Release asset (NOT the auto-generated archive).
- Download `SHA256SUMS` and `SHA256SUMS.sig.bundle`.
- Verify the bundle signature using cosign with a regex-form identity assertion:
  ```
  cosign verify-blob \
    --bundle SHA256SUMS.sig.bundle \
    --certificate-identity-regexp '^https://github\.com/diktahq/edikt/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$' \
    --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
    SHA256SUMS
  ```
  The regex form is required because the launcher does not know the exact tag at verify time. Literal-tag identity would require re-resolving per install.
- Grep `SHA256SUMS` for the payload filename, assert the SHA matches the downloaded bytes.
- If cosign is not installed, abort with an actionable install instruction unless `EDIKT_INSTALL_INSECURE=1` is set. When honored, append a loud banner to the post-install output: "⚠️ Integrity verification was disabled via EDIKT_INSTALL_INSECURE=1."

(e) The README install command and all documentation MUST pin to a specific release tag (`releases/download/v<PINNED>/install.sh`). `main` and `releases/latest/download/` are never user-facing install surfaces. This decision is companion to INV-008 which states the hard rule; this ADR documents the implementation mechanism.

## Alternatives Considered

### GPG signing
- **Pros:** Well-understood, no third-party trust dependency beyond the maintainer.
- **Cons:** Requires a private key kept secret forever; key rotation procedures; key loss means no more signed releases; contributor onboarding requires a web-of-trust step.
- **Rejected because:** the key-custody burden is disproportionate for a solo-maintainer project. Sigstore offloads the trust root to the verifiable chain of the workflow's OIDC identity.

### No signing (fix ADR-013 wiring only)
- **Pros:** Smaller scope; closes CRIT-6.
- **Cons:** CRIT-7 remains — a transient repo compromise still compromises future installs.
- **Rejected because:** the marginal cost of adding cosign to the release workflow is small (~30 lines of YAML) vs. leaving a Critical finding open.

## Consequences

- **Good:** CRIT-6 and CRIT-7 are both closed. `EDIKT_INSTALL_INSECURE=1` becomes a rarely-used escape hatch rather than the default path.
- **Good:** The supply chain is publicly auditable — anyone can run `cosign verify-blob` against a released `SHA256SUMS` and confirm it came from edikt's workflow at a specific tag.
- **Good:** Key rotation is continuous — every release gets a fresh certificate. No long-lived secret to lose.
- **Bad:** Adds cosign as a prerequisite for strict-mode installs. Mitigated by including install instructions in the installer's error message.
- **Bad:** GitHub OIDC trust assumption — if GitHub's OIDC token issuance is compromised, Sigstore could issue malicious certificates under edikt's identity. Mitigated by Sigstore's transparency log (`rekor`), which allows after-the-fact detection of any signing event.
- **Neutral:** ADR-013's aggregated format decision is preserved. Its implementation wiring is what changes.

## Confirmation

- `.github/workflows/release.yml` publishes `edikt-payload-v<ver>.tar.gz`, `SHA256SUMS`, and `SHA256SUMS.sig.bundle` as Release assets.
- `install.sh` and `bin/edikt install` both use `cosign verify-blob` with the regex identity before extracting any artifact.
- With a tampered `SHA256SUMS` (manually edited post-release), verification fails and the installer aborts with exit code 3 (`EX_CHECKSUM`).
- README install command uses `releases/download/v<PINNED>/install.sh` — `grep -rn 'raw\.githubusercontent\.com/.*/main/' README.md docs/` returns zero matches.
- `ADR-013.md` Status line reads "Superseded by ADR-016" in the bolded Status line and the frontmatter.

## Directives

[edikt:directives:start]: #
source_hash: pending
directives_hash: pending
compiler_version: "0.4.3"
topic: release
paths:
  - ".github/workflows/release.yml"
  - "install.sh"
  - "bin/edikt"
  - "README.md"
scope:
  - implementation
  - review
directives:
  - Release workflow MUST publish `edikt-payload-<tag>.tar.gz`, `SHA256SUMS`, and `SHA256SUMS.sig.bundle` as GitHub Release assets. NEVER use the auto-generated archive as the canonical artifact. (ref: ADR-016)
  - Release workflow MUST sign `SHA256SUMS` with Sigstore keyless (`cosign sign-blob --yes --bundle SHA256SUMS.sig.bundle SHA256SUMS`) using the workflow's GitHub OIDC identity. NEVER publish a redundant `.pem` or detached `.sig` — the bundle is self-contained. (ref: ADR-016)
  - `install.sh` and `bin/edikt install`/`upgrade` MUST verify `SHA256SUMS.sig.bundle` with `cosign verify-blob --bundle ... --certificate-identity-regexp '^https://github\.com/diktahq/edikt/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$' --certificate-oidc-issuer 'https://token.actions.githubusercontent.com'` before extracting any artifact. (ref: ADR-016)
  - If cosign is unavailable, the installer MUST abort with an actionable message unless `EDIKT_INSTALL_INSECURE=1` is set. When honored, the post-install banner MUST include a loud warning. (ref: ADR-016)
  - Launcher URL composition MUST use `https://github.com/diktahq/edikt/releases/download/<tag>/edikt-payload-<tag>.tar.gz`. NEVER use `archive/refs/tags/<tag>.tar.gz`. (ref: ADR-016)
manual_directives: []
suppressed_directives: []
[edikt:directives:end]: #

---

*Captured by edikt:adr — 2026-04-17*
