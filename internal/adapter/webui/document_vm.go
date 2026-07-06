package webui

import (
	"html/template"
)

// DocCrumb is one Spine ".up" crumb on the document page: a clickable
// ancestor node. Deliberately webui-local (not components.Crumb) — the
// document page's crumb chain always ends in a plain, non-clickable "Wissen"
// label rendered separately by the templ, not as a trailing DocCrumb.
type DocCrumb struct {
	Label, Href string
}

// DocumentVM is the Lesesaal document-page view model: Spine (path
// breadcrumb + title), Provenance row (actor/time/path/reading time/
// pin/edit/delete), the rendered prose, and the ToC rail. Backlinks/Outgoing
// (Verweise rail) are Task 6's addition — Task 5's docrail only carries the
// ToC block.
type DocumentVM struct {
	ID      string
	Title   string
	Path    string
	Crumbs  []DocCrumb
	// UpdatedByKind/UpdatedByRef mirror domain.Document — both empty means
	// unknown/pre-provenance (Task 3): the Prov row then renders a neutral
	// avatar without a bold actor name, just time+path+reading time.
	UpdatedByKind string
	UpdatedByRef  string
	UpdatedRel    string // pre-formatted relative time, e.g. "vor 3 Min"
	ReadMinutes   int
	Pinned        bool
	HTML          template.HTML
	// Embed mirrors the Kristall-era embedding-status affordance
	// (TestWebDocumentView_EmbedBadgeFailedShowsRetry) — kept verbatim, only
	// its chrome is de-glassed (DocumentEmbedBadge).
	Embed *EmbedView
}

func embedLabelKey(state string) string {
	switch state {
	case "ok":
		return "embed.ok"
	case "pending":
		return "embed.pending"
	case "retrying":
		return "embed.retrying"
	case "failed":
		return "embed.failed"
	default:
		return "embed.unknown"
	}
}

func embedToneClass(state string) string {
	switch state {
	case "ok":
		return "bg-success/10 text-success border-success/30"
	case "pending":
		return "bg-sunken text-muted border-line"
	case "retrying":
		return "bg-warning/10 text-warning border-warning/35"
	case "failed":
		return "bg-danger/10 text-danger border-danger/35"
	default:
		return "bg-sunken text-muted border-line"
	}
}
