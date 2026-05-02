// Package dayoffstsv implements ports.DayOffStore as a TSV file at the
// configured primary path (typically ~/.tmux/worktime-dayoffs.tsv).
//
// File format:
//
//	# comments are allowed
//	2026-04-30	holiday	Karfreitag
//	2026-05-01	vacation	Urlaub Mai	4.5
//
// Columns: date, kind, label, optional target hours. Legacy 2- or
// 3-column rows without a kind keyword are read as KindHoliday so older
// files keep working.
//
// A second read-only legacy path may be passed to New (typically
// ~/.tmux/worktime-holidays.tsv). It is consulted only when the primary
// file does not exist — bridging users who still have the pre-rename
// filename. Writes always target the primary path; the legacy file is
// never modified.
//
// The per-Store cache is invalidated by Add/Remove. List and Lookup go
// through the cache; concurrent reads are safe.
package dayoffstsv
