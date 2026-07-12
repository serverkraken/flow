package webui

import (
	"sort"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzymatch"
)

// scoredInsertRow pairs an already-built InsertPickerRow with its fuzzy-match
// score, so BuildArtefaktInsertRows/BuildSeitenInsertRows can share the same
// sort+cap tail (sortScoredRows) that BuildPaletteVM (palette_vm.go) applies
// to its own Nodes/Docs slices.
type scoredInsertRow struct {
	row   components.InsertPickerRow
	score int
}

// sortScoredRows sorts rows by score (desc, stable) and caps them at
// paletteMaxRows — the editor pickers are a dropdown, not a full list, same
// row budget as the ⌘K-Palette.
func sortScoredRows(rows []scoredInsertRow) []components.InsertPickerRow {
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].score > rows[j].score })
	if len(rows) > paletteMaxRows {
		rows = rows[:paletteMaxRows]
	}
	out := make([]components.InsertPickerRow, len(rows))
	for i, r := range rows {
		out[i] = r.row
	}
	return out
}

// BuildArtefaktInsertRows turns the Ahnenkette's artifact list
// (usecase.ListArtifacts — already scoped to the editor's node plus its
// ancestors) into "Artefakt einfügen" picker rows: Label=Name, Sub=Slug,
// Value=the ![[slug]] embed markdown the row's selection inserts. An empty q
// keeps List's own (newest-first) order; a non-empty q fuzzy-matches Name
// first (weighted higher, like the palette's short-name preference), then
// falls back to Slug.
func BuildArtefaktInsertRows(arts []domain.Artifact, q string) []components.InsertPickerRow {
	rows := make([]scoredInsertRow, 0, len(arts))
	for i, a := range arts {
		row := components.InsertPickerRow{Label: a.Name, Sub: a.Slug, Value: "![[" + a.Slug + "]]"}
		if q == "" {
			rows = append(rows, scoredInsertRow{row, len(arts) - i})
			continue
		}
		if _, s, ok := fuzzymatch.Match(q, a.Name); ok {
			rows = append(rows, scoredInsertRow{row, s + 1000})
		} else if _, s, ok := fuzzymatch.Match(q, a.Slug); ok {
			rows = append(rows, scoredInsertRow{row, s})
		}
	}
	return sortScoredRows(rows)
}

// BuildSeitenInsertRows mirrors BuildPaletteVM's document filtering (⌘K
// pattern): an empty q keeps docs in their own (updated-desc) order; a
// non-empty q fuzzy-matches Title first, then Path. Each row's Value is
// [[path]] — the wikilink target domain.ResolveWikilink expects.
func BuildSeitenInsertRows(docs []domain.Document, q string) []components.InsertPickerRow {
	rows := make([]scoredInsertRow, 0, len(docs))
	for i, d := range docs {
		row := components.InsertPickerRow{Label: d.Title, Sub: d.Path, Value: "[[" + d.Path + "]]"}
		if q == "" {
			rows = append(rows, scoredInsertRow{row, len(docs) - i})
			continue
		}
		if _, s, ok := fuzzymatch.Match(q, d.Title); ok {
			rows = append(rows, scoredInsertRow{row, s + 1000})
		} else if _, s, ok := fuzzymatch.Match(q, d.Path); ok {
			rows = append(rows, scoredInsertRow{row, s})
		}
	}
	return sortScoredRows(rows)
}
