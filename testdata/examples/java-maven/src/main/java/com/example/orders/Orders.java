package com.example.orders;

/** Order validation and pricing. */
public final class Orders {
    static final int BULK_MINIMUM = 10;

    private Orders() {}

    /** Checks an order before it is stored; returns an error or null. */
    public static String validate(Order o) {
        if (o.qty() <= 0) {
            return "quantity must be at least 1";
        }
        if (o.sku() == null || o.sku().isEmpty()) {
            return "sku is required";
        }
        if (o.qty() > 1000) {
            return "quantity is over the limit of 1000";
        }
        return null;
    }

    /** What the customer pays, in cents. */
    public static int total(Order o) {
        return o.qty() * unitPrice(o);
    }

    /** Applies the bulk discount. Only total calls it. */
    private static int unitPrice(Order o) {
        if (o.qty() >= BULK_MINIMUM) {
            return o.priceCents() * 9 / 10;
        }
        return o.priceCents();
    }

    /** The old pricing, kept for a report nobody runs. Nothing calls it. */
    public static int legacyTotal(Order o) {
        int sum = 0;
        for (int i = 0; i < o.qty(); i++) {
            sum += o.priceCents();
        }
        return sum;
    }
}
