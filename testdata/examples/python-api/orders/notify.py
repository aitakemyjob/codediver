"""Tells the warehouse about new orders.

It reads the clock, the environment and the network directly, which is
what makes it hard to test.
"""
import json
import os
import urllib.request
from datetime import datetime, timezone

from .order import Order


def send(o: Order) -> None:
    """Post an order to the warehouse webhook."""
    url = os.environ.get("WAREHOUSE_URL")
    if not url:
        return
    body = json.dumps({"order": o.__dict__, "sent_at": datetime.now(timezone.utc).isoformat()}).encode()
    req = urllib.request.Request(url, data=body, headers={"Content-Type": "application/json"})
    urllib.request.urlopen(req, timeout=5).close()
