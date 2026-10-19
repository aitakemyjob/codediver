// Shows invoices from the billing service.

export interface Invoice {
  id: string;
  customer: string;
  cents: number;
  region: string;
  void: boolean;
}

/** One line of the invoice list. */
export function formatInvoice(inv: Invoice): string {
  if (inv.void) return `${inv.id} (void)`;
  return `${inv.id} ${inv.customer} ${dollars(inv.cents)}`;
}

/** Formats cents as dollars. Only formatInvoice calls it. */
function dollars(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

/** Loads invoices from the API. It uses the network, so no test runs it. */
export async function fetchInvoices(): Promise<Invoice[]> {
  const res = await fetch(`/api/invoices?since=${Date.now() - 86_400_000}`);
  return (await res.json()) as Invoice[];
}
