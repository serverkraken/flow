package domain

import "time"

// NodeRollup is a node's worktime summed over its whole subtree.
// Wire shape is httpserver.nodeRollupDTO (minutes); these fields are not serialized directly.
type NodeRollup struct {
	Total time.Duration
	Week  time.Duration
	Month time.Duration
}
