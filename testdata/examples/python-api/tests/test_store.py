from orders.order import Order
from orders.store import MemoryStore


def test_save_and_get():
    s = MemoryStore()
    s.save(Order(id="o1", sku="A1", qty=2, price_cents=100))
    assert s.get("o1").qty == 2
