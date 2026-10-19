# python-api

A small orders service in Python: `orders/order.py` (validation and
pricing), `store.py` (in-memory), `notify.py` (a webhook that's hard to
test) and `api.py` (request handlers). See [../README.md](../README.md) for
what to try and what the gaps are.

Regenerate coverage:
`pip install -r requirements-dev.txt && python -m pytest --cov=orders --cov-report=lcov:coverage.lcov`
