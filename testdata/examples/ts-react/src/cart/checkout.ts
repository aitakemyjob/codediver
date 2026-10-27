// Paying for a gift cart. The cart field is typed, so this.cart.subtotal()
// is GiftCart's subtotal, although Cart beside it has one too.
import { formatCents } from '../order';
import type { GiftCart } from './cart';

/** What's left to charge on a gift cart. */
export class Checkout {
  constructor(private cart: GiftCart) {}

  /** The amount to charge, as text. */
  due(): string {
    return formatCents(this.cart.subtotal());
  }
}
