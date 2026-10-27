"""Maintenance jobs for the orders service."""
import threading

from .order import Order


def _by_id(o: Order) -> str:
    """Sort key: the order's ID. Only passed to sorted, never called."""
    return o.id


def replay(target, orders: list[Order]) -> None:
    """Save orders again, in ID order, into any target with a save method.

    target has no annotation, so target.save could be MemoryStore.save or
    AuditLog.save: CodeDiver links one and marks the edge as a guess.
    """
    for o in sorted(orders, key=_by_id):
        target.save(o)


def replay_in_background(target, orders: list[Order]) -> threading.Thread:
    """Replay on a background thread; replay is passed as its target."""
    t = threading.Thread(target=replay, args=(target, orders))
    t.start()
    return t
