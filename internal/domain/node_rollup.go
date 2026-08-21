package domain

import "time"

// NodeRollup is a node's worktime summed over its whole subtree.
// Wire shape is httpserver.nodeRollupDTO (minutes); these fields are not serialized directly.
type NodeRollup struct {
	Total time.Duration
	Week  time.Duration
	Month time.Duration
	// PrevWeek is the ISO week before weekStart; for the overview delta.
	PrevWeek time.Duration
	// Year is the current calendar year's total; PrevYearToDate is the SAME
	// span one year earlier (Jan 1 .. today-minus-one-year), so the Screen-02
	// year tile can show an honest ±% instead of comparing eleven months
	// against twelve.
	Year           time.Duration
	PrevYearToDate time.Duration
	// Work* is the subset of Total/Week/Month that counts toward the Soll
	// (effective CountsTowardTarget flag = Work). Privat is derived by
	// callers as Total-WorkTotal / Week-WorkWeek / Month-WorkMonth.
	WorkTotal time.Duration
	WorkWeek  time.Duration
	WorkMonth time.Duration
}
