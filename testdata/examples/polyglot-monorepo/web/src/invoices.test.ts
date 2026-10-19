import { expect, it } from 'vitest';
import { formatInvoice } from './invoices';

it('formats an open invoice', () => {
  expect(formatInvoice({ id: 'i1', customer: 'Ada', cents: 1200, region: 'EU', void: false })).toBe('i1 Ada $12.00');
});
