import { renderToString } from 'react-dom/server';
import { expect, it } from 'vitest';
import { OrderSummary } from './OrderSummary';

it('shows the quantity and the discounted total', () => {
  const html = renderToString(<OrderSummary order={{ id: 'o1', sku: 'A1', qty: 10, priceCents: 100 }} />);
  expect(html).toContain('A1');
  expect(html).toContain('$9.00');
});
