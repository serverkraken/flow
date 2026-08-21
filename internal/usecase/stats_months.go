package usecase

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// Months ist die Jahressicht der Historie (Screen 32): je Monat die
// erfasste Zeit (zählende Buchungen, laufende bis jetzt) und das Soll —
// die Summe der Tagesziele über die Werktage des Monats; Frei-Tage und
// Feiertage sind keine Werktage. Monate in der Zukunft tragen ihr Soll, aber nichts
// Erfasstes. Owner-scoped wie der Rest des Rechners.
func (c StatsComputer) Months(ctx context.Context, ownerID string, year int) ([]domain.MonthLedger, error) {
	now := c.Clock.Now().In(c.loc())
	from := time.Date(year, 1, 1, 0, 0, 0, 0, c.loc())
	to := from.AddDate(1, 0, 0)
	res, _, err := c.resolver(ctx, ownerID, from, to.AddDate(0, 0, -1))
	if err != nil {
		return nil, err
	}
	countsToward, err := c.countsTowardFn(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	sessions, err := c.Sessions.List(ctx, ownerID, from)
	if err != nil {
		return nil, err
	}
	logged := make([]time.Duration, 13)
	for _, rec := range domain.BuildDayRecords(sessions, now, res.For, countsToward) {
		d := rec.Date.In(c.loc())
		if d.Year() != year {
			continue
		}
		logged[d.Month()] += rec.Total
	}
	out := make([]domain.MonthLedger, 0, 12)
	for m := time.January; m <= time.December; m++ {
		first := time.Date(year, m, 1, 0, 0, 0, 0, c.loc())
		next := first.AddDate(0, 1, 0)
		var target time.Duration
		for d := first; d.Before(next); d = d.AddDate(0, 0, 1) {
			if res.IsWorkday(d) {
				target += res.For(d)
			}
		}
		out = append(out, domain.MonthLedger{
			Month:   first,
			Logged:  logged[m],
			Target:  target,
			Current: now.Year() == year && now.Month() == m,
			Future:  first.After(now),
		})
	}
	return out, nil
}
