"""Order-related business logic for the fixture.

All datetime values in this module are timezone-aware. Never use
Python's naive `datetime.now()` (without a timezone argument) —
comparing a naive datetime against a timezone-aware one from the
database either raises TypeError or (worse) silently produces wrong
results across DST transitions and server timezone changes.

The existing `get_todays_orders` function demonstrates the correct
pattern: `datetime.now(UTC)` for current time, then pass through to
the database layer which expects timezone-aware datetimes.
"""

from datetime import datetime, timedelta, UTC
from typing import Any

from app import db


def get_todays_orders() -> list[dict[str, Any]]:
    """Return all orders created today (since midnight UTC)."""
    now = datetime.now(UTC)
    start_of_day = now.replace(hour=0, minute=0, second=0, microsecond=0)
    return db.query_orders_since(start_of_day)
