package webui

import (
	"context"
	"html/template"
	"sort"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// weekdayKeys maps time.Weekday to the catalog's short weekday words. It
// reuses the Datumsstaffel's vocabulary (staffel.wd.*) instead of opening a
// second set — the staffel already had to be de-duplicated once.
var weekdayKeys = [7]string{
	"staffel.wd.sun", "staffel.wd.mon", "staffel.wd.tue", "staffel.wd.wed",
	"staffel.wd.thu", "staffel.wd.fri", "staffel.wd.sat",
}

// TagebuchNote is one row in Screen 04's middle-column list: the current
// month's daily notes, newest first.
type TagebuchNote struct {
	ID, Weekday, DateLabel, Excerpt string
	Selected                        bool
}

// TagebuchDetail is the reading pane shared by Screens 04 and 27.
type TagebuchDetail struct {
	ID, Title, Path, MetaLabel string
	BodyHTML                   template.HTML
	EditHref, MarkierenHref    string
}

// TagebuchVM is Screen 04 ("heutige Tagesnotiz"): the month's notes plus the
// selected note's reading pane.
type TagebuchVM struct {
	MonthLabel      string
	NotesCountLabel string
	StreakLabel     string
	Notes           []TagebuchNote
	Selected        *TagebuchDetail
}

// dailyExcerpt clips a note body to a short single-line preview (first
// non-empty line, hard-capped) — mirrors docrow's Preview intent but plain
// text only, since the middle-column list has no room for Markdown.
func dailyExcerpt(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		const maxLen = 90
		r := []rune(line)
		if len(r) > maxLen {
			return string(r[:maxLen]) + "…"
		}
		return line
	}
	return ""
}

// dailyStreak counts consecutive calendar days with a note, walking back
// from the most recent one (docs must be sorted newest-first, dedup'd by day).
func dailyStreak(days []time.Time) int {
	if len(days) == 0 {
		return 0
	}
	streak := 1
	for i := 1; i < len(days); i++ {
		if days[i-1].AddDate(0, 0, -1).Equal(days[i]) {
			streak++
			continue
		}
		break
	}
	return streak
}

// BuildTagebuchVM assembles Screen 04 from every DocDaily document (already
// owner-scoped, un-archived) and the viewer's local "now". selectedID picks
// the reading pane; "" defaults to the newest note.
func BuildTagebuchVM(ctx context.Context, dailyDocs []domain.Document, selectedID string, now time.Time) TagebuchVM {
	sorted := make([]domain.Document, len(dailyDocs))
	copy(sorted, dailyDocs)
	sort.Slice(sorted, func(i, j int) bool { return docDate(sorted[i]).After(docDate(sorted[j])) })

	vm := TagebuchVM{}
	vm.NotesCountLabel = components.Tn(ctx, "tagebuch.notesCount", len(sorted))
	if len(sorted) > 0 {
		vm.MonthLabel = monthText(ctx, docDate(sorted[0]).Month())
	}

	days := make([]time.Time, 0, len(sorted))
	seen := map[string]bool{}
	for _, d := range sorted {
		day := dayOnly(docDate(d))
		key := day.Format("2006-01-02")
		if !seen[key] {
			seen[key] = true
			days = append(days, day)
		}
	}
	vm.StreakLabel = components.Tn(ctx, "tagebuch.streak", dailyStreak(days))

	vm.Notes = make([]TagebuchNote, len(sorted))
	for i, d := range sorted {
		vm.Notes[i] = TagebuchNote{
			ID: d.ID, Weekday: components.T(ctx, weekdayKeys[docDate(d).Weekday()]),
			DateLabel: docDate(d).Format("02.01."), Excerpt: dailyExcerpt(d.Body),
			Selected: d.ID == selectedID,
		}
	}

	idx := 0
	found := false
	if selectedID != "" {
		for i, d := range sorted {
			if d.ID == selectedID {
				idx, found = i, true
				break
			}
		}
	}
	if !found && len(sorted) > 0 {
		idx, found = 0, true
		vm.Notes[0].Selected = true
	}
	if found {
		vm.Selected = tagebuchDetailOf(ctx, sorted[idx])
	}
	return vm
}

func tagebuchDetailOf(ctx context.Context, d domain.Document) *TagebuchDetail {
	weekday := components.T(ctx, weekdayKeys[docDate(d).Weekday()])
	return &TagebuchDetail{
		ID: d.ID, Title: weekday + ", " + docDate(d).Format("2. ") + monthText(ctx, docDate(d).Month()),
		Path: d.Path,
		// One line of provenance under the title (Screen 04): when the note was
		// last written in. The day's figures the mockup also shows there need
		// per-day statistics the reading pane does not load.
		MetaLabel:     components.T(ctx, "tagebuch.lastWritten") + " " + d.UpdatedAt.Local().Format("15:04"),
		BodyHTML:      RenderMarkdown(d.Body),
		EditHref:      "/wissen/" + d.ID + "/bearbeiten",
		MarkierenHref: "/tagebuch/" + d.ID + "/markieren",
	}
}

// docDate returns d.Date if set, else the zero time — daily documents always
// carry Date (domain.Document.Validate enforces it).
func docDate(d domain.Document) time.Time {
	if d.Date == nil {
		return time.Time{}
	}
	return *d.Date
}

func dayOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
