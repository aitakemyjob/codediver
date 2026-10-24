"""One receipt line per order.

This module's format is not orders.text's format: callers have to follow
their imports to tell the two apart.
"""
from ..order import Order, total
from ..text import format as dollars


def format(o: Order) -> str:
    """One receipt line: quantity, SKU and price."""
    return f"{o.qty} x {o.sku}  {dollars(total(o))}"
