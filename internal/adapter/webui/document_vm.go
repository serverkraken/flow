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
// pin/edit/delete), the rendered prose, the ToC rail, and the Verweise rail
// (Outgoing/Backlinks, Task 6).
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
	// Outgoing/Backlinks feed the docrail's Verweise `.blk` (Task 6, Mockup
	// Z.788-793): Outgoing are this document's own resolved wikilinks ("von
	// hier"), Backlinks are other documents that link here ("hierher"). Only
	// resolved targets appear — an unresolved (broken) wikilink stays visible
	// as `.wikilink-broken` in the prose but is never listed in the rail.
	Outgoing  []RefRow
	Backlinks []RefRow
	// Embed mirrors the Kristall-era embedding-status affordance
	// (TestWebDocumentView_EmbedBadgeFailedShowsRetry) — kept verbatim, only
	// its chrome is de-glassed (DocumentEmbedBadge).
	Embed *EmbedView
	// Context feeds the docrail's "Im Agenten-Kontext" `.blk` (Task 6, Mockup
	// Z.794-798) plus the mode switcher (Task 4): nil means no block at all
	// (non-context-type doc, or Compose itself failed) — for every
	// context-eligible type, buildDocumentVM always builds one so the
	// switcher stays reachable, even in the "absent"/"nie" states.
	Context *DocContextVM
}

// RefRow is one Verweise-rail `.krow` line (Task 6): a resolved reference to
// another document, either outgoing (this doc links to it) or a backlink
// (it links to this doc). Dir carries the i18n key for the direction label
// ("document.ref.from"/"document.ref.to"), not display text.
type RefRow struct {
	Title string
	Href  string
	Dir   string
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
