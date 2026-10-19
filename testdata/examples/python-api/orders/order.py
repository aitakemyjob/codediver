"""The order model: validation and pricing."""
from dataclasses import dataclass

BULK_MINIMUM = 10


@dataclass
class Order:
    id: str
    sku: str
    qty: int
    price_cents: int


class InvalidOrder(ValueError):
    """Raised when an order fails validation."""


def validate(o: Order) -> None:
    """Check an order before it is stored."""
    if o.qty <= 0:
        raise InvalidOrder("quantity must be at least 1")
    if not o.sku:
        raise InvalidOrder("sku is required")
    if o.qty > 1000:
        raise InvalidOrder("quantity is over the limit of 1000")


def total(o: Order) -> int:
    """What the customer pays, in cents."""
    return o.qty * _unit_price(o)


def _unit_price(o: Order) -> int:
    """Apply the bulk discount. Only total calls it."""
    if o.qty >= BULK_MINIMUM:
        return o.price_cents * 9 // 10
    return o.price_cents


def legacy_total(o: Order) -> int:
    """The old pricing, kept for a report nobody runs. Nothing calls it."""
    result = 0
    for _ in range(o.qty):
        result += o.price_cents
    return result
