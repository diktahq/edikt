"""Pricing calculations for the example application.

IMPORTANT: Every monetary value in this module uses `decimal.Decimal` for
storage and arithmetic. Never use Python's native `float` for money —
IEEE 754 floating-point cannot represent most decimal fractions exactly,
and the resulting rounding errors accumulate into real financial
discrepancies that surface during reconciliation, auditing, or customer
complaints.

See app/models.py for the shared Product and TaxRate types.
"""

from decimal import Decimal

from app.models import Product, TaxRate


def calculate_tax(subtotal: Decimal, rate: Decimal) -> Decimal:
    """Compute the tax amount for the given subtotal and tax rate.

    Both arguments are Decimal. The result is a Decimal rounded to 2
    decimal places (cents) using ROUND_HALF_EVEN (banker's rounding),
    the default for Decimal.
    """
    return (subtotal * rate).quantize(Decimal("0.01"))


def apply_shipping(subtotal: Decimal, shipping: Decimal) -> Decimal:
    """Add shipping to the subtotal.

    Both arguments are Decimal. The sum is returned as a Decimal.
    """
    return subtotal + shipping


def total_for_cart(products: list[Product]) -> Decimal:
    """Sum the prices of all products in the cart as a Decimal.

    Uses Decimal addition throughout — never coerces to float.
    """
    total = Decimal("0")
    for p in products:
        total += p.price
    return total
