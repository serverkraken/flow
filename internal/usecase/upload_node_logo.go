package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"time"

	_ "golang.org/x/image/webp"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// MaxNodeLogoBytes caps uploaded node logos (DB-blob storage keeps rows small).
const MaxNodeLogoBytes = 512 * 1024

var (
	// ErrLogoTooLarge rejects uploads beyond MaxNodeLogoBytes.
	ErrLogoTooLarge = errors.New("logo exceeds 512 KiB")
	// ErrLogoBadType rejects anything that does not sniff as PNG/JPEG/WebP.
	ErrLogoBadType = errors.New("logo must be PNG, JPEG or WebP")
)

// ValidateNodeLogo size-checks, content-sniffs and measures logo bytes.
// Handlers call it BEFORE creating a node so a bad file rejects the whole
// form; UploadNodeLogo calls it again as its own invariant. Width/height come
// from image.DecodeConfig (jpeg/png stdlib, webp via golang.org/x/image); a
// sniff-valid but unparseable image is rejected.
func ValidateNodeLogo(data []byte) (mime string, w, h int, err error) {
	if len(data) > MaxNodeLogoBytes {
		return "", 0, 0, ErrLogoTooLarge
	}
	mime = http.DetectContentType(data)
	switch mime {
	case "image/png", "image/jpeg", "image/webp":
	default:
		return "", 0, 0, ErrLogoBadType
	}
	cfg, _, derr := image.DecodeConfig(bytes.NewReader(data))
	if derr != nil {
		return "", 0, 0, ErrLogoBadType
	}
	return mime, cfg.Width, cfg.Height, nil
}

func buildNodeLogo(ownerID, nodeID string, data []byte, now time.Time) (domain.NodeLogo, error) {
	mime, w, h, err := ValidateNodeLogo(data)
	if err != nil {
		return domain.NodeLogo{}, err
	}
	sum := sha256.Sum256(data)
	return domain.NodeLogo{
		NodeID: nodeID, OwnerID: ownerID, Mime: mime,
		Ref: hex.EncodeToString(sum[:])[:12], Bytes: data, UpdatedAt: now,
		Width: w, Height: h,
	}, nil
}

// UploadNodeLogo stores a node's logo image (replace-on-upload) and stamps the
// node's LogoRef with the content hash for cache-busting URLs and ETags.
type UploadNodeLogo struct {
	Nodes     ports.NodeStore
	Logos     ports.NodeLogoStore
	Aggregate ports.NodeAggregateStore
	Clock     ports.Clock
}

func (uc UploadNodeLogo) Execute(ctx context.Context, ownerID, nodeID string, data []byte) (domain.Node, error) {
	logo, err := buildNodeLogo(ownerID, nodeID, data, uc.Clock.Now())
	if err != nil {
		return domain.Node{}, err
	}
	ref := logo.Ref
	now := logo.UpdatedAt
	if uc.Aggregate != nil {
		return uc.Aggregate.UpdateAggregate(ctx, ownerID, nodeID, func(n domain.Node) (domain.Node, ports.NodeAggregateChanges, error) {
			n.LogoRef = ref
			n.UpdatedAt = now
			return n, ports.NodeAggregateChanges{Logo: ports.NodeLogoPut, LogoValue: logo}, nil
		})
	}
	n, err := uc.Nodes.Get(ctx, ownerID, nodeID)
	if err != nil {
		return domain.Node{}, err
	}
	if err := uc.Logos.Put(ctx, logo); err != nil {
		return domain.Node{}, err
	}
	n.LogoRef = ref
	n.UpdatedAt = now
	return uc.Nodes.Update(ctx, ownerID, n)
}
