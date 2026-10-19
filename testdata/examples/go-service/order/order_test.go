package order

import "testing"

func TestValidateRejectsZeroQty(t *testing.T) {
	if err := Validate(Order{SKU: "A1", Qty: 0}); err != ErrQty {
		t.Fatalf("err = %v", err)
	}
}

func TestTotalAppliesBulkDiscount(t *testing.T) {
	if got := Total(Order{Qty: 10, PriceCents: 100}); got != 900 {
		t.Fatalf("total = %d", got)
	}
}
