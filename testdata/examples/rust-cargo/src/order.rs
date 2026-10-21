//! The order model: validation and pricing.

/// One customer order.
#[derive(Clone, Debug, PartialEq)]
pub struct Order {
    pub id: String,
    pub sku: String,
    pub qty: u32,
    pub price_cents: u32,
}

const BULK_MINIMUM: u32 = 10;

/// Checks an order before it is stored; returns an error message.
pub fn validate(o: &Order) -> Result<(), String> {
    if o.qty == 0 {
        return Err("quantity must be at least 1".into());
    }
    if o.sku.is_empty() {
        return Err("sku is required".into());
    }
    if o.qty > 1000 {
        return Err("quantity is over the limit of 1000".into());
    }
    Ok(())
}

/// What the customer pays, in cents.
pub fn total(o: &Order) -> u32 {
    o.qty * unit_price(o)
}

/// Applies the bulk discount. Only total calls it.
fn unit_price(o: &Order) -> u32 {
    if o.qty >= BULK_MINIMUM {
        o.price_cents * 9 / 10
    } else {
        o.price_cents
    }
}

/// The old pricing, kept for a report nobody runs. Nothing calls it.
pub fn legacy_total(o: &Order) -> u32 {
    let mut sum = 0;
    for _ in 0..o.qty {
        sum += o.price_cents;
    }
    sum
}

#[cfg(test)]
mod tests {
    use super::*;

    fn order(qty: u32) -> Order {
        Order { id: "o1".into(), sku: "A1".into(), qty, price_cents: 100 }
    }

    #[test]
    fn validate_rejects_zero_qty() {
        assert_eq!(validate(&order(0)), Err("quantity must be at least 1".into()));
    }

    #[test]
    fn total_applies_bulk_discount() {
        assert_eq!(total(&order(10)), 900);
    }
}
