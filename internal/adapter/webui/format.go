// Package webui holds the templ view components for the server-rendered UI.
package webui

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

// fmtDur renders a duration as HH:MM (clamped at zero).
func fmtDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

// monthText localizes a month name through the catalog (date.month.1 … .12).
// The Historie still carries a hardcoded German month table in
// webui_historie.go (historieMonthYear) — that one has no ctx to read a
// locale from; pulling it onto this helper is its own step.
func monthText(ctx context.Context, m time.Month) string {
	return components.T(ctx, "date.month."+strconv.Itoa(int(m)))
}
