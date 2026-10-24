import { formatCents } from '../order';
import type { Cart } from './cart';

/** One line for the cart page: item count and subtotal. */
export function format(cart: Cart): string {
  return `${cart.count()} items, ${formatCents(cart.subtotal())}`;
}
