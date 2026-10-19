from datetime import date

from reminders.reminders import overdue


def test_overdue_skips_void_invoices():
    invoices = [
        {"id": "i1", "issued": "2026-01-01", "void": False},
        {"id": "i2", "issued": "2026-01-01", "void": True},
    ]
    assert [i["id"] for i in overdue(invoices, date(2026, 3, 1))] == ["i1"]
