package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"

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

// ValidateNodeLogo size-checks and content-sniffs logo bytes. Handlers call it
// BEFORE creating a node so a bad file rejects the whole form; UploadNodeLogo
// calls it again as its own invariant.
func ValidateNodeLogo(data []byte) (string, error) {
	if len(data) > MaxNodeLogoBytes {
		return "", ErrLogoTooLarge
	}
	mime := http.DetectContentType(data)
	switch mime {
	case "image/png", "image/jpeg", "image/webp":
		return mime, nil
	default:
		return "", ErrLogoBadType
	}
}

// UploadNodeLogo stores a node's logo image (replace-on-upload) and stamps the
// node's LogoRef with the content hash for cache-busting URLs and ETags.
type UploadNodeLogo struct {
	Nodes ports.NodeStore
	Logos ports.NodeLogoStore
	Clock ports.Clock
}

func (uc UploadNodeLogo) Execute(ctx context.Context, ownerID, nodeID string, data []byte) (domain.Node, error) {
	mime, err := ValidateNodeLogo(data)
	if err != nil {
		return domain.Node{}, err
	}
	n, err := uc.Nodes.Get(ctx, ownerID, nodeID)
	if err != nil {
		return domain.Node{}, err
	}
	sum := sha256.Sum256(data)
	ref := hex.EncodeToString(sum[:])[:12]
	now := uc.Clock.Now()
	if err := uc.Logos.Put(ctx, domain.NodeLogo{
		NodeID: nodeID, OwnerID: ownerID, Mime: mime, Ref: ref, Bytes: data, UpdatedAt: now,
	}); err != nil {
		return domain.Node{}, err
	}
	n.LogoRef = ref
	n.UpdatedAt = now
	return uc.Nodes.Update(ctx, ownerID, n)
}
