package webui

import (
	"context"
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// TagebuchMonthSummary is one row in Screen 14's month list.
type TagebuchMonthSummary struct {
	Label, MetaLabel, Href string
	Selected               bool
}

// TagebuchArchiveEntry is one note in the selected month's list.
type TagebuchArchiveEntry struct {
	ID, Href, Weekday, DateLabel, Excerpt string
}

// TagebuchArchivVM is Screen 14 ("Archiv alter Tagesnotizen").
type TagebuchArchivVM struct {
	Year               int
	PrevYear, NextYear int
	TotalCountLabel    string
	Months             []TagebuchMonthSummary
	SelectedMonthLabel string
	SelectedCountLabel string
	Entries            []TagebuchArchiveEntry
}

// BuildTagebuchArchivVM groups every DocDaily document by calendar year and
// month. year/month select the view; year==0 defaults to the newest year
// present (or now's year if there are no notes yet), month==0 to the newest
// month with entries in that year.
func BuildTagebuchArchivVM(ctx context.Context, dailyDocs []domain.Document, year, month int, now time.Time) TagebuchArchivVM {
	type ym struct{ y, m int }
	byMonth := map[ym][]domain.Document{}
	years := map[int]bool{}
	for _, d := range dailyDocs {
		dt := docDate(d)
		y, m, _ := dt.Date()
		key := ym{y, int(m)}
		byMonth[key] = append(byMonth[key], d)
		years[y] = true
	}

	if year == 0 {
		year = newestYear(years, now.Year())
	}

	vm := TagebuchArchivVM{Year: year, PrevYear: year - 1, NextYear: year + 1}
	vm.TotalCountLabel = components.Tn(ctx, "tagebuch.notesCount", len(dailyDocs))

	var monthsPresent []int
	for k := range byMonth {
		if k.y == year {
			monthsPresent = append(monthsPresent, k.m)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(monthsPresent)))

	if month == 0 && len(monthsPresent) > 0 {
		month = monthsPresent[0]
	}

	vm.Months = make([]TagebuchMonthSummary, len(monthsPresent))
	for i, m := range monthsPresent {
		docs := byMonth[ym{year, m}]
		vm.Months[i] = TagebuchMonthSummary{
			Label:     monthText(ctx, time.Month(m)),
			MetaLabel: components.Tn(ctx, "tagebuch.notesCount", len(docs)),
			Href:      tagebuchArchivHref(year, m),
			Selected:  m == month,
		}
	}

	docs := byMonth[ym{year, month}]
	sort.Slice(docs, func(i, j int) bool { return docDate(docs[i]).After(docDate(docs[j])) })
	vm.SelectedMonthLabel = monthText(ctx, time.Month(month))
	vm.SelectedCountLabel = components.Tn(ctx, "tagebuch.notesCount", len(docs))
	vm.Entries = make([]TagebuchArchiveEntry, len(docs))
	for i, d := range docs {
		vm.Entries[i] = TagebuchArchiveEntry{
			ID: d.ID, Href: "/tagebuch?selected=" + d.ID,
			Weekday:   components.T(ctx, weekdayKeys[docDate(d).Weekday()]),
			DateLabel: docDate(d).Format("02.01.2006"), Excerpt: dailyExcerpt(d.Body),
		}
	}
	return vm
}

func tagebuchArchivHref(year, month int) string {
	return "/tagebuch/archiv?year=" + itoa(year) + "&month=" + itoa(month)
}

// newestYear returns the highest year present, or fallback when years is empty.
func newestYear(years map[int]bool, fallback int) int {
	best := 0
	for y := range years {
		if y > best {
			best = y
		}
	}
	if best == 0 {
		return fallback
	}
	return best
}
