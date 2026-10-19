"""Keeps orders in memory."""
from .order import Order


class NotFound(KeyError):
    """Raised for unknown order IDs."""


class MemoryStore:
    """An in-memory order store."""

    def __init__(self) -> None:
        self._orders: dict[str, Order] = {}

    def save(self, o: Order) -> None:
        """Store an order, replacing one with the same ID."""
        self._orders[o.id] = o

    def get(self, order_id: str) -> Order:
        """Load an order by ID."""
        try:
            return self._orders[order_id]
        except KeyError:
            raise NotFound(order_id) from None
