// Talks to the orders API. It reads the clock and the network directly,
// which is what makes it hard to test.
import type { Order } from './order';

/** Sends a new order and returns its id. */
export async function createOrder(o: Order): Promise<string> {
  const res = await fetch('/orders', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ...o, placedAt: Date.now() }),
  });
  if (!res.ok) throw new Error(`order failed: ${res.status}`);
  const body = (await res.json()) as { id: string };
  return body.id;
}
