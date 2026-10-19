import { formatCents, total, type Order } from '../order';

/**
 * One order's line and total. App renders it only through JSX
 * (<OrderSummary />), never by calling it.
 */
export function OrderSummary({ order }: { order: Order }) {
  return (
    <div className="summary">
      <span>{order.qty} × {order.sku}</span>
      <b>{formatCents(total(order))}</b>
    </div>
  );
}
