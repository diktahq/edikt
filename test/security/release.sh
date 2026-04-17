#!/usr/bin/env bash
# Pins ADR-016 / CRIT-6, CRIT-7 — release integrity wiring.

set -eu
cd "$(dirname "$0")/../.."

fail=0

# ── 1. release.yml has cosign signing + id-token permission.
if ! grep -q 'id-token:[[:space:]]*write' .github/workflows/release.yml; then
    echo "[ADR-016] release.yml missing id-token: write permission (required for OIDC signing)" >&2
    fail=1
fi
if ! grep -q 'cosign sign-blob' .github/workflows/release.yml; then
    echo "[ADR-016] release.yml missing cosign sign-blob step" >&2
    fail=1
fi
if ! grep -q 'SHA256SUMS.sig.bundle' .github/workflows/release.yml; then
    echo "[ADR-016] release.yml does not publish SHA256SUMS.sig.bundle" >&2
    fail=1
fi

# ── 2. install.sh has cosign verification.
if ! grep -q 'cosign verify-blob' install.sh; then
    echo "[ADR-016] install.sh missing cosign verify-blob call" >&2
    fail=1
fi
if ! grep -q 'certificate-identity-regexp' install.sh; then
    echo "[ADR-016] install.sh missing --certificate-identity-regexp (literal identity is fragile)" >&2
    fail=1
fi

# ── 3. Go upgrade command uses releases/download (not auto-archive).
# bin/edikt (bash launcher) was deleted in ADR-022 Phase 3 — the URL
# check now applies to the Go upgrade implementation in tools/gov-compile/.
if grep -rq 'archive/refs/tags' tools/gov-compile/cmd/upgrade.go 2>/dev/null; then
    echo "[ADR-016] tools/gov-compile/cmd/upgrade.go still references archive/refs/tags" >&2
    fail=1
fi
if ! grep -rq 'releases/download' tools/gov-compile/cmd/upgrade.go 2>/dev/null; then
    echo "[ADR-016] tools/gov-compile/cmd/upgrade.go does not use releases/download URL base" >&2
    fail=1
fi

# ── 4. README install command is tag-pinned (INV-008).
if grep -q 'raw.githubusercontent.com/diktahq/edikt/main/install.sh' README.md; then
    echo "[INV-008] README still points at raw.githubusercontent.com/.../main/install.sh" >&2
    fail=1
fi
if ! grep -q 'releases/download/v[0-9]' README.md; then
    echo "[INV-008] README does not pin install URL to a specific release tag" >&2
    fail=1
fi

# ── 5. Loud banner when EDIKT_INSTALL_INSECURE is honored.
if ! grep -q 'INTEGRITY VERIFICATION WAS DISABLED' install.sh; then
    echo "[MED-13] install.sh missing loud post-install banner for insecure mode" >&2
    fail=1
fi

exit $fail
