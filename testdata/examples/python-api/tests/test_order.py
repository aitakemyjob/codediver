import pytest

from orders.order import InvalidOrder, Order, total, validate


def test_validate_rejects_zero_qty():
    with pytest.raises(InvalidOrder):
        validate(Order(id="o1", sku="A1", qty=0, price_cents=100))


class TestTotal:
    def test_applies_bulk_discount(self):
        assert total(Order(id="o1", sku="A1", qty=10, price_cents=100)) == 900
