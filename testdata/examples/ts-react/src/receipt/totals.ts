// Renames on import: orderTotal is order.ts's total.
import { total as orderTotal, type Order } from '../order';

/** What a set of orders costs together, in cents. */
export function grandTotal(orders: Order[]): number {
  return orders.reduce((sum, o) => sum + orderTotal(o), 0);
}
