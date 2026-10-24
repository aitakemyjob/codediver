# python-api

A small orders service in Python: `orders/order.py` (validation and
pricing), `store.py` (in-memory), `notify.py` (a webhook that's hard to
test) and `api.py` (request handlers). See [../README.md](../README.md) for
what to try and what the gaps are.

`orders/receipt/` and `api.get_receipt` show calls that only resolve
through imports: `orders/text.py` and `receipt/lines.py` both define
`format`, and `receipt/heading.py` imports the one from `text.py` although
the other sits beside it; `lines.py` imports it as `dollars`; `totals.py`
uses `import orders.order as pricing`; and `get_receipt` does
`from . import receipt` and calls `receipt.line()` and
`receipt.grand_total()`, which the package's `__init__.py` re-exports
(`from .lines import format as line`, `from .totals import *`). Open
**Calls** on `get_receipt` or **Trace to here** on either `format`.

Regenerate coverage:
`pip install -r requirements-dev.txt && python -m pytest --cov=orders --cov-report=lcov:coverage.lcov`
