"""Statistics helpers — Wilson score interval for binomial proportions.

Used for per-case confidence intervals when N runs are performed.
Wilson CI is preferred over Normal approximation for small samples and
proportions near 0 or 1.
"""

from __future__ import annotations

import math
from dataclasses import dataclass


@dataclass
class WilsonCI:
    """Wilson score interval for a binomial proportion."""
    p_hat: float         # sample proportion
    lower: float         # lower bound
    upper: float         # upper bound
    n: int               # sample size
    z: float             # z-score used (1.96 for 95%)

    @property
    def width(self) -> float:
        return self.upper - self.lower


def wilson_ci(passes: int, runs: int, z: float = 1.96) -> WilsonCI:
    """Compute the Wilson score interval for `passes` passes out of `runs` runs.

    z=1.96 → 95% CI, z=2.576 → 99% CI.

    Degenerate cases:
    - runs == 0 → returns (0, 0, 1, 0, z) — no data, interval spans [0, 1]
    """
    if runs <= 0:
        return WilsonCI(p_hat=0.0, lower=0.0, upper=1.0, n=0, z=z)

    p = passes / runs
    denom = 1 + z * z / runs
    centre = (p + z * z / (2 * runs)) / denom
    half_width = (z * math.sqrt(p * (1 - p) / runs + z * z / (4 * runs * runs))) / denom
    return WilsonCI(
        p_hat=p,
        lower=max(0.0, centre - half_width),
        upper=min(1.0, centre + half_width),
        n=runs,
        z=z,
    )


def verdict_from_wilson(ci: WilsonCI) -> str:
    """Map a Wilson CI to a PASS / FAIL / UNCLEAR verdict.

    Per METHODOLOGY.md §6:
    - PASS if lower ≥ 0.5
    - FAIL if upper < 0.5
    - UNCLEAR otherwise (CI straddles 0.5)
    """
    if ci.n == 0:
        return "UNCLEAR"
    if ci.lower >= 0.5:
        return "PASS"
    if ci.upper < 0.5:
        return "FAIL"
    return "UNCLEAR"


def fisher_exact_2x2(
    a_pass: int, a_fail: int, b_pass: int, b_fail: int,
) -> float:
    """Two-tailed Fisher exact p-value for a 2x2 table.

    Used for cross-model comparison: is model A's compliance significantly
    different from model B's on the same case set?

    Exact calculation using the hypergeometric distribution.
    """
    n = a_pass + a_fail + b_pass + b_fail
    row1 = a_pass + a_fail
    col1 = a_pass + b_pass

    # Probability of observed or more extreme tables.
    observed = _hypergeom_pmf(a_pass, n, row1, col1)
    p_two_tail = 0.0
    k_max = min(row1, col1)
    for k in range(0, k_max + 1):
        prob_k = _hypergeom_pmf(k, n, row1, col1)
        if prob_k <= observed + 1e-12:
            p_two_tail += prob_k
    return min(1.0, p_two_tail)


def _hypergeom_pmf(k: int, N: int, K: int, n: int) -> float:
    """Hypergeometric PMF: P(X = k) where X ~ Hypergeometric(N, K, n)."""
    if k > K or k > n or k < 0 or (n - k) > (N - K):
        return 0.0
    return (
        math.comb(K, k) * math.comb(N - K, n - k) / math.comb(N, n)
    )
