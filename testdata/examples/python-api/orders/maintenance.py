"""Maintenance jobs for the orders service."""
from .order import Order


def replay(target, orders: list[Order]) -> None:
    """Save orders again into any target with a save method.

    target has no annotation, so target.save could be MemoryStore.save or
    AuditLog.save: CodeDiver links one and marks the edge as a guess.
    """
    for o in orders:
        target.save(o)
