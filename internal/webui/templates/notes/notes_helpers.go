// Package notes renders the WebUI notes surface at `/notes` and
// `/notes/:id`. All data resolution happens in the handler; templates
// only render formatted strings off a flat view-model.
//
// Shared formatters (date headers, relative time) live in
// internal/webui/format/. This file keeps only the notes-specific
// view-model shape + the labels that map domain.NoteType to German UI
// strings.
package notes

import (
	"strings"

	"github.com/a-h/templ"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

// noteViewHref returns the canonical /notes/:id link wrapped in a
// templ.SafeURL so the templ surface can use it in href attributes
// without further escaping. The kompendium ID may contain `/` (e.g.
// "projects/serverkraken/flow/foo") — net/http's mux decodes path
// segments per-segment, so we pass the raw ID through.
func noteViewHref(id string) templ.SafeURL {
	return templ.SafeURL("/notes/" + id)
}

// noteFormAction returns the URL the edit form posts to. Same path as
// the view, with the HTML form using POST + _method=PUT for the no-JS
// fallback (HTMX upgrades the request to a true PUT when available).
func noteFormAction(id string) templ.SafeURL {
	return templ.SafeURL("/notes/" + id)
}

// SubTab identifies the active notes index sub-tab. Values are
// lower-case and map to `?type=` query values so the URL reads
// `/notes?type=daily`. "alle" is the default which omits the type
// filter on the underlying ListNotes call.
type SubTab string

// Defined sub-tab values.
const (
	TabAlle    SubTab = "alle"
	TabDaily   SubTab = "daily"
	TabProject SubTab = "project"
	TabFree    SubTab = "frei"
)

// ParseSubTab maps a raw `?type=` query value to a SubTab; an empty or
// unknown value falls through to "alle" so a typo never 400s.
func ParseSubTab(raw string) SubTab {
	switch SubTab(raw) {
	case TabDaily, TabProject, TabFree:
		return SubTab(raw)
	default:
		return TabAlle
	}
}

// AsNoteType converts a SubTab to the kompendium NoteType used by the
// ListNotes filter. TabAlle returns the empty NoteType which the
// usecase treats as "no filter".
func (t SubTab) AsNoteType() domain.NoteType {
	switch t {
	case TabDaily:
		return domain.TypeDaily
	case TabProject:
		return domain.TypeProject
	case TabFree:
		return domain.TypeFree
	default:
		return ""
	}
}

// GermanTypeLabel maps a NoteType to its German UI label.
func GermanTypeLabel(t domain.NoteType) string {
	switch t {
	case domain.TypeDaily:
		return "Daily"
	case domain.TypeProject:
		return "Project"
	case domain.TypeFree:
		return "Frei"
	default:
		return "—"
	}
}

// previewBudget caps how many runes of body the index preview shows.
// Enough for two short sentences; the rest is replaced with an
// ellipsis.
const previewBudget = 220

// BuildPreview returns the first paragraph of body, trimmed and
// length-capped. Headings, blockquotes, and code-fence lines are
// dropped — the preview should read like prose, not raw markdown.
func BuildPreview(body []byte) string {
	lines := strings.Split(string(body), "\n")
	var collected []string
	inFence := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if line == "" {
			if len(collected) > 0 {
				break // first paragraph collected, stop
			}
			continue
		}
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ">") {
			continue
		}
		collected = append(collected, line)
	}
	joined := strings.Join(collected, " ")
	if joined == "" {
		return ""
	}
	if len(joined) <= previewBudget {
		return joined
	}
	cut := joined[:previewBudget]
	// Avoid cutting in the middle of a UTF-8 sequence. strings.Index
	// on the last space is good enough; the preview is decorative.
	if sp := strings.LastIndex(cut, " "); sp > previewBudget/2 {
		cut = cut[:sp]
	}
	return strings.TrimRight(cut, " ,;.") + " …"
}

// TitleOf returns the display title for a note, preferring the YAML
// frontmatter `title` and falling back to the first H1 found in body,
// then to the note's path-based ID. Used by the index list and the
// view breadcrumb.
func TitleOf(fmTitle string, body []byte, idFallback string) string {
	if strings.TrimSpace(fmTitle) != "" {
		return strings.TrimSpace(fmTitle)
	}
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(trimmed[2:])
		}
	}
	return idFallback
}
