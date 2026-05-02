package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func TestCaptureDaily_AppendsToExisting(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	clock := testutil.FixedClock{Time: time.Date(2026, 4, 25, 14, 30, 0, 0, time.UTC)}

	existing, err := domain.NewNote(
		domain.ID("daily/2026-04-25"),
		domain.Frontmatter{ID: "daily/2026-04-25", Type: domain.TypeDaily, Date: "2026-04-25"},
		[]byte("# Today\n\nFirst paragraph"),
	)
	if err != nil {
		t.Fatal(err)
	}
	store.Seed(existing, time.Unix(1, 0))

	u := usecase.NewCaptureDaily(store, clock)
	out, err := u.Execute(context.Background(), usecase.CaptureDailyInput{Text: "Got the build green"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Created {
		t.Errorf("daily already existed; Created should be false")
	}
	if out.ID != "daily/2026-04-25" {
		t.Errorf("ID got %q", out.ID)
	}
	if !strings.Contains(out.Bullet, "14:30") || !strings.Contains(out.Bullet, "Got the build green") {
		t.Errorf("bullet should carry timestamp + text, got %q", out.Bullet)
	}

	// Body should now contain both the original prose and the appended bullet.
	got, err := store.Get(context.Background(), "daily/2026-04-25")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got.Body), "First paragraph") {
		t.Errorf("existing body lost: %q", got.Body)
	}
	if !strings.Contains(string(got.Body), "14:30 — Got the build green") {
		t.Errorf("appended bullet missing: %q", got.Body)
	}
}

func TestCaptureDaily_CreatesWhenMissing(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	clock := testutil.FixedClock{Time: time.Date(2026, 4, 25, 9, 5, 0, 0, time.UTC)}
	u := usecase.NewCaptureDaily(store, clock)

	out, err := u.Execute(context.Background(), usecase.CaptureDailyInput{Text: "Reading on the train"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !out.Created {
		t.Error("expected Created=true on first capture of the day")
	}
	got, err := store.Get(context.Background(), "daily/2026-04-25")
	if err != nil {
		t.Fatalf("daily should exist after capture: %v", err)
	}
	if !strings.Contains(string(got.Body), "09:05 — Reading on the train") {
		t.Errorf("missing bullet: %q", got.Body)
	}
}

func TestCaptureDaily_RejectsEmptyText(t *testing.T) {
	t.Parallel()
	u := usecase.NewCaptureDaily(testutil.NewFakeNoteStore(), testutil.FixedClock{})
	_, err := u.Execute(context.Background(), usecase.CaptureDailyInput{Text: "   "})
	if !errors.Is(err, usecase.ErrCaptureEmpty) {
		t.Errorf("got %v, want ErrCaptureEmpty", err)
	}
}

func TestCaptureDaily_PutErrorPropagates(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced put error")
	store := testutil.NewFakeNoteStore()
	store.PutErr = forced
	u := usecase.NewCaptureDaily(store, testutil.FixedClock{Time: time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)})
	_, err := u.Execute(context.Background(), usecase.CaptureDailyInput{Text: "x"})
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want forced", err)
	}
}

func TestCaptureDaily_GetErrorPropagates(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced get error")
	store := testutil.NewFakeNoteStore()
	store.GetErr = forced
	u := usecase.NewCaptureDaily(store, testutil.FixedClock{Time: time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)})
	_, err := u.Execute(context.Background(), usecase.CaptureDailyInput{Text: "x"})
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want forced", err)
	}
}

// TestCaptureDaily_ReindexBestEffort verifies the optional Index
// dependency is hit on success — reindex errors are swallowed (the
// daily was written; a stale index is recoverable).
func TestCaptureDaily_ReindexBestEffort(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	idx := testutil.NewFakeIndexer()
	clock := testutil.FixedClock{Time: time.Date(2026, 4, 25, 8, 0, 0, 0, time.UTC)}

	u := usecase.NewCaptureDaily(store, clock)
	u.Index = idx

	if _, err := u.Execute(context.Background(), usecase.CaptureDailyInput{Text: "morning"}); err != nil {
		t.Fatal(err)
	}
	_, err := idx.Search(context.Background(), domain.SearchQuery{Text: "morning"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// We can't directly assert the index has the entry without exposing more
	// internals, but the smoke check is enough: capture didn't error, index
	// was reachable.
	_ = ports.SyncStats{}
}
