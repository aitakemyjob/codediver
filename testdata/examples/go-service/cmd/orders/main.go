// Command orders runs the orders API.
package main

import (
	"log"
	"net/http"

	"example.com/orders/api"
	"example.com/orders/store"
)

// main serves the API on :8080.
func main() {
	h := api.New(store.NewMemory())
	http.HandleFunc("POST /orders", h.CreateOrder)
	http.HandleFunc("GET /orders/{id}", h.GetOrder)
	http.HandleFunc("GET /orders", h.ListOrders)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
