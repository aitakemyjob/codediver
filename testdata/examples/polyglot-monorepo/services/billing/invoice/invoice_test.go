package invoice

import "testing"

func TestTotalAddsEUTax(t *testing.T) {
	got, err := Total(Invoice{Cents: 1000, Region: "EU"})
	if err != nil || got != 1200 {
		t.Fatalf("total = %d, %v", got, err)
	}
}
