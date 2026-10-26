//! Handles order requests: "id=o1;sku=A1;qty=2;price=100".

use crate::notify;
use crate::order::{self, Order};
use crate::store::Store;

/// Serves order requests from a store.
pub struct Handler {
    store: Box<dyn Store>,
}

impl Handler {
    /// Creates a handler backed by store.
    pub fn new(store: impl Store + 'static) -> Self {
        Handler { store: Box::new(store) }
    }

    /// Validates, prices, stores and announces an order; returns a status line.
    pub fn create_order(&mut self, body: &str) -> String {
        let Some(o) = parse(body) else {
            return "400 bad request".into();
        };
        if let Err(problem) = order::validate(&o) {
            return format!("422 {problem}");
        }
        let total = order::total(&o);
        let _ = notify::send(&o);
        self.store.save(o);
        format!("201 total={total}")
    }

    /// Returns one order.
    pub fn get_order(&self, id: &str) -> String {
        match self.store.find(id) {
            Some(o) => format!("{o:?}"),
            None => "404 not found".into(),
        }
    }
}

/// Reads key=value pairs separated by semicolons; None if malformed.
fn parse(body: &str) -> Option<Order> {
    let mut o = Order { id: String::new(), sku: String::new(), qty: 0, price_cents: 0 };
    for part in body.split(';') {
        let (k, v) = part.split_once('=')?;
        match k {
            "id" => o.id = v.into(),
            "sku" => o.sku = v.into(),
            "qty" => o.qty = v.parse().ok()?,
            "price" => o.price_cents = v.parse().ok()?,
            _ => {}
        }
    }
    Some(o)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::store::MemoryStore;

    #[test]
    fn create_order_rejects_malformed_body() {
        let mut h = Handler::new(MemoryStore::new());
        assert_eq!(h.create_order("nonsense"), "400 bad request");
    }
}
