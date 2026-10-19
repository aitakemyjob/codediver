"""Request handlers for the orders API."""
import json

from . import notify
from .order import InvalidOrder, Order, total, validate
from .store import MemoryStore, NotFound


def create_order(store: MemoryStore, body: str) -> tuple[int, dict]:
    """Validate, price, store and announce an order. Returns (status, response)."""
    try:
        data = json.loads(body)
    except json.JSONDecodeError as e:
        return 400, {"error": str(e)}
    o = Order(id=data.get("id", ""), sku=data.get("sku", ""), qty=data.get("qty", 0), price_cents=data.get("price_cents", 0))
    try:
        validate(o)
    except InvalidOrder as e:
        return 422, {"error": str(e)}
    store.save(o)
    notify.send(o)
    return 201, {"id": o.id, "total": total(o)}


def get_order(store: MemoryStore, order_id: str) -> tuple[int, dict]:
    """Return one order."""
    try:
        return 200, store.get(order_id).__dict__
    except NotFound:
        return 404, {"error": "not found"}
