package domain

import "time"

// NodeLogo is a node's uploaded logo image (at most one per node,
// replace-on-upload; stored as a DB blob). Mime is sniffed server-side
// (image/png, image/jpeg or image/webp). Ref is the first 12 hex chars of
// the content's SHA-256 — mirrored onto Node.LogoRef for cache-busting URLs
// and used as the serving ETag.
type NodeLogo struct {
	NodeID    string
	OwnerID   string
	Mime      string
	Ref       string
	Bytes     []byte
	UpdatedAt time.Time
	// Width, Height are the pixel dimensions, gemessen beim Upload via
	// image.DecodeConfig; 0 = Altbestand, wird beim ersten Get lazy vermessen.
	Width, Height int
}
