package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// fakeDayOffs is an in-memory DayOffStore keyed by (owner, yyyy-mm-dd).
type fakeDayOffs struct{ m map[string]domain.DayOff }

func newFakeDayOffs() *fakeDayOffs           { return &fakeDayOffs{m: map[string]domain.DayOff{}} }
func key(owner string, day time.Time) string { return owner + ":" + day.Format("2006-01-02") }

func (f *fakeDayOffs) Add(_ context.Context, owner string, d domain.DayOff) error {
	f.m[key(owner, d.Date)] = d
	return nil
}
func (f *fakeDayOffs) Delete(_ context.Context, owner string, day time.Time) error {
	delete(f.m, key(owner, day))
	return nil
}
func (f *fakeDayOffs) ListRange(_ context.Context, owner string, from, to time.Time) ([]domain.DayOff, error) {
	var out []domain.DayOff
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if v, ok := f.m[key(owner, d)]; ok {
			out = append(out, v)
		}
	}
	return out, nil
}

type fakeSettings struct{ land string }

func (f fakeSettings) Get(context.Context, string) (domain.Settings, error) {
	return domain.Settings{Bundesland: f.land}, nil
}
func (f fakeSettings) SetBundesland(context.Context, string, string) error { return nil }
func (f fakeSettings) SetTargetConfig(context.Context, string, int, map[time.Weekday]int) error {
	return nil
}

type recEmitter struct{ events []domain.Event }

func (e *recEmitter) Emit(_ context.Context, ev domain.Event) { e.events = append(e.events, ev) }

func TestAddDayOffs_ExpandsAndPublishesOnce(t *testing.T) {
	store := newFakeDayOffs()
	emitter := &recEmitter{}
	uc := usecase.AddDayOffs{Store: store, Emitter: emitter}
	from := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC) // Mon
	to := time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)   // Fri
	if err := uc.Execute(context.Background(), "u1", from, to, domain.KindVacation, "Sommer", 0, true); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(store.m) != 5 {
		t.Fatalf("want 5 stored days, got %d", len(store.m))
	}
	if len(emitter.events) != 1 || emitter.events[0].Type != domain.EventDayOffChanged {
		t.Fatalf("want exactly one dayoff.changed, got %+v", emitter.events)
	}
}

func TestAddDayOffs_RejectsHolidayKind(t *testing.T) {
	uc := usecase.AddDayOffs{Store: newFakeDayOffs(), Emitter: &recEmitter{}}
	d := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	if err := uc.Execute(context.Background(), "u1", d, d, domain.KindHoliday, "", 0, false); err == nil {
		t.Fatal("holiday kind must be rejected (holidays are computed)")
	}
}

func TestListDayOffs_MergesComputedHolidays(t *testing.T) {
	store := newFakeDayOffs()
	_ = store.Add(context.Background(), "u1", domain.DayOff{
		Date: time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local), Kind: domain.KindVacation,
	})
	uc := usecase.ListDayOffs{Store: store, Settings: fakeSettings{land: "NW"}, Loc: time.Local}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 12, 31, 0, 0, 0, 0, time.Local)
	got, err := uc.Execute(context.Background(), "u1", from, to)
	if err != nil {
		t.Fatal(err)
	}
	var vac, hol int
	for _, d := range got {
		switch d.Kind {
		case domain.KindVacation:
			vac++
		case domain.KindHoliday:
			hol++
		}
	}
	if vac != 1 || hol == 0 {
		t.Fatalf("want 1 vacation + NRW holidays merged, got vac=%d hol=%d", vac, hol)
	}
}

func TestAddDayOffs_RejectsInvalidKind(t *testing.T) {
	uc := usecase.AddDayOffs{Store: newFakeDayOffs(), Emitter: &recEmitter{}}
	d := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	if err := uc.Execute(context.Background(), "u1", d, d, domain.Kind("bogus"), "", 0, false); err == nil {
		t.Fatal("bogus kind must be rejected")
	}
}

func TestAddDayOffs_RejectsEmptyExpansion(t *testing.T) {
	store := newFakeDayOffs()
	emitter := &recEmitter{}
	uc := usecase.AddDayOffs{Store: store, Emitter: emitter}
	sat := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC) // Saturday
	if err := uc.Execute(context.Background(), "u1", sat, sat, domain.KindVacation, "", 0, true); err == nil {
		t.Fatal("empty expansion (single weekend day, skipWeekends) must be rejected")
	}
	if len(store.m) != 0 || len(emitter.events) != 0 {
		t.Fatalf("nothing should be stored/emitted on empty expansion: stored=%d events=%d", len(store.m), len(emitter.events))
	}
}

func TestAddDayOffs_RejectsUnboundedRangeBeforeWriting(t *testing.T) {
	store := newFakeDayOffs()
	emitter := &recEmitter{}
	uc := usecase.AddDayOffs{Store: store, Emitter: emitter}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC)

	if err := uc.Execute(context.Background(), "u1", from, to, domain.KindVacation, "", 0, false); err == nil {
		t.Fatal("unbounded day-off range must be rejected")
	}
	if len(store.m) != 0 || len(emitter.events) != 0 {
		t.Fatalf("rejected range mutated state: stored=%d events=%d", len(store.m), len(emitter.events))
	}
}

func TestDeleteDayOff_RemovesAndEmits(t *testing.T) {
	store := newFakeDayOffs()
	emitter := &recEmitter{}
	day := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	_ = store.Add(context.Background(), "u1", domain.DayOff{Date: day, Kind: domain.KindVacation})
	uc := usecase.DeleteDayOff{Store: store, Emitter: emitter}
	if err := uc.Execute(context.Background(), "u1", day); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(store.m) != 0 {
		t.Fatalf("want entry removed, got %d", len(store.m))
	}
	if len(emitter.events) != 1 || emitter.events[0].Type != domain.EventDayOffChanged {
		t.Fatalf("want exactly one dayoff.changed, got %+v", emitter.events)
	}
}
