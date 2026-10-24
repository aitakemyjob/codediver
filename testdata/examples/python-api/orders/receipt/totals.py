"""What a receipt adds up to."""
import orders.order as pricing


def grand_total(orders: list) -> int:
    """What a set of orders costs together, in cents."""
    return sum(pricing.total(o) for o in orders)
