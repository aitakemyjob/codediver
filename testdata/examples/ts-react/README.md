# ts-react

A small React front end for the orders service: `order.ts` (validation and
pricing), `api.ts` (a `fetch` call that's hard to test) and components:
`OrderForm` (untested) and `OrderSummary`, which `App` renders only through
JSX. See [../README.md](../README.md) for what to try and what the gaps are.

Regenerate coverage: `npm install && npm run coverage`
