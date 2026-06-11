package httpapi

// DayOffs implements ports.DayOffStore against the bearer API.
//
// Server scopes all reads/writes to the authenticated user.
//
// List() has no error return — errors are handled gracefully via slog.Warn and
// snapshot fallback so callers always receive a (possibly stale) result.

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// DayOffs implements ports.DayOffStore via the server bearer API.
type DayOffs struct {
	c     *Client
	cache map[int][]dayOffDTO // keyed by year
	mu    sync.Mutex
}

// NewDayOffs constructs a DayOffs adapter backed by c.
func NewDayOffs(c *Client) *DayOffs {
	d := &DayOffs{c: c, cache: make(map[int][]dayOffDTO)}
	if snap, ok := loadSnapshot(); ok && len(snap.DayOffs) > 0 {
		// Seed the cache from snapshot, grouped by year
		for _, dto := range snap.DayOffs {
			t, err := time.Parse(time.DateOnly, dto.Day)
			if err != nil {
				continue
			}
			y := t.Year()
			d.cache[y] = append(d.cache[y], dto)
		}
	}
	return d
}

var _ ports.DayOffStore = (*DayOffs)(nil)

// List returns all entries in [from, to] (inclusive). Errors are swallowed —
// the interface contract does not return an error.
func (d *DayOffs) List(from, to time.Time) []domain.DayOff {
	if from.IsZero() && to.IsZero() {
		return nil
	}
	fromYear := from.Year()
	toYear := to.Year()
	if from.IsZero() {
		fromYear = toYear
	}
	if to.IsZero() {
		toYear = fromYear
	}
	var all []dayOffDTO
	for y := fromYear; y <= toYear; y++ {
		dtos := d.fetchYear(y)
		all = append(all, dtos...)
	}
	// Filter to [from, to] range and convert
	var out []domain.DayOff
	for _, dto := range all {
		off, err := dayOffFromDTO(dto)
		if err != nil {
			slog.Warn("httpapi: malformed dayoff", "day", dto.Day, "err", err)
			continue
		}
		if (!from.IsZero() && off.Date.Before(from)) ||
			(!to.IsZero() && off.Date.After(to)) {
			continue
		}
		out = append(out, off)
	}
	return out
}

// Lookup returns the entry for a specific date.
func (d *DayOffs) Lookup(date time.Time) (domain.DayOff, bool) {
	results := d.List(date, date)
	for _, off := range results {
		if off.Date.Equal(date) {
			return off, true
		}
	}
	return domain.DayOff{}, false
}

// Add inserts or replaces the entry for the given date.
func (d *DayOffs) Add(off domain.DayOff) error {
	body := dayOffDTO{
		Day:   off.Date.Format(time.DateOnly),
		Kind:  string(off.Kind),
		Label: off.Label,
	}
	if off.Target > 0 {
		body.Target = off.Target.String()
	}
	err := d.c.doJSON(context.Background(), http.MethodPut,
		fmt.Sprintf("/api/v1/day-offs/%s", off.Date.Format(time.DateOnly)),
		body, -1, nil)
	if err != nil {
		return err
	}
	d.invalidateYear(off.Date.Year())
	return nil
}

// AddBatch inserts or replaces every entry in offs.
//
// Deviation from ports.DayOffStore contract: the server has no bulk day-off
// endpoint in Phase 1, so this is a sequential loop of individual PUT calls
// rather than an atomic all-or-nothing operation. A partial failure leaves
// already-written days in place. Callers that require atomicity (e.g.
// usecase/dayoff_writer.AddRange) should be aware of this limitation until a
// server-side bulk endpoint is added (post-R2a).
func (d *DayOffs) AddBatch(offs []domain.DayOff) error {
	var firstErr error
	for _, off := range offs {
		if err := d.Add(off); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Remove deletes the entry for the given date. A non-existent entry is a no-op.
func (d *DayOffs) Remove(date time.Time) error {
	err := d.c.doJSON(context.Background(), http.MethodDelete,
		fmt.Sprintf("/api/v1/day-offs/%s", date.Format(time.DateOnly)),
		nil, -1, nil)
	if err != nil {
		if statusCode(err) == http.StatusNotFound {
			return nil // already gone
		}
		return err
	}
	d.invalidateYear(date.Year())
	return nil
}

// — helpers -------------------------------------------------------------------

// fetchYear returns cached DTOs for year, fetching from server if necessary.
// On network error it warns and returns stale data (or empty).
func (d *DayOffs) fetchYear(year int) []dayOffDTO {
	d.mu.Lock()
	cached, ok := d.cache[year]
	d.mu.Unlock()
	if ok {
		return cached
	}
	var env itemsEnvelope[dayOffDTO]
	err := d.c.doJSON(context.Background(), http.MethodGet,
		fmt.Sprintf("/api/v1/day-offs?year=%d", year),
		nil, -1, &env)
	if err != nil {
		slog.Warn("httpapi: day-offs fetch failed — using stale data",
			"year", year, "err", err)
		// Return stale snapshot data for this year if available
		if snap, snapOK := loadSnapshot(); snapOK {
			var stale []dayOffDTO
			for _, dto := range snap.DayOffs {
				t, perr := time.Parse(time.DateOnly, dto.Day)
				if perr == nil && t.Year() == year {
					stale = append(stale, dto)
				}
			}
			return stale
		}
		return nil
	}
	d.mu.Lock()
	d.cache[year] = env.Items
	d.mu.Unlock()
	// Persist snapshot asynchronously
	go d.saveSnapshot()
	return env.Items
}

func (d *DayOffs) invalidateYear(year int) {
	d.mu.Lock()
	delete(d.cache, year)
	d.mu.Unlock()
}

func (d *DayOffs) saveSnapshot() {
	d.mu.Lock()
	var all []dayOffDTO
	for _, dtos := range d.cache {
		all = append(all, dtos...)
	}
	d.mu.Unlock()
	snap, ok := loadSnapshot()
	if !ok {
		snap = Snapshot{}
	}
	snap.DayOffs = all
	if err := saveSnapshot(snap); err != nil {
		slog.Warn("httpapi: dayoffs snapshot save failed", "err", err)
	}
}
