package com.example.orders;

/** One customer order. */
public record Order(String id, String sku, int qty, int priceCents) {}
