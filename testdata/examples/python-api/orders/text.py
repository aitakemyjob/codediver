"""Text formatting shared by the handlers."""


def format(cents: int) -> str:
    """Cents as dollars: 1250 → "$12.50"."""
    return f"${cents // 100}.{cents % 100:02d}"
