# ts-react

A small React front end for the orders service: `order.ts` (validation and
pricing), `api.ts` (a `fetch` call that's hard to test) and components:
`OrderForm` (untested) and `OrderSummary`, which `App` renders only through
JSX. `cart/cart.ts` holds a `Cart` class and a `GiftCart` subclass, written
with the TypeScript shapes the analyzer has to handle: a one-line
constructor and methods, a class field holding an arrow (`clear`),
`super.subtotal()`, `cart?.subtotal()` and a multi-line import.
`pages/CartPage.tsx` is an anonymous default export, and `api.ts` groups
its endpoints in an object literal (`ordersApi`). `receipt/` and
`pages/ReceiptPage.tsx` show calls that only resolve through imports:
`cart/format.ts` and `receipt/format.ts` both export `format`, `totals.ts`
imports `total as orderTotal`, `receipt/index.ts` is a barrel that renames
`format` to `formatLine` and re-exports `totals.ts`, and `ReceiptPage`
uses it as `import * as receipt` and imports the printer's default export
as `emit`. Open **Calls** on `ReceiptPage` or **Trace to here** on either
`format` to see each call land on the right definition. See [../README.md](../README.md) for what to try and what the gaps are.

Regenerate coverage: `npm install && npm run coverage`
