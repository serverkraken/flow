package testutil

import (
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// FakeDayOffStore is an in-memory ports.DayOffStore keyed by YYYY-MM-DD.
type FakeDayOffStore struct {
	Entries map[string]domain.DayOff
	Err     error // returned by Add/Remove when non-nil
}

// NewFakeDayOffStore constructs a store seeded with the given entries.
func NewFakeDayOffStore(entries ...domain.DayOff) *FakeDayOffStore {
	s := &FakeDayOffStore{Entries: map[string]domain.DayOff{}}
	for _, e := range entries {
		s.Entries[e.Date.Format("2006-01-02")] = e
	}
	return s
}

func (f *FakeDayOffStore) List(from, to time.Time) []domain.DayOff {
	out := make([]domain.DayOff, 0, len(f.Entries))
	for _, d := range f.Entries {
		if !from.IsZero() && d.Date.Before(from) {
			continue
		}
		if !to.IsZero() && d.Date.After(to) {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out
}

func (f *FakeDayOffStore) Lookup(date time.Time) (domain.DayOff, bool) {
	d, ok := f.Entries[date.Format("2006-01-02")]
	return d, ok
}

func (f *FakeDayOffStore) Add(off domain.DayOff) error {
	if f.Err != nil {
		return f.Err
	}
	if f.Entries == nil {
		f.Entries = map[string]domain.DayOff{}
	}
	f.Entries[off.Date.Format("2006-01-02")] = off
	return nil
}

// AddBatch mirrors the production all-or-nothing contract: if Err is
// set, no entry is written. Otherwise every entry lands.
func (f *FakeDayOffStore) AddBatch(offs []domain.DayOff) error {
	if f.Err != nil {
		return f.Err
	}
	if len(offs) == 0 {
		return nil
	}
	if f.Entries == nil {
		f.Entries = map[string]domain.DayOff{}
	}
	for _, off := range offs {
		f.Entries[off.Date.Format("2006-01-02")] = off
	}
	return nil
}

func (f *FakeDayOffStore) Remove(date time.Time) error {
	if f.Err != nil {
		return f.Err
	}
	delete(f.Entries, date.Format("2006-01-02"))
	return nil
}
