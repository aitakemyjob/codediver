package com.example.orders;

import static org.junit.jupiter.api.Assertions.assertEquals;

import org.junit.jupiter.api.Test;

class OrderHandlerTest {
    @Test
    void createOrderRejectsMalformedBody() throws Exception {
        assertEquals("400 bad request", new OrderHandler(new MemoryStore()).createOrder("nonsense"));
    }
}
