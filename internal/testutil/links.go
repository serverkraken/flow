package testutil

import (
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.LinkStore = (*FakeLinkStore)(nil)

// FakeLinkStore is an in-memory ports.LinkStore keyed by YYYY-MM-DD.
// Insertion order per date is preserved (the slice is appended to).
type FakeLinkStore struct {
	ByDate map[string][]string
	Err    error // returned by every method when non-nil
}

func (f *FakeLinkStore) ListByDate(date time.Time) ([]string, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	ids := f.ByDate[date.Format("2006-01-02")]
	out := make([]string, len(ids))
	copy(out, ids)
	return out, nil
}

func (f *FakeLinkStore) Add(date time.Time, noteID string) error {
	if f.Err != nil {
		return f.Err
	}
	if f.ByDate == nil {
		f.ByDate = map[string][]string{}
	}
	key := date.Format("2006-01-02")
	for _, id := range f.ByDate[key] {
		if id == noteID {
			return nil // idempotent
		}
	}
	f.ByDate[key] = append(f.ByDate[key], noteID)
	return nil
}

func (f *FakeLinkStore) CountsByDate() (map[string]int, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	out := make(map[string]int, len(f.ByDate))
	for k, ids := range f.ByDate {
		if n := len(ids); n > 0 {
			out[k] = n
		}
	}
	return out, nil
}

func (f *FakeLinkStore) Remove(date time.Time, noteID string) error {
	if f.Err != nil {
		return f.Err
	}
	key := date.Format("2006-01-02")
	ids := f.ByDate[key]
	for i, id := range ids {
		if id == noteID {
			f.ByDate[key] = append(ids[:i], ids[i+1:]...)
			return nil
		}
	}
	return nil // idempotent: removing missing is no-op
}
