package com.example.orders;

import java.util.HashMap;
import java.util.Map;
import java.util.Optional;

/** Keeps orders in memory. */
public class MemoryStore {
    private final Map<String, Order> orders = new HashMap<>();

    /** Stores an order, replacing one with the same ID. */
    public void save(Order o) {
        orders.put(o.id(), o);
    }

    /** Loads an order by ID. */
    public Optional<Order> find(String id) {
        return Optional.ofNullable(orders.get(id));
    }
}
