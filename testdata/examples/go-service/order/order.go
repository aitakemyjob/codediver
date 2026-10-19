// Package order is the order model: validation and pricing.
package order

import "errors"

// Order is one customer order.
type Order struct {
	ID         string `json:"id"`
	SKU        string `json:"sku"`
	Qty        int    `json:"qty"`
	PriceCents int    `json:"priceCents"`
}

var (
	ErrQty      = errors.New("quantity must be at least 1")
	ErrSKU      = errors.New("sku is required")
	ErrTooMany  = errors.New("quantity is over the limit of 1000")
	bulkMinimum = 10
)

// Validate checks an order before it is stored.
func Validate(o Order) error {
	if o.Qty <= 0 {
		return ErrQty
	}
	if o.SKU == "" {
		return ErrSKU
	}
	if o.Qty > 1000 {
		return ErrTooMany
	}
	return nil
}

// Total is what the customer pays, in cents.
func Total(o Order) int {
	return o.Qty * unitPrice(o)
}

// unitPrice applies the bulk discount. Only Total calls it.
func unitPrice(o Order) int {
	if o.Qty >= bulkMinimum {
		return o.PriceCents * 9 / 10
	}
	return o.PriceCents
}

// LegacyTotal is the old pricing, kept for a report nobody runs anymore.
// Nothing calls it and no test covers it.
func LegacyTotal(o Order) int {
	total := 0
	for i := 0; i < o.Qty; i++ {
		total += o.PriceCents
	}
	return total
}
