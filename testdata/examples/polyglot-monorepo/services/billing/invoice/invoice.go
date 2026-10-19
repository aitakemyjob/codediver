// Package invoice prices invoices for the billing service.
package invoice

import "errors"

// Invoice is a bill for one customer.
type Invoice struct {
	ID       string `json:"id"`
	Customer string `json:"customer"`
	Cents    int    `json:"cents"`
	Region   string `json:"region"`
	Void     bool   `json:"void"`
}

// ErrVoid is returned when pricing a voided invoice.
var ErrVoid = errors.New("invoice is void")

// Total is the amount due including tax, in cents.
func Total(inv Invoice) (int, error) {
	if inv.Void {
		return 0, ErrVoid
	}
	return inv.Cents + applyTax(inv), nil
}

// applyTax is the tax for the invoice's region. Only Total calls it.
func applyTax(inv Invoice) int {
	switch inv.Region {
	case "EU":
		return inv.Cents * 20 / 100
	case "US":
		return inv.Cents * 8 / 100
	}
	return 0
}

// MarkVoid voids an invoice. No test covers it.
func MarkVoid(inv *Invoice) {
	inv.Void = true
}
