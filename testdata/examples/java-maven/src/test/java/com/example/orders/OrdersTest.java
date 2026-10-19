package com.example.orders;

import static org.junit.jupiter.api.Assertions.assertEquals;

import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;

class OrdersTest {
    @Test
    void validateRejectsZeroQty() {
        assertEquals("quantity must be at least 1", Orders.validate(new Order("o1", "A1", 0, 100)));
    }

    @Nested
    class Total {
        @Test
        void appliesBulkDiscount() {
            assertEquals(900, Orders.total(new Order("o1", "A1", 10, 100)));
        }
    }
}
