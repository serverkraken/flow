// internal/adapter/pgstore/dayoffs_test.go
package pgstore_test

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
)

func TestDayOffs_PutListDelete(t *testing.T) {
	t.Parallel()
	d := pgstore.NewDayOffs(testStore)
	uid := mustUser(t, "dayoff-1")

	day := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	off := domain.DayOff{Date: day, Kind: domain.KindVacation, Label: "Sommer", Target: 0}
	if err := d.Put(uid, off); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// Put ist Upsert: Kind ändern
	off.Kind = domain.KindSick
	if err := d.Put(uid, off); err != nil {
		t.Fatalf("Put upsert: %v", err)
	}

	list, err := d.List(uid, 2026)
	if err != nil || len(list) != 1 {
		t.Fatalf("List: err=%v len=%d", err, len(list))
	}
	if list[0].Kind != domain.KindSick || list[0].Label != "Sommer" {
		t.Errorf("roundtrip: %+v", list[0])
	}

	other, _ := d.List(uid, 2025)
	if len(other) != 0 {
		t.Errorf("List 2025 should be empty, got %d", len(other))
	}

	if err := d.Delete(uid, day); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list, _ = d.List(uid, 2026)
	if len(list) != 0 {
		t.Errorf("after Delete: %d entries", len(list))
	}
	// Delete ist idempotent
	if err := d.Delete(uid, day); err != nil {
		t.Errorf("Delete idempotent: %v", err)
	}
}
