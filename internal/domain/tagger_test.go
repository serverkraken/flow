package domain_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func mkSess(date string, h, m int, dur time.Duration, tag string) domain.Session {
	d, _ := time.ParseInLocation("2006-01-02", date, time.Local)
	start := d.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute)
	return domain.Session{Date: d, Start: start, Stop: start.Add(dur), Elapsed: dur, Tag: tag}
}

func TestRecentTags(t *testing.T) {
	sessions := []domain.Session{
		mkSess("2026-04-25", 9, 0, time.Hour, "deep"),
		mkSess("2026-04-26", 9, 0, time.Hour, "meeting"),
		mkSess("2026-04-27", 9, 0, time.Hour, "deep"), // duplicate, more recent
		mkSess("2026-04-28", 9, 0, time.Hour, ""),     // empty tag — skipped
		mkSess("2026-04-29", 9, 0, time.Hour, "ops"),  // newest
	}

	t.Run("returns newest first, deduplicated", func(t *testing.T) {
		got := domain.RecentTags(sessions, 5)
		want := []string{"ops", "deep", "meeting"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("respects n limit", func(t *testing.T) {
		got := domain.RecentTags(sessions, 2)
		if len(got) != 2 {
			t.Errorf("expected 2 results, got %v", got)
		}
		if got[0] != "ops" || got[1] != "deep" {
			t.Errorf("expected [ops deep], got %v", got)
		}
	})

	t.Run("n<=0 returns nil", func(t *testing.T) {
		if got := domain.RecentTags(sessions, 0); got != nil {
			t.Errorf("n=0 should return nil, got %v", got)
		}
		if got := domain.RecentTags(sessions, -1); got != nil {
			t.Errorf("n<0 should return nil, got %v", got)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		got := domain.RecentTags(nil, 5)
		if len(got) != 0 {
			t.Errorf("expected empty, got %v", got)
		}
	})
}

func TestTopUsageTags(t *testing.T) {
	sessions := []domain.Session{
		mkSess("2026-04-25", 9, 0, time.Hour, "deep"),
		mkSess("2026-04-26", 9, 0, time.Hour, "deep"),
		mkSess("2026-04-27", 9, 0, time.Hour, "deep"),
		mkSess("2026-04-25", 9, 0, time.Hour, "meeting"),
		mkSess("2026-04-29", 9, 0, time.Hour, "meeting"), // tied count with X but more recent
		mkSess("2026-04-25", 9, 0, time.Hour, "ops"),
		mkSess("2026-04-26", 9, 0, time.Hour, "ops"),
		mkSess("2026-04-27", 9, 0, time.Hour, ""), // empty — skipped
	}

	t.Run("sorted by count desc", func(t *testing.T) {
		got := domain.TopUsageTags(sessions, 10)
		// deep=3, ops=2, meeting=2 — meeting more recent than ops.
		want := []string{"deep", "meeting", "ops"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("respects n limit", func(t *testing.T) {
		got := domain.TopUsageTags(sessions, 2)
		if len(got) != 2 {
			t.Errorf("expected 2 results, got %v", got)
		}
	})

	t.Run("n<=0 returns nil", func(t *testing.T) {
		if got := domain.TopUsageTags(sessions, 0); got != nil {
			t.Errorf("n=0 should return nil, got %v", got)
		}
		if got := domain.TopUsageTags(sessions, -1); got != nil {
			t.Errorf("n<0 should return nil, got %v", got)
		}
	})
}

func TestSessionTemplatesOf(t *testing.T) {
	t.Run("buckets by rounded start + dur + tag", func(t *testing.T) {
		// Two standups around 09:30 — should bucket together.
		sessions := []domain.Session{
			mkSess("2026-04-27", 9, 28, 30*time.Minute, "standup"), // rounded → 09:15, 30m
			mkSess("2026-04-28", 9, 30, 30*time.Minute, "standup"), // rounded → 09:30, 30m
			mkSess("2026-04-29", 9, 32, 30*time.Minute, "standup"), // rounded → 09:30, 30m
		}
		got := domain.SessionTemplatesOf(sessions, 5)
		// 09:30 bucket has 2 entries (28th + 29th), 09:15 has 1 → only the
		// 2-count bucket survives the count >= 2 filter.
		if len(got) != 1 {
			t.Fatalf("expected 1 template (count>=2), got %d: %+v", len(got), got)
		}
		if got[0].Count != 2 {
			t.Errorf("Count = %d, want 2", got[0].Count)
		}
		if got[0].Tag != "standup" {
			t.Errorf("Tag = %q", got[0].Tag)
		}
	})

	t.Run("ignores cross-midnight sessions", func(t *testing.T) {
		d, _ := time.ParseInLocation("2006-01-02", "2026-04-27", time.Local)
		s := domain.Session{
			Date:    d,
			Start:   d.Add(23 * time.Hour),
			Stop:    d.AddDate(0, 0, 1).Add(time.Hour),
			Elapsed: 2 * time.Hour,
			Tag:     "deep",
		}
		got := domain.SessionTemplatesOf([]domain.Session{s, s}, 5)
		if len(got) != 0 {
			t.Errorf("cross-midnight should be excluded, got %+v", got)
		}
	})

	t.Run("ignores sub-15-minute sessions", func(t *testing.T) {
		got := domain.SessionTemplatesOf([]domain.Session{
			mkSess("2026-04-27", 9, 0, 5*time.Minute, "deep"),
			mkSess("2026-04-28", 9, 0, 5*time.Minute, "deep"),
		}, 5)
		if len(got) != 0 {
			t.Errorf("sub-grid should be excluded, got %+v", got)
		}
	})

	t.Run("count<2 buckets are dropped", func(t *testing.T) {
		got := domain.SessionTemplatesOf([]domain.Session{
			mkSess("2026-04-27", 9, 0, time.Hour, "deep"),
		}, 5)
		if len(got) != 0 {
			t.Errorf("singleton should be excluded, got %+v", got)
		}
	})

	t.Run("preserves casing of the most recent occurrence", func(t *testing.T) {
		got := domain.SessionTemplatesOf([]domain.Session{
			mkSess("2026-04-27", 9, 0, time.Hour, "Deep"),
			mkSess("2026-04-28", 9, 0, time.Hour, "DEEP"),
		}, 5)
		if len(got) != 1 || got[0].Tag != "DEEP" {
			t.Errorf("expected DEEP from latest, got %+v", got)
		}
	})

	t.Run("respects n limit", func(t *testing.T) {
		// Build three distinct buckets each with count 2 by varying tag.
		ses := []domain.Session{
			mkSess("2026-04-27", 9, 0, time.Hour, "a"),
			mkSess("2026-04-28", 9, 0, time.Hour, "a"),
			mkSess("2026-04-27", 10, 0, time.Hour, "b"),
			mkSess("2026-04-28", 10, 0, time.Hour, "b"),
			mkSess("2026-04-27", 11, 0, time.Hour, "c"),
			mkSess("2026-04-28", 11, 0, time.Hour, "c"),
		}
		got := domain.SessionTemplatesOf(ses, 2)
		if len(got) != 2 {
			t.Errorf("n=2 should clamp to 2 results, got %d", len(got))
		}
	})

	t.Run("sorts by count desc when counts differ", func(t *testing.T) {
		// Bucket A occurs 3×, bucket B occurs 2× — A must come first
		// regardless of recency.
		ses := []domain.Session{
			mkSess("2026-04-27", 9, 0, time.Hour, "a"),
			mkSess("2026-04-28", 9, 0, time.Hour, "a"),
			mkSess("2026-04-29", 9, 0, time.Hour, "a"),
			// B is more recent than A's last but has lower count.
			mkSess("2026-04-30", 11, 0, time.Hour, "b"),
			mkSess("2026-05-01", 11, 0, time.Hour, "b"),
		}
		got := domain.SessionTemplatesOf(ses, 5)
		if len(got) != 2 {
			t.Fatalf("expected 2 templates, got %d", len(got))
		}
		if got[0].Tag != "a" || got[0].Count != 3 {
			t.Errorf("position 0 should be 'a' x3, got %+v", got[0])
		}
		if got[1].Tag != "b" || got[1].Count != 2 {
			t.Errorf("position 1 should be 'b' x2, got %+v", got[1])
		}
	})

	t.Run("n<=0 returns nil", func(t *testing.T) {
		if got := domain.SessionTemplatesOf(nil, 0); got != nil {
			t.Errorf("n=0 should return nil")
		}
	})
}
