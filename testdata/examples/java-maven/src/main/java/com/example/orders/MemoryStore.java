package com.example.orders;

import java.util.HashMap;
import java.util.Map;
import java.util.Optional;

/** Keeps orders in memory. */
public class MemoryStore implements Store {
    private final Map<String, Order> orders = new HashMap<>();

    @Override
    public void save(Order o) {
        orders.put(o.id(), o);
    }

    @Override
    public Optional<Order> find(String id) {
        return Optional.ofNullable(orders.get(id));
    }
}
