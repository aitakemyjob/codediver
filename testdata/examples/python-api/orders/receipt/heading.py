"""The line at the top of a receipt.

It imports orders.text's format while lines.py beside it defines a format
too: matching by name alone would pick the one in this folder.
"""
from ..text import format


def heading(customer: str, cents: int) -> str:
    """The receipt's first line: who it's for and what they paid."""
    return f"Receipt for {customer}: {format(cents)}"
