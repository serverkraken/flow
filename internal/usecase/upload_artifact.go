package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"

	_ "golang.org/x/image/webp"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// MaxArtifactBytesPerOwner caps an owner's total artifact storage (soft cap —
// see the Execute doc comment for the accepted check-then-act race).
const MaxArtifactBytesPerOwner int64 = 256 << 20

var (
	// ErrArtifactTooLarge rejects a single upload beyond domain.MaxArtifactBytes.
	ErrArtifactTooLarge = errors.New("artifact exceeds 8 MiB")
	// ErrArtifactBadType rejects content that sniffs/declares outside the
	// artifact MIME allowlist (SVG included — see domain.ArtifactDownloadMimes).
	ErrArtifactBadType = errors.New("artifact type not allowed")
	// ErrArtifactQuotaExceeded rejects an upload that would push the owner's
	// total artifact bytes over MaxArtifactBytesPerOwner.
	ErrArtifactQuotaExceeded = errors.New("owner artifact quota exceeded")
)

// ValidateArtifactBytes size-checks, content-sniffs and (for images) measures
// artifact bytes — the artifact-flavored sibling of ValidateNodeLogo. The
// sniffed MIME is authoritative for images: a PNG declared as "application/pdf"
// is still stored as image/png, and image.DecodeConfig must succeed (an image
// mime that fails to decode is rejected). An upload DECLARED as an image whose
// bytes don't actually sniff as one is rejected outright — it never falls
// back to being treated as a download. For non-images the declared MIME wins
// when it's in domain.ArtifactDownloadMimes, else the sniffed MIME is used as
// a fallback; if neither matches the allowlist (e.g. SVG, which is
// deliberately excluded) the upload is rejected.
func ValidateArtifactBytes(data []byte, declaredMime string) (mime string, w, h int, err error) {
	if int64(len(data)) > domain.MaxArtifactBytes {
		return "", 0, 0, ErrArtifactTooLarge
	}
	sniffed := http.DetectContentType(data)
	if isArtifactImageMime(sniffed) {
		cfg, _, derr := image.DecodeConfig(bytes.NewReader(data))
		if derr != nil {
			return "", 0, 0, ErrArtifactBadType
		}
		return sniffed, cfg.Width, cfg.Height, nil
	}
	if isArtifactImageMime(declaredMime) {
		return "", 0, 0, ErrArtifactBadType
	}
	if isArtifactDownloadMime(declaredMime) {
		return declaredMime, 0, 0, nil
	}
	if isArtifactDownloadMime(sniffed) {
		return sniffed, 0, 0, nil
	}
	return "", 0, 0, ErrArtifactBadType
}

func isArtifactImageMime(mime string) bool {
	for _, m := range domain.ArtifactImageMimes {
		if m == mime {
			return true
		}
	}
	return false
}

func isArtifactDownloadMime(mime string) bool {
	for _, m := range domain.ArtifactDownloadMimes {
		if m == mime {
			return true
		}
	}
	return false
}

// nextArtifactSlug returns base unchanged if it's free, otherwise the next
// free "base-1", "base-2", … suffix against the existing slug set.
func nextArtifactSlug(base string, existing []string) string {
	taken := make(map[string]bool, len(existing))
	for _, s := range existing {
		taken[s] = true
	}
	if !taken[base] {
		return base
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !taken[candidate] {
			return candidate
		}
	}
}

// UploadArtifact stores a node-scoped artifact (image or download), enforcing
// the per-file size ceiling, the owner-wide storage quota, and slug
// collision/replace semantics. It emits the resulting artifact.created or
// artifact.updated event itself (rather than leaving that to the caller)
// because there are two entry points — REST and the web gallery (Task 5) —
// and emitting once here keeps them from duplicating (or forgetting) the
// SSE publish.
type UploadArtifact struct {
	Nodes     ports.NodeStore
	Artifacts ports.ArtifactStore
	IDs       ports.IDGen
	Clock     ports.Clock
	Emitter   ports.Emitter
}

// Execute validates and stores data as an artifact on nodeID.
//
// replaceSlug == "" means a new upload: the slug is derived from name via
// domain.ArtifactSlug, with a "-1"/"-2" suffix on collision, and the event is
// artifact.created. A non-empty replaceSlug overwrites that existing slug in
// place (new content ref, name updated) and emits artifact.updated.
//
// Quota race (accepted soft-cap, agy-Fund #5): the TotalBytes read and the
// subsequent Put are not atomic, so concurrent uploads from the same owner
// can overshoot MaxArtifactBytesPerOwner by up to (MaxArtifactBytes ×
// concurrent uploads) — bounded per upload (≤ 8 MiB) and per tenant (this
// owner only). For a 256 MiB soft cap that bounded overshoot is acceptable;
// a transactional check (SELECT … FOR UPDATE / advisory lock) is deferred
// until it's actually needed. No concurrency test is part of this slice.
func (uc UploadArtifact) Execute(ctx context.Context, ownerID, nodeID, name, declaredMime string, data []byte, replaceSlug, actorKind, actorRef string) (domain.Artifact, error) {
	if nodeID != "" {
		if _, err := uc.Nodes.Get(ctx, ownerID, nodeID); err != nil {
			return domain.Artifact{}, err
		}
	}
	mime, w, h, err := ValidateArtifactBytes(data, declaredMime)
	if err != nil {
		return domain.Artifact{}, err
	}
	size := int64(len(data))
	total, err := uc.Artifacts.TotalBytes(ctx, ownerID)
	if err != nil {
		return domain.Artifact{}, err
	}
	if total+size > MaxArtifactBytesPerOwner {
		return domain.Artifact{}, ErrArtifactQuotaExceeded
	}

	slug := replaceSlug
	eventType := domain.EventArtifactUpdated
	if slug == "" {
		existing, err := uc.Artifacts.ExistingSlugs(ctx, ownerID, nodeID)
		if err != nil {
			return domain.Artifact{}, err
		}
		slug = nextArtifactSlug(domain.ArtifactSlug(name), existing)
		eventType = domain.EventArtifactCreated
	}

	sum := sha256.Sum256(data)
	ref := hex.EncodeToString(sum[:])[:12]
	now := uc.Clock.Now()
	a := domain.Artifact{
		ID:            uc.IDs.NewID(),
		OwnerID:       ownerID,
		NodeID:        nodeID,
		Slug:          slug,
		Name:          name,
		Mime:          mime,
		SizeBytes:     size,
		Ref:           ref,
		Bytes:         data,
		Width:         w,
		Height:        h,
		CreatedByKind: actorKind,
		CreatedByRef:  actorRef,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := uc.Artifacts.Put(ctx, a); err != nil {
		return domain.Artifact{}, err
	}
	uc.Emitter.Emit(ctx, domain.Event{Type: eventType, UserID: ownerID, Data: map[string]any{"id": slug, "name": a.Name, "node": nodeID}})
	a.Bytes = nil
	return a, nil
}
