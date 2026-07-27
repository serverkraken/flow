package usecase

import (
	"context"
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

// NodeMRU ranks the owner's nodes by most-recently-booked (Spec §1 MRU support).
// Pure composition over SessionStore.LastBookedByNode — no new store method here.
type NodeMRU struct {
	Sessions ports.SessionStore
}

// NodeMRUEntry is one node's last booking time.
type NodeMRUEntry struct {
	NodeID       string
	LastBookedAt time.Time
}

func (uc NodeMRU) Execute(ctx context.Context, ownerID string) ([]NodeMRUEntry, error) {
	m, err := uc.Sessions.LastBookedByNode(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]NodeMRUEntry, 0, len(m))
	for id, t := range m {
		out = append(out, NodeMRUEntry{NodeID: id, LastBookedAt: t})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastBookedAt.After(out[j].LastBookedAt) })
	return out, nil
}
