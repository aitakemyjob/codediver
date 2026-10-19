package com.example.orders;

import static org.junit.jupiter.api.Assertions.assertEquals;

import org.junit.jupiter.api.Test;

class MemoryStoreTest {
    @Test
    void saveAndFind() {
        MemoryStore s = new MemoryStore();
        s.save(new Order("o1", "A1", 2, 100));
        assertEquals(2, s.find("o1").orElseThrow().qty());
    }
}
