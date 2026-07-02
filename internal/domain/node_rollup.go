package domain

import "time"

// NodeRollup is a node's worktime summed over its whole subtree.
// Wire shape is httpserver.nodeRollupDTO (minutes); these fields are not serialized directly.
type NodeRollup struct {
	Total time.Duration
	Week  time.Duration
	Month time.Duration
	// Work* is the subset of Total/Week/Month that counts toward the Soll
	// (effective CountsTowardTarget flag = Work). Privat is derived by
	// callers as Total-WorkTotal / Week-WorkWeek / Month-WorkMonth.
	WorkTotal time.Duration
	WorkWeek  time.Duration
	WorkMonth time.Duration
}
