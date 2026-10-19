package com.example.orders;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Instant;

/**
 * Tells the warehouse about new orders. It reads the clock, the environment
 * and the network directly, which is what makes it hard to test.
 */
public final class Notifier {
    private Notifier() {}

    /** Posts an order to the warehouse webhook. */
    public static void send(Order o) throws Exception {
        String url = System.getenv("WAREHOUSE_URL");
        if (url == null) {
            return;
        }
        String body = "{\"id\":\"" + o.id() + "\",\"sentAt\":\"" + Instant.now() + "\"}";
        HttpRequest req = HttpRequest.newBuilder(URI.create(url))
                .header("Content-Type", "application/json")
                .POST(HttpRequest.BodyPublishers.ofString(body))
                .build();
        HttpClient.newHttpClient().send(req, HttpResponse.BodyHandlers.discarding());
    }
}
