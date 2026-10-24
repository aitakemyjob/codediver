/** Sends receipt lines to the printer. The module's default export. */
export default function printReceipt(lines: string[]): void {
  window.print();
  console.log(lines.join('\n'));
}
