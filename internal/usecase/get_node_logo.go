package usecase

import (
	"bytes"
	"context"
	"image"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// GetNodeLogo returns a node's stored logo blob (the WebUI serving path).
type GetNodeLogo struct {
	Logos ports.NodeLogoStore
}

func (uc GetNodeLogo) Execute(ctx context.Context, ownerID, nodeID string) (domain.NodeLogo, error) {
	l, err := uc.Logos.Get(ctx, ownerID, nodeID)
	if err != nil {
		return domain.NodeLogo{}, err
	}
	// Altbestand (vor Migration 0027 hochgeladen): Maße beim ersten Zugriff
	// nachmessen und persistieren, damit LogoShape entscheiden kann.
	if l.Width == 0 || l.Height == 0 {
		if cfg, _, derr := image.DecodeConfig(bytes.NewReader(l.Bytes)); derr == nil {
			l.Width, l.Height = cfg.Width, cfg.Height
			_ = uc.Logos.Put(ctx, l) // best-effort; serving darf nie daran scheitern
		}
	}
	return l, nil
}
