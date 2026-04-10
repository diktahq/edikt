"""Domain models for the pricing fixture.

All monetary values use `decimal.Decimal` — never `float`. Floating-point
arithmetic is inexact by design and silently corrupts money values through
accumulated rounding errors.
"""

from dataclasses import dataclass
from decimal import Decimal


@dataclass(frozen=True)
class Product:
    """A product with a fixed-point price.

    The price field is always a Decimal. Never use float or Python's native
    numeric types for money — they cannot represent common decimal fractions
    exactly (e.g., 0.1 + 0.2 != 0.3 in float arithmetic).
    """

    id: str
    name: str
    price: Decimal


@dataclass(frozen=True)
class TaxRate:
    """A tax rate expressed as a Decimal fraction (0.08 == 8%)."""

    jurisdiction: str
    rate: Decimal
