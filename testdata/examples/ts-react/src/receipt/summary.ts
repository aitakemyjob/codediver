// The cart line printed at the top of a receipt. It imports cart/format.ts's
// format, while receipt/format.ts beside it exports a format too: matching
// by name alone would pick the one in this folder.
import { format } from '../cart/format';
import type { Cart } from '../cart/cart';

/** The receipt's heading: who the cart belongs to and what's in it. */
export function cartLine(cart: Cart): string {
  return `${cart.owner}: ${format(cart)}`;
}
