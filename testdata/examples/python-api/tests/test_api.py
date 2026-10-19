from orders.api import create_order
from orders.store import MemoryStore


def test_create_order_rejects_bad_json():
    status, _ = create_order(MemoryStore(), "{")
    assert status == 400
