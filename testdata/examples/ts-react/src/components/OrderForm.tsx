import { useState } from 'react';
import { createOrder } from '../api';
import { validate, type Order } from '../order';

/** Collects a new order and submits it. No test renders it. */
export function OrderForm({ onPlaced }: { onPlaced: (id: string) => void }) {
  const [order, setOrder] = useState<Order>({ id: '', sku: '', qty: 1, priceCents: 0 });
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    const problem = validate(order);
    setError(problem);
    if (!problem) onPlaced(await createOrder(order));
  };

  return (
    <form onSubmit={(e) => { e.preventDefault(); void submit(); }}>
      <input value={order.sku} onChange={(e) => setOrder({ ...order, sku: e.target.value })} placeholder="SKU" />
      <input type="number" value={order.qty} onChange={(e) => setOrder({ ...order, qty: Number(e.target.value) })} />
      {error && <p role="alert">{error}</p>}
      <button type="submit">Place order</button>
    </form>
  );
}
