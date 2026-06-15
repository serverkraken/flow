package domain

import (
	"fmt"
	"time"
)

// Money is an amount in integer minor units (e.g. cents) plus an ISO-4217
// currency. Used as a per-hour rate on Project and as the derived Σh×Satz
// amount in exports. Integer math avoids float rounding drift.
type Money struct {
	Amount   int64  `json:"amount"`   // minor units (cents)
	Currency string `json:"currency"` // ISO-4217, e.g. "EUR"
}

// Mul returns the cost of duration d at this per-hour rate, rounded half-up to
// the nearest minor unit (amounts are non-negative; rates are validated >= 0).
// (Amount is per-hour minor units.)
func (m Money) Mul(d time.Duration) Money {
	secs := int64(d / time.Second)
	total := (m.Amount*secs + 1800) / 3600 // round-half-up over 3600s/h
	return Money{Amount: total, Currency: m.Currency}
}

// String formats the amount as major.minor + currency, assuming a 2-decimal
// minor unit (the common case: EUR/USD/…). e.g. "4800.00 EUR".
func (m Money) String() string {
	a, sign := m.Amount, ""
	if a < 0 {
		sign, a = "-", -a
	}
	return fmt.Sprintf("%s%d.%02d %s", sign, a/100, a%100, m.Currency)
}
