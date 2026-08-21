package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// AssignHighlight records a marked passage of a document assigned to one
// register (Screen 27, "Stellen markieren und je Register zuordnen").
type AssignHighlight struct {
	Highlights ports.NodeHighlightStore
	IDs        ports.IDGen
	Clock      ports.Clock
}

func (uc AssignHighlight) Execute(ctx context.Context, ownerID, documentID, nodeID, quote string) (domain.NodeHighlight, error) {
	quote = strings.TrimSpace(quote)
	if quote == "" {
		return domain.NodeHighlight{}, fmt.Errorf("%w: empty quote", domain.ErrInvalidHighlight)
	}
	h := domain.NodeHighlight{
		ID: uc.IDs.NewID(), OwnerID: ownerID, DocumentID: documentID, NodeID: nodeID,
		Quote: quote, CreatedAt: uc.Clock.Now(),
	}
	return uc.Highlights.Create(ctx, h)
}

// RemoveHighlight deletes a highlight (undoes an assignment).
type RemoveHighlight struct{ Highlights ports.NodeHighlightStore }

func (uc RemoveHighlight) Execute(ctx context.Context, ownerID, id string) error {
	return uc.Highlights.Delete(ctx, ownerID, id)
}

// ListDocumentHighlights lists one document's highlights in reading order —
// the source for both the inline marks and the "Zuordnungen in dieser Notiz" list.
type ListDocumentHighlights struct{ Highlights ports.NodeHighlightStore }

func (uc ListDocumentHighlights) Execute(ctx context.Context, ownerID, documentID string) ([]domain.NodeHighlight, error) {
	return uc.Highlights.ListForDocument(ctx, ownerID, documentID)
}

// ListRecentHighlights lists highlights created at or after since, newest
// first — the source for the "Diesen Monat markiert" per-register tally.
type ListRecentHighlights struct{ Highlights ports.NodeHighlightStore }

func (uc ListRecentHighlights) Execute(ctx context.Context, ownerID string, since time.Time) ([]domain.NodeHighlight, error) {
	return uc.Highlights.ListSince(ctx, ownerID, since)
}

// ListNewestHighlights lists the owner's newest highlights capped by limit —
// the source for the register entry point's "Woran zuletzt gearbeitet" block.
// limit <= 0 is normalised to 1 so a miswired caller cannot pull the whole
// table.
type ListNewestHighlights struct{ Highlights ports.NodeHighlightStore }

func (uc ListNewestHighlights) Execute(ctx context.Context, ownerID string, limit int) ([]domain.NodeHighlight, error) {
	if limit <= 0 {
		limit = 1
	}
	return uc.Highlights.ListRecent(ctx, ownerID, limit)
}

// ForNodes lists the newest highlights of a node set, newest first, capped by
// limit — the register entry point's "Woran zuletzt gearbeitet". limit <= 0 is
// normalised to 1 so a miswired caller cannot pull the whole table.
func (uc ListNewestHighlights) ForNodes(ctx context.Context, ownerID string, nodeIDs []string, limit int) ([]domain.NodeHighlight, error) {
	if limit <= 0 {
		limit = 1
	}
	return uc.Highlights.ListRecentForNodes(ctx, ownerID, nodeIDs, limit)
}
