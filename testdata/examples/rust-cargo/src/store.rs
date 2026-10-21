//! Keeps orders in memory.

use std::collections::HashMap;

use crate::order::Order;

/// An in-memory order store.
#[derive(Default)]
pub struct MemoryStore {
    orders: HashMap<String, Order>,
}

impl MemoryStore {
    /// Creates an empty store.
    pub fn new() -> Self {
        Self::default()
    }

    /// Stores an order, replacing one with the same ID.
    pub fn save(&mut self, o: Order) {
        self.orders.insert(o.id.clone(), o);
    }

    /// Loads an order by ID.
    pub fn find(&self, id: &str) -> Option<&Order> {
        self.orders.get(id)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn save_and_find() {
        let mut s = MemoryStore::new();
        s.save(Order { id: "o1".into(), sku: "A1".into(), qty: 2, price_cents: 100 });
        assert_eq!(s.find("o1").map(|o| o.qty), Some(2));
    }
}
