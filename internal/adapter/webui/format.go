// Package webui holds the templ view components for the server-rendered UI.
package webui

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// fmtClock renders a running clock the way the mockups write it: hours
// unpadded, minutes padded, no unit — "2:41", not "02:41" and not "2:41 h".
// The rail still appends " h" via FmtDurHMExport; aligning it is its own pass.
func fmtClock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

// monthText localizes a month name through the catalog (date.month.1 … .12).
// The Historie still carries a hardcoded German month table in
// webui_historie.go (historieMonthYear) — that one has no ctx to read a
// locale from; pulling it onto this helper is its own step.
func monthText(ctx context.Context, m time.Month) string {
	return components.T(ctx, "date.month."+strconv.Itoa(int(m)))
}

// RateLabel formats an optional per-hour rate as "95 €/h" or "—". It lives in
// webui because both the view models and the handlers need it; the handler's
// rateLabel delegates here so there is one formatting rule, not two.
func RateLabel(rate *domain.Money) string {
	if rate == nil {
		return "—"
	}
	sym := rate.Currency
	if rate.Currency == "EUR" {
		sym = "€"
	}
	return fmt.Sprintf("%d %s/h", rate.Amount/100, sym)
}
