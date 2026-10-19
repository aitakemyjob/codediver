// The order model: validation and pricing.

export interface Order {
  id: string;
  sku: string;
  qty: number;
  priceCents: number;
}

const bulkMinimum = 10;

/** Checks an order before it is sent; returns an error message or null. */
export function validate(o: Order): string | null {
  if (o.qty <= 0) return 'quantity must be at least 1';
  if (!o.sku) return 'sku is required';
  if (o.qty > 1000) return 'quantity is over the limit of 1000';
  return null;
}

/** What the customer pays, in cents. */
export function total(o: Order): number {
  return o.qty * unitPrice(o);
}

/** Applies the bulk discount. Only total calls it. */
function unitPrice(o: Order): number {
  return o.qty >= bulkMinimum ? Math.round((o.priceCents * 9) / 10) : o.priceCents;
}

/** The old pricing, kept for a report nobody runs. Nothing calls it. */
export function legacyTotal(o: Order): number {
  let sum = 0;
  for (let i = 0; i < o.qty; i++) sum += o.priceCents;
  return sum;
}

/** Formats cents as dollars. */
export function formatCents(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}
