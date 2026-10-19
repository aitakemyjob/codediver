import { describe, expect, it } from 'vitest';
import { total, validate } from './order';

describe('validate', () => {
  it('rejects a zero quantity', () => {
    expect(validate({ id: 'o1', sku: 'A1', qty: 0, priceCents: 100 })).toBe('quantity must be at least 1');
  });
});

describe('total', () => {
  it('applies the bulk discount', () => {
    expect(total({ id: 'o1', sku: 'A1', qty: 10, priceCents: 100 })).toBe(900);
  });
});
