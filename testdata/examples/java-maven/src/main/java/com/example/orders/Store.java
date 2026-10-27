package com.example.orders;

import java.util.Optional;

/** Where orders are kept; OrderHandler only knows this interface. */
public interface Store {
    /** Stores an order, replacing one with the same ID. */
    void save(Order o);

    /** Loads an order by ID. */
    Optional<Order> find(String id);
}
