import { useState } from 'react';
import { OrderForm } from './components/OrderForm';
import { OrderSummary } from './components/OrderSummary';
import type { Order } from './order';

/** The page: a form, then a summary of what was placed. */
export function App() {
  const [placed, setPlaced] = useState<Order[]>([]);
  return (
    <main>
      <OrderForm onPlaced={(id) => setPlaced([...placed, { id, sku: '', qty: 0, priceCents: 0 }])} />
      {placed.map((o) => <OrderSummary key={o.id} order={o} />)}
    </main>
  );
}
