# ts-react

A small React front end for the orders service: `order.ts` (validation and
pricing), `api.ts` (a `fetch` call that's hard to test) and components:
`OrderForm` (untested) and `OrderSummary`, which `App` renders only through
JSX. `cart/cart.ts` holds a `Cart` class and a `GiftCart` subclass, written
with the TypeScript shapes the analyzer has to handle: a one-line
constructor and methods, a class field holding an arrow (`clear`),
`super.subtotal()`, `cart?.subtotal()` and a multi-line import.
`pages/CartPage.tsx` is an anonymous default export, and `api.ts` groups
its endpoints in an object literal (`ordersApi`). See [../README.md](../README.md) for what to try and what the gaps are.

Regenerate coverage: `npm install && npm run coverage`
