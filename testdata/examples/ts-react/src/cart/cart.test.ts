import { describe, expect, it } from 'vitest';
import { Cart, GiftCart, summarize } from './cart';

const order = { id: 'o1', sku: 'A1', qty: 2, priceCents: 400 };

describe('GiftCart', () => {
  it('takes the gift card off the subtotal', () => {
    const cart = new GiftCart('ana');
    cart.add(order);
    expect(cart.subtotal()).toBe(300);
  });
});

describe('summarize', () => {
  it('formats a cart without a gift card', () => {
    const cart = new Cart('ana');
    cart.add(order);
    expect(summarize(cart)).toBe('$8.00');
  });
});
