package domain

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// MaxArtifactBytes is the per-file size ceiling for a single artifact upload.
const MaxArtifactBytes int64 = 8 << 20

// Artifact is a node-scoped binary asset (image or download) attached to
// exactly one node. Reach is the node's ancestor chain, not its subtree —
// a document resolves ![[slug]] against its own node's ancestors, nearest
// first. Bytes is excluded from JSON (REST meta responses never inline the
// payload; Get serves it as a raw byte stream).
type Artifact struct {
	ID      string `json:"id"`
	OwnerID string `json:"-"`
	NodeID  string `json:"nodeId"`
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Mime    string `json:"mime"`

	SizeBytes int64  `json:"sizeBytes"`
	Ref       string `json:"ref"`
	Bytes     []byte `json:"-"`

	// Width, Height are pixel dimensions for images (image.DecodeConfig at
	// upload time); zero for non-image artifacts.
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`

	// CreatedByKind/CreatedByRef stamp the uploading actor (actor.Kind as
	// string + actor.Actor.Ref), mirroring Document's UpdatedByKind/Ref.
	CreatedByKind string `json:"createdByKind,omitempty"`
	CreatedByRef  string `json:"createdByRef,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ArtifactImageMimes are inline-renderable image MIME types.
var ArtifactImageMimes = []string{"image/png", "image/jpeg", "image/webp", "image/gif"}

// ArtifactDownloadMimes are accepted non-image (attachment) MIME types.
// image/svg+xml is deliberately excluded — inline SVG can carry scripts.
var ArtifactDownloadMimes = []string{
	"application/pdf", "text/csv", "text/plain", "application/json",
	"application/zip", "application/octet-stream",
}

// ArtifactMimeAllowlist is the set of all MIME types accepted for artifact
// upload — the union of ArtifactImageMimes and ArtifactDownloadMimes.
var ArtifactMimeAllowlist = newArtifactMimeAllowlist()

func newArtifactMimeAllowlist() map[string]bool {
	m := make(map[string]bool, len(ArtifactImageMimes)+len(ArtifactDownloadMimes))
	for _, mime := range ArtifactImageMimes {
		m[mime] = true
	}
	for _, mime := range ArtifactDownloadMimes {
		m[mime] = true
	}
	return m
}

// artifactSlugRe matches a flat, single-segment slug token: lowercase
// alphanumerics joined by single hyphens, no '/' — deliberately stricter than
// domain.SlugOK (documents' hierarchical slug), since an artifact slug is
// node-local, not path-like.
var artifactSlugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// ArtifactSlugOK reports whether s is a valid flat artifact slug.
func ArtifactSlugOK(s string) bool {
	return s != "" && artifactSlugRe.MatchString(s)
}

// artifactDeUmlauts and artifactNonSlug duplicate usecase.Slugify's
// transliteration/collapsing rules here in domain — domain must not import
// usecase (hexagonal dependency direction points inward).
var (
	artifactDeUmlauts = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss")
	artifactNonSlug   = regexp.MustCompile(`[^a-z0-9]+`)
)

// ArtifactSlug derives a flat slug from a file's display name: the extension
// is stripped first (so "Mein Bild.PNG" → "mein-bild"), then the remainder is
// lowercased, transliterated, and collapsed to hyphens. Callers append a
// "-1"/"-2" suffix on collision (see ArtifactStore.ExistingSlugs).
func ArtifactSlug(name string) string {
	if ext := filepath.Ext(name); ext != "" {
		name = strings.TrimSuffix(name, ext)
	}
	s := artifactDeUmlauts.Replace(strings.ToLower(name))
	s = artifactNonSlug.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// NextArtifactSlug returns base unchanged when it is free, otherwise the next
// free base-1, base-2, ... suffix. Callers must serialize the read and write
// when uniqueness matters; this helper only performs the deterministic choice.
func NextArtifactSlug(base string, existing []string) string {
	taken := make(map[string]bool, len(existing))
	for _, slug := range existing {
		taken[slug] = true
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

// IsImage reports whether the artifact's MIME type is inline-renderable.
func (a Artifact) IsImage() bool {
	return strings.HasPrefix(a.Mime, "image/")
}

// Validate checks the artifact invariants: slug shape, MIME allowlist
// membership, size ceiling, and a non-empty display name. Pure — no I/O, no
// ownership/existence checks (those are the store's job).
func (a Artifact) Validate() error {
	if !ArtifactSlugOK(a.Slug) {
		return fmt.Errorf("%w: bad slug %q", ErrInvalidArtifact, a.Slug)
	}
	if !ArtifactMimeAllowlist[a.Mime] {
		return fmt.Errorf("%w: unsupported mime %q", ErrInvalidArtifact, a.Mime)
	}
	if a.SizeBytes > MaxArtifactBytes {
		return fmt.Errorf("%w: size %d exceeds max %d bytes", ErrInvalidArtifact, a.SizeBytes, MaxArtifactBytes)
	}
	if a.Name == "" {
		return fmt.Errorf("%w: name required", ErrInvalidArtifact)
	}
	return nil
}
