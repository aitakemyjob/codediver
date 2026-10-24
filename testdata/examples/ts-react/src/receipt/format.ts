import { formatCents, total, type Order } from '../order';

/**
 * One receipt line. Same name as cart/format.ts's format: only the import
 * tells a caller which one it means.
 */
export function format(o: Order): string {
  return `${o.qty} × ${o.sku}  ${formatCents(total(o))}`;
}
