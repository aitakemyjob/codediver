// The receipt page. Every call here goes through an import that renames
// or groups what it imports:
//   receipt.formatLine → receipt/format.ts's format (via the barrel)
//   receipt.grandTotal → receipt/totals.ts (via export *)
//   emit               → receipt/print.ts's default export
import * as receipt from '../receipt';
import emit from '../receipt/print';
import { formatCents, type Order } from '../order';

/** Lists the orders and their total, and prints them on request. */
export function ReceiptPage({ orders }: { orders: Order[] }) {
  const lines = orders.map((o) => receipt.formatLine(o));
  const sum = formatCents(receipt.grandTotal(orders));
  return (
    <section>
      {lines.map((l) => <p key={l}>{l}</p>)}
      <b>{sum}</b>
      <button onClick={() => emit([...lines, sum])}>Print</button>
    </section>
  );
}
