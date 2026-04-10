"""Mock database layer for the orders fixture.

Simulates a database that stores timestamps as timezone-aware datetimes
(Postgres `TIMESTAMP WITH TIME ZONE`). Query functions REJECT naive
datetimes with a TypeError — any caller passing a datetime without
timezone information gets a clear error.

This models real-world behavior: a strict ORM or database driver will
either raise TypeError when mixing naive and aware datetimes, or (worse)
silently produce wrong results across DST transitions and server
timezone changes.
"""

from datetime import datetime, UTC
from typing import Any


# In-memory fake "table" with a few orders. created_at is always
# timezone-aware. A real implementation would be Postgres, SQLite,
# or equivalent.
_orders: list[dict[str, Any]] = [
    {
        "id": "order-1",
        "total_cents": 1000,
        "created_at": datetime(2026, 4, 9, 10, 0, 0, tzinfo=UTC),
    },
    {
        "id": "order-2",
        "total_cents": 2500,
        "created_at": datetime(2026, 4, 10, 8, 30, 0, tzinfo=UTC),
    },
    {
        "id": "order-3",
        "total_cents": 5000,
        "created_at": datetime(2026, 4, 10, 14, 15, 0, tzinfo=UTC),
    },
]


def query_orders_since(cutoff: datetime) -> list[dict[str, Any]]:
    """Return all orders with created_at >= cutoff.

    Raises TypeError if `cutoff` is a naive datetime (no tzinfo). This
    matches the behavior of strict databases and ORMs that refuse to
    mix naive and aware datetimes.
    """
    if cutoff.tzinfo is None:
        raise TypeError(
            "cutoff must be a timezone-aware datetime "
            "(use datetime.now(UTC), not datetime.now())"
        )

    return [o for o in _orders if o["created_at"] >= cutoff]
