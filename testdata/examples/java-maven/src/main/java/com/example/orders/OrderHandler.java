package com.example.orders;

/** Handles order requests: "id=o1;sku=A1;qty=2;price=100". */
public class OrderHandler {
    private final Store store;

    /** Creates a handler backed by store. */
    public OrderHandler(Store store) {
        this.store = store;
    }

    /** Validates, prices, stores and announces an order; returns a status line. */
    public String createOrder(String body) throws Exception {
        Order o = parse(body);
        if (o == null) {
            return "400 bad request";
        }
        String problem = Orders.validate(o);
        if (problem != null) {
            return "422 " + problem;
        }
        store.save(o);
        Notifier.send(o);
        return "201 total=" + Orders.total(o);
    }

    /** Returns one order. */
    public String getOrder(String id) {
        return store.find(id).map(Order::toString).orElse("404 not found");
    }

    /** Reads key=value pairs separated by semicolons; null if malformed. */
    private static Order parse(String body) {
        String id = null, sku = null;
        int qty = 0, price = 0;
        for (String part : body.split(";")) {
            String[] kv = part.split("=", 2);
            if (kv.length != 2) {
                return null;
            }
            switch (kv[0]) {
                case "id" -> id = kv[1];
                case "sku" -> sku = kv[1];
                case "qty" -> qty = Integer.parseInt(kv[1]);
                case "price" -> price = Integer.parseInt(kv[1]);
                default -> { }
            }
        }
        return new Order(id, sku, qty, price);
    }
}
