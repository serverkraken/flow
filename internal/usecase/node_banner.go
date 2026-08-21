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

// MaxNodeBannerBytes caps uploaded node banners (DB-blob storage keeps rows
// small). Larger than MaxNodeLogoBytes — banners are wide strips
// (1600×400 recommended, per the Karteikasten design), not small square marks.
const MaxNodeBannerBytes = 1024 * 1024

var (
	// ErrBannerTooLarge rejects uploads beyond MaxNodeBannerBytes.
	ErrBannerTooLarge = errors.New("banner exceeds 1 MiB")
	// ErrBannerBadType rejects anything that does not sniff as PNG/JPEG/WebP.
	ErrBannerBadType = errors.New("banner must be PNG, JPEG or WebP")
)

// ValidateNodeBanner size-checks and content-sniffs banner bytes. Mirrors
// ValidateNodeLogo; unlike the logo the banner has no shape decision, so the
// decoded dimensions are discarded — DecodeConfig still has to succeed, or a
// file that merely starts like a PNG would pass on the sniff alone.
func ValidateNodeBanner(data []byte) (mime string, err error) {
	if len(data) > MaxNodeBannerBytes {
		return "", ErrBannerTooLarge
	}
	mime = http.DetectContentType(data)
	switch mime {
	case "image/png", "image/jpeg", "image/webp":
	default:
		return "", ErrBannerBadType
	}
	if _, _, derr := image.DecodeConfig(bytes.NewReader(data)); derr != nil {
		return "", ErrBannerBadType
	}
	return mime, nil
}

func buildNodeBanner(ownerID, nodeID string, data []byte, now time.Time) (domain.NodeBanner, error) {
	mime, err := ValidateNodeBanner(data)
	if err != nil {
		return domain.NodeBanner{}, err
	}
	sum := sha256.Sum256(data)
	return domain.NodeBanner{
		NodeID: nodeID, OwnerID: ownerID, Mime: mime,
		Ref: hex.EncodeToString(sum[:])[:12], Bytes: data, UpdatedAt: now,
	}, nil
}

// GetNodeBanner returns a node's stored banner blob (the WebUI serving path).
type GetNodeBanner struct {
	Banners ports.NodeBannerStore
}

func (uc GetNodeBanner) Execute(ctx context.Context, ownerID, nodeID string) (domain.NodeBanner, error) {
	return uc.Banners.Get(ctx, ownerID, nodeID)
}
