// Package handlers implements the WebUI HTTP handlers.
package handlers

import (
	"fmt"
	"path"
	"strings"
	"time"

	flowports "github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/webui/format"
	notestmpl "github.com/serverkraken/flow/internal/webui/templates/notes"
)

// buildDocumentsIndexVM maps DocumentEntry rows onto the existing notes
// IndexVM. Typ-Sub-Tabs filtern in R1 nicht (Dokumente haben keine
// kompendium-Typen) — ActiveTab bleibt TabAlle, der Strip bleibt sichtbar.
func buildDocumentsIndexVM(entries []flowports.DocumentEntry, query string, clock flowports.Clock) notestmpl.IndexVM {
	now := time.Now()
	if clock != nil {
		now = clock.Now()
	}
	rows := make([]notestmpl.IndexRow, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, notestmpl.IndexRow{
			ID:      e.Path,
			Title:   strings.TrimSuffix(path.Base(e.Path), ".md"),
			Type:    docTypeLabel(e.Path),
			When:    format.HumanRelativeTime(e.UpdatedAt, now),
			Preview: e.Snippet, // FTS-Headline bei Suche, sonst leer
			Href:    "/notes/" + e.Path,
		})
	}
	vm := notestmpl.IndexVM{
		ActiveTab:  notestmpl.TabAlle,
		Query:      query,
		Configured: true, // documents-API ist immer da — kein NOTEBOOK_ROOT-Placeholder mehr
		Rows:       rows,
		TotalLabel: documentsTotalLabel(len(rows)),
	}
	if len(rows) == 0 {
		if query != "" {
			vm.EmptyReason = "search"
		} else {
			vm.EmptyReason = "empty"
		}
	}
	return vm
}

func documentsTotalLabel(n int) string {
	if n == 1 {
		return "1 Note"
	}
	return fmt.Sprintf("%d Notes", n)
}

// docTypeLabel derives the badge from the path root — the directory
// layout survives the import 1:1 (daily/…, projects/…, repos/…).
func docTypeLabel(docPath string) string {
	switch {
	case strings.HasPrefix(docPath, "daily/"):
		return "Daily"
	case strings.HasPrefix(docPath, "projects/"):
		return "Project"
	case strings.HasPrefix(docPath, "repos/"):
		return "Repo"
	default:
		return "Frei"
	}
}

// buildDocumentViewVM renders the markdown body into the notes ViewVM.
func buildDocumentViewVM(d DocumentsDeps, doc flowports.Document) (notestmpl.ViewVM, error) {
	html, err := d.Markdown.Render([]byte(doc.Body))
	if err != nil {
		html = "" // degrade wie bei den alten notes: Rail bleibt nutzbar
	}
	now := time.Now()
	if d.Clock != nil {
		now = d.Clock.Now()
	}
	vm := notestmpl.ViewVM{
		ID:            doc.Path,
		Title:         docTitle(doc.Path, doc.Body),
		Path:          doc.Path,
		TypeLabel:     docTypeLabel(doc.Path),
		HTML:          html,
		CreatedLabel:  "—", // documents tragen kein created-Datum; ehrlich statt geraten
		ModifiedLabel: format.HumanRelativeTime(doc.UpdatedAt, now),
		SyncLabel:     fmt.Sprintf("server · v%d", doc.Version),
		BreadcrumbHrefs: notestmpl.Breadcrumb{
			NotesHref: "/notes",
			TypeHref:  "/notes",
		},
	}
	for _, h := range d.Markdown.Headings([]byte(doc.Body)) {
		vm.Headings = append(vm.Headings, notestmpl.HeadingItem{
			Level: h.Level, Text: h.Text, Anchor: h.Anchor,
		})
	}
	return vm, nil
}
