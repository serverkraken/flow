package webui

import (
	"sort"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
)

// ArtifactUse ist eine Karte, die ein Artefakt einbettet oder verlinkt.
type ArtifactUse struct {
	Title, Href string
}

// ArtifactRefs sammelt je Artefakt-Slug die Karten, die es nennen — als
// Einbettung ![[slug]] oder Verweis [[slug]] (beide tragen „[[slug"). Ein
// Artefakt ohne Verweis ist „ohne Verweis": löschen tut dann niemandem weh.
// Die Liste ist nach Titel sortiert; der Aufrufer kappt sie.
func ArtifactRefs(docs []domain.Document) map[string][]ArtifactUse {
	out := map[string][]ArtifactUse{}
	for _, d := range docs {
		if d.Archived {
			continue
		}
		seen := map[string]bool{}
		body := d.Body
		for {
			i := strings.Index(body, "[[")
			if i < 0 {
				break
			}
			rest := body[i+2:]
			j := strings.IndexAny(rest, "]|#")
			if j < 0 {
				break
			}
			slug := strings.TrimSpace(rest[:j])
			body = rest[j:]
			if slug == "" || seen[slug] {
				continue
			}
			seen[slug] = true
			out[slug] = append(out[slug], ArtifactUse{Title: d.Title, Href: "/wissen/" + d.ID})
		}
	}
	for k := range out {
		sort.SliceStable(out[k], func(i, j int) bool { return out[k][i].Title < out[k][j].Title })
	}
	return out
}

// AttachArtifactRefs hängt den Karten ihre Verweise an (gekappt auf drei
// Namen, die Zahl bleibt ganz).
func AttachArtifactRefs(cards []ArtifactCardVM, refs map[string][]ArtifactUse) {
	for i := range cards {
		rs := refs[cards[i].Slug]
		cards[i].Refs = len(rs)
		if len(rs) > 3 {
			rs = rs[:3]
		}
		cards[i].RefDocs = rs
	}
}

// ArtifactTypeFilter ist die Sicht der Bibliothek: alle, Bilder, PDF, Daten.
func ArtifactTypeFilter(raw string) string {
	switch raw {
	case "bild", "pdf", "daten":
		return raw
	}
	return ""
}

// artifactTypeKey ordnet ein MIME der Filtersicht zu.
func artifactTypeKey(card ArtifactCardVM) string {
	switch {
	case card.IsImage:
		return "bild"
	case card.TypeLabel == "PDF":
		return "pdf"
	default:
		return "daten"
	}
}
