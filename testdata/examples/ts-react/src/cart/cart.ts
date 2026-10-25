// The shopping cart: a class with a gift-card subclass.
import {
  formatCents,
  total,
  type Order,
} from '../order';

/** Orders waiting to be placed. */
export class Cart {
  protected items: Order[] = [];

  constructor(readonly owner: string) { this.reset() }

  reset() { this.items = [] }

  add(o: Order) { this.items.push(o); return this.count() }

  count() { return this.items.length }

  /** What the cart costs before any gift card. */
  subtotal(): number {
    return this.items.reduce((sum, o) => sum + total(o), 0);
  }

  // Bound once, so it can be passed straight to onClick.
  clear = () => {
    this.reset();
  };
}

/** A cart paid partly by a $5 gift card. */
export class GiftCart extends Cart {
  subtotal(): number {
    return Math.max(0, super.subtotal() - 500);
  }
}

/** The cart's total as text, or $0.00 without a cart. */
export function summarize(cart?: Cart): string {
  return formatCents(cart?.subtotal() ?? 0);
}

/**
 * A gift cart's total as text. cart is a GiftCart, so subtotal() is the
 * one with the gift card taken off, not Cart's above.
 */
export function giftSummary(cart: GiftCart): string {
  return formatCents(cart.subtotal());
}
