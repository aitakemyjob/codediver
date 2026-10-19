package store

import (
	"testing"

	"example.com/orders/order"
)

func TestMemorySaveAndGet(t *testing.T) {
	m := NewMemory()
	if err := m.Save(order.Order{ID: "o1", SKU: "A1", Qty: 2}); err != nil {
		t.Fatal(err)
	}
	if o, err := m.Get("o1"); err != nil || o.Qty != 2 {
		t.Fatalf("get = %+v, %v", o, err)
	}
}
