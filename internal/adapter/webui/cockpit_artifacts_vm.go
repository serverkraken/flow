package webui

import "github.com/serverkraken/flow/internal/domain"

// ArtifactCardVM is one gallery card in the cockpit's artifact section
// (Task 5). Inherited marks a card whose artifact hangs off an ANCESTOR
// node rather than the cockpit's own node — read-only in the templ (no
// rename/delete/replace affordance), with FromNode carrying the owning
// ancestor's display name for the origin marker. Href already carries the
// image cache-buster ("?v={ref}") for IsImage cards (Muster ArtifactRef,
// artifact_embed.go) — the struct itself stays Ref-free per the Task-5
// brief's exact field list, so the versioned query is baked in at build time
// instead of recomputed in the templ.
type ArtifactCardVM struct {
	Slug      string
	Name      string
	Href      string
	SizeStr   string
	TypeLabel string
	IsImage   bool
	Inherited bool
	FromNode  string
}

// artifactTypeLabels maps the artifact MIME allowlist (domain.go) to a short
// monospace-glyph chip label (OE #3 — no emoji, just a kürzel). A MIME
// outside the allowlist can't reach here (UploadArtifact rejects it before
// Put), but "FILE" is a safe fallback rather than an empty chip.
var artifactTypeLabels = map[string]string{
	"application/pdf":          "PDF",
	"text/csv":                 "CSV",
	"text/plain":               "TXT",
	"application/json":         "JSON",
	"application/zip":          "ZIP",
	"application/octet-stream": "BIN",
}

func artifactTypeLabel(mime string) string {
	if l, ok := artifactTypeLabels[mime]; ok {
		return l
	}
	return "FILE"
}

// BuildArtifactCards converts ListArtifacts' Ahnenkette result (Node +
// ancestors, nearest first) into gallery display cards. nodeID is the
// cockpit's OWN node id — an artifact whose NodeID differs is inherited;
// names maps a node id to its display name (the caller's already-loaded
// owner-nodes map, Muster nodeCockpitData's "names") for the origin label.
// A missing names entry degrades to an empty FromNode rather than panicking.
// A free (node-less, Task 2) artifact has NodeID=="", which never equals a
// real nodeID, so it is always Inherited=true and gets an /artefakte/{slug}
// href; its origin label comes from names[""], which the caller sets to the
// "Frei" i18n string (Task 3) — this function does not special-case it.
func BuildArtifactCards(arts []domain.Artifact, nodeID string, names map[string]string) []ArtifactCardVM {
	cards := make([]ArtifactCardVM, 0, len(arts))
	for _, a := range arts {
		var href string
		if a.NodeID == "" {
			href = "/artefakte/" + a.Slug
		} else {
			href = "/nodes/" + a.NodeID + "/artifacts/" + a.Slug
		}
		if a.IsImage() {
			href += "?v=" + a.Ref
		}
		card := ArtifactCardVM{
			Slug:      a.Slug,
			Name:      a.Name,
			Href:      href,
			SizeStr:   FormatArtifactSize(a.SizeBytes),
			TypeLabel: artifactTypeLabel(a.Mime),
			IsImage:   a.IsImage(),
			Inherited: a.NodeID != nodeID,
		}
		if card.Inherited {
			card.FromNode = names[a.NodeID]
		}
		cards = append(cards, card)
	}
	return cards
}
