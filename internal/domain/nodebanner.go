package domain

import "time"

// NodeBanner is a node's uploaded banner image (at most one per node,
// replace-on-upload; stored as a DB blob, mirrors NodeLogo). Mime is sniffed
// server-side (image/png, image/jpeg or image/webp). Ref is the first 12 hex
// chars of the content's SHA-256 — mirrored onto Node.BannerRef for
// cache-busting URLs and used as the serving ETag. Unlike the logo, the
// banner has no shape decision (always a fixed-aspect object-cover strip),
// so — unlike NodeLogo — it carries no measured width/height.
type NodeBanner struct {
	NodeID    string
	OwnerID   string
	Mime      string
	Ref       string
	Bytes     []byte
	UpdatedAt time.Time
}
