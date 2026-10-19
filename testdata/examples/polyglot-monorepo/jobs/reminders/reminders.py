"""Payment reminders for overdue invoices."""
import os
import smtplib
from datetime import date, timedelta

GRACE_DAYS = 14


def overdue(invoices: list[dict], today: date) -> list[dict]:
    """Invoices past their grace period that aren't void."""
    return [inv for inv in invoices if not inv.get("void") and _is_late(inv, today)]


def _is_late(inv: dict, today: date) -> bool:
    """Whether an invoice is past due. Only overdue calls it."""
    return date.fromisoformat(inv["issued"]) + timedelta(days=GRACE_DAYS) < today


def send_reminders(invoices: list[dict]) -> int:
    """Email every overdue customer. Reads the clock, the environment and SMTP."""
    late = overdue(invoices, date.today())
    with smtplib.SMTP(os.environ.get("SMTP_HOST", "localhost")) as smtp:
        for inv in late:
            smtp.sendmail("billing@example.com", inv["email"], f"Invoice {inv['id']} is overdue")
    return len(late)
