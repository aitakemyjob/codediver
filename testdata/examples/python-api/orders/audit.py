"""Keeps a record of what happened to each order.

AuditLog.save and MemoryStore.save share a name: create_order calls both,
and each call has to reach its own class's save.
"""
from .order import Order


class AuditLog:
    """An in-memory record of stored orders."""

    def __init__(self) -> None:
        self.entries: list[str] = []

    def save(self, o: Order) -> None:
        """Record that an order was stored."""
        self.entries.append(f"saved {o.id}")
