package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestEditSession(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	stop := start.Add(2 * time.Hour)
	if _, err := ss.Create(ctx, domain.WorkSession{ID: "s1", OwnerID: "u1", Start: start, Stop: &stop}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	uc := usecase.EditSession{Sessions: ss, Tags: testutil.NewFakeTagStore()}

	newStop := start.Add(3 * time.Hour)
	deepTags := []string{"deep"}
	got, err := uc.Execute(ctx, "u1", "s1", usecase.EditSessionInput{Tags: &deepTags, Note: "n", Start: start, Stop: &newStop})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "deep" || got.Stop == nil || !got.Stop.Equal(newStop) {
		t.Fatalf("edit not applied: %+v", got)
	}

	// stop <= start -> ErrStopBeforeStart
	bad := start.Add(-time.Minute)
	if _, err := uc.Execute(ctx, "u1", "s1", usecase.EditSessionInput{Start: start, Stop: &bad}); !errors.Is(err, domain.ErrStopBeforeStart) {
		t.Fatalf("want ErrStopBeforeStart, got %v", err)
	}
	// foreign owner -> not found (store-enforced)
	if _, err := uc.Execute(ctx, "other", "s1", usecase.EditSessionInput{Start: start, Stop: &newStop}); err == nil {
		t.Fatal("foreign edit should fail")
	}
}

func TestEditSession_RejectsOverlap(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	// existing other session 09:00–11:00
	aStop := time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC)
	if _, err := ss.Create(ctx, domain.WorkSession{ID: "a", OwnerID: "u1",
		Start: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC), Stop: &aStop}); err != nil {
		t.Fatalf("seed a: %v", err)
	}
	// session under edit, currently 13:00–14:00
	bStop := time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC)
	if _, err := ss.Create(ctx, domain.WorkSession{ID: "b", OwnerID: "u1",
		Start: time.Date(2026, 6, 15, 13, 0, 0, 0, time.UTC), Stop: &bStop}); err != nil {
		t.Fatalf("seed b: %v", err)
	}
	uc := usecase.EditSession{Sessions: ss}
	// move b onto a → overlap
	newStart := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	newStop := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	if _, err := uc.Execute(ctx, "u1", "b", usecase.EditSessionInput{Start: newStart, Stop: &newStop}); !errors.Is(err, domain.ErrOverlap) {
		t.Fatalf("want ErrOverlap, got %v", err)
	}
}

func TestEditSession_NotFound(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	uc := usecase.EditSession{Sessions: ss}
	start := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	stop := start.Add(time.Hour)
	_, err := uc.Execute(ctx, "u1", "does-not-exist", usecase.EditSessionInput{Start: start, Stop: &stop})
	if !errors.Is(err, ports.ErrSessionNotFound) {
		t.Fatalf("want ErrSessionNotFound, got %v", err)
	}
}

func TestEditSession_OverlapWithRunningOutsideWindow(t *testing.T) {
	// Regression: a running session (Stop==nil) that started more than 24h before
	// the edited session's target day is not returned by ListRange, so without the
	// explicit Running() check HasOverlap would miss it.
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	// Running session "r" started 2026-06-12 08:00 — well outside the ListRange window.
	if _, err := ss.Create(ctx, domain.WorkSession{
		ID: "r", OwnerID: "u1",
		Start: time.Date(2026, 6, 12, 8, 0, 0, 0, time.UTC), Stop: nil,
	}); err != nil {
		t.Fatalf("seed running: %v", err)
	}
	// Session "b" currently at 13:00–14:00 on 2026-06-15.
	bStop := time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC)
	if _, err := ss.Create(ctx, domain.WorkSession{
		ID: "b", OwnerID: "u1",
		Start: time.Date(2026, 6, 15, 13, 0, 0, 0, time.UTC), Stop: &bStop,
	}); err != nil {
		t.Fatalf("seed b: %v", err)
	}
	uc := usecase.EditSession{Sessions: ss}
	// Edit "b" into 09:00–10:00 — inside the interval spanned by the running session "r".
	newStart := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	newStop := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	if _, err := uc.Execute(ctx, "u1", "b", usecase.EditSessionInput{Start: newStart, Stop: &newStop}); !errors.Is(err, domain.ErrOverlap) {
		t.Fatalf("want ErrOverlap (running session spans target interval), got %v", err)
	}
}

func TestEditSession_NoSelfOverlap(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	bStop := time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC)
	if _, err := ss.Create(ctx, domain.WorkSession{ID: "b", OwnerID: "u1",
		Start: time.Date(2026, 6, 15, 13, 0, 0, 0, time.UTC), Stop: &bStop}); err != nil {
		t.Fatalf("seed b: %v", err)
	}
	uc := usecase.EditSession{Sessions: ss}
	// edit b's note but keep overlapping times — must NOT report self-overlap
	if _, err := uc.Execute(ctx, "u1", "b", usecase.EditSessionInput{
		Note:  "updated",
		Start: time.Date(2026, 6, 15, 13, 0, 0, 0, time.UTC), Stop: &bStop,
	}); err != nil {
		t.Fatalf("self-edit should succeed, got %v", err)
	}
}

// TestEditSession_NilTagsPreserve is a regression test for the tri-state Tags
// field: nil must leave a session's taggings untouched so that the TUI
// adjust-start path (which passes Tags: nil) does not wipe existing tags.
func TestEditSession_NilTagsPreserve(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	ts := testutil.NewFakeTagStore()
	start := time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
	stop := start.Add(2 * time.Hour)
	if _, err := ss.Create(ctx, domain.WorkSession{ID: "s1", OwnerID: "u1", Start: start, Stop: &stop}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	uc := usecase.EditSession{Sessions: ss, Tags: ts}
	newStop := start.Add(3 * time.Hour)

	// Seed tags onto s1 via a first edit.
	deepTags := []string{"deep"}
	if _, err := uc.Execute(ctx, "u1", "s1", usecase.EditSessionInput{
		Tags: &deepTags, Note: "n", Start: start, Stop: &newStop,
	}); err != nil {
		t.Fatalf("first edit: %v", err)
	}

	// Second edit with Tags: nil — must NOT wipe the existing "deep" tag.
	got, err := uc.Execute(ctx, "u1", "s1", usecase.EditSessionInput{
		Tags: nil, Note: "updated", Start: start, Stop: &newStop,
	})
	if err != nil {
		t.Fatalf("nil-tags edit: %v", err)
	}
	stored, err := ts.TagsFor(ctx, "u1", domain.TaggableWorkSession, "s1")
	if err != nil {
		t.Fatalf("TagsFor: %v", err)
	}
	if len(stored) != 1 || stored[0].Slug != "deep" {
		t.Fatalf("nil Tags wiped taggings: stored=%v result=%v", stored, got.Tags)
	}

	// Explicit &[]string{} must clear the tags.
	empty := []string{}
	if _, err := uc.Execute(ctx, "u1", "s1", usecase.EditSessionInput{
		Tags: &empty, Note: "updated", Start: start, Stop: &newStop,
	}); err != nil {
		t.Fatalf("clear edit: %v", err)
	}
	stored, err = ts.TagsFor(ctx, "u1", domain.TaggableWorkSession, "s1")
	if err != nil {
		t.Fatalf("TagsFor after clear: %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("explicit empty Tags did not wipe: stored=%v", stored)
	}
}
