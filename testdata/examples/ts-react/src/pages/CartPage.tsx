import { Cart, summarize } from '../cart/cart';
import { format } from '../cart/format';

// The cart page; the router imports it as the module's default export.
export default function () {
  const cart = new Cart('guest');
  return (
    <section>
      <p>{summarize(cart)}</p>
      <p>{format(cart)}</p>
      <button onClick={cart.clear}>Empty the cart</button>
    </section>
  );
}
