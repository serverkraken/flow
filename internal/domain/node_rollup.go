package domain

import "time"

// NodeRollup is a node's worktime summed over its whole subtree.
type NodeRollup struct {
	Total time.Duration `json:"total"`
	Week  time.Duration `json:"week"`
	Month time.Duration `json:"month"`
}
