package testutil

import (
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// FakeKompendiumGateway returns canned data without touching the
// kompendium binary. Tests set Notes / Paths / DailyDates directly.
type FakeKompendiumGateway struct {
	Notes      []domain.KompendiumNote
	Paths      map[string]string // id → fs path
	DailyDates map[string]bool   // YYYY-MM-DD → exists?
	ListErr    error
}

func (f *FakeKompendiumGateway) DailyExists(date time.Time) bool {
	return f.DailyDates[date.Format("2006-01-02")]
}

func (f *FakeKompendiumGateway) List() ([]domain.KompendiumNote, error) {
	if f.ListErr != nil {
		return nil, f.ListErr
	}
	out := make([]domain.KompendiumNote, len(f.Notes))
	copy(out, f.Notes)
	return out, nil
}

func (f *FakeKompendiumGateway) ResolvePath(id string) (string, error) {
	return f.Paths[id], nil
}

// FakeNoteLauncher records every Open/View invocation as the note ID,
// prefixed with "open:" or "view:" so a single Calls slice tells the test
// what happened in order.
type FakeNoteLauncher struct {
	Calls []string
	Err   error
}

func (f *FakeNoteLauncher) Open(id string) error {
	f.Calls = append(f.Calls, "open:"+id)
	return f.Err
}

func (f *FakeNoteLauncher) View(id string) error {
	f.Calls = append(f.Calls, "view:"+id)
	return f.Err
}
