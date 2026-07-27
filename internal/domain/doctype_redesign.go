package domain

import "strings"

// RedesignReport summarises a RedesignDocTypes maintenance run.
type RedesignReport struct {
	Scanned   int `json:"scanned"`   // legacy agent docs seen
	Converted int `json:"converted"` // docs rewritten (== Scanned outside dry-run)
}

// RedesignedDocType maps a legacy `agent` document's path to its new (type, path):
// a `plans/` prefix -> DocPlan, anything else -> DocSpec; the leading
// `specs/`|`plans/` segment is stripped so path becomes the slim node-local slug.
func RedesignedDocType(path string) (DocumentType, string) {
	switch {
	case strings.HasPrefix(path, "plans/"):
		return DocPlan, strings.TrimPrefix(path, "plans/")
	case strings.HasPrefix(path, "specs/"):
		return DocSpec, strings.TrimPrefix(path, "specs/")
	default:
		return DocSpec, path
	}
}
