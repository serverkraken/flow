package webui

import (
	"sort"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzymatch"
)

// PaletteNodeVM / PaletteDocVM sind je eine Sprungzeile der ⌘K-Palette.
type PaletteNodeVM struct {
	ID, Short, Full, Initials, Tone, Kind string
	Score                                 int
}

type PaletteDocVM struct {
	ID, Title, Path, Type string
	Score                 int
}

type PaletteVM struct {
	Query string
	Nodes []PaletteNodeVM
	Docs  []PaletteDocVM
}

const paletteMaxRows = 8

// BuildPaletteVM filtert Knoten + Dokumente fuzzy (Spec §5.4). Leere Query:
// recentNodeIDs (MRU) zuerst, dann alle Knoten alphabetisch; Docs kommen in
// der gelieferten (updated-desc) Reihenfolge. Mit Query gewinnt der Score.
func BuildPaletteVM(nodes []domain.Node, recentNodeIDs []string, docs []domain.Document, q string) PaletteVM {
	vm := PaletteVM{Query: q}
	recent := map[string]int{}
	for i, id := range recentNodeIDs {
		recent[id] = len(recentNodeIDs) - i // jünger = höher
	}
	for _, n := range nodes {
		short := ShortName(n.Name)
		row := PaletteNodeVM{ID: n.ID, Short: short, Full: n.Name,
			Initials: Initials(short), Tone: AvatarTone(n.Name), Kind: string(n.Kind)}
		if q == "" {
			row.Score = recent[n.ID]
			vm.Nodes = append(vm.Nodes, row)
			continue
		}
		if _, s, ok := fuzzymatch.Match(q, short); ok {
			row.Score = s + 1000 // Kurzname-Treffer schlagen Pfad-Treffer
		} else if _, s, ok := fuzzymatch.Match(q, n.Name); ok {
			row.Score = s
		} else {
			continue
		}
		vm.Nodes = append(vm.Nodes, row)
	}
	sort.SliceStable(vm.Nodes, func(i, j int) bool { return vm.Nodes[i].Score > vm.Nodes[j].Score })
	if len(vm.Nodes) > paletteMaxRows {
		vm.Nodes = vm.Nodes[:paletteMaxRows]
	}
	for i, d := range docs {
		row := PaletteDocVM{ID: d.ID, Title: d.Title, Path: d.Path, Type: string(d.Type)}
		if q == "" {
			row.Score = len(docs) - i
			vm.Docs = append(vm.Docs, row)
			continue
		}
		if _, s, ok := fuzzymatch.Match(q, d.Title); ok {
			row.Score = s + 1000
		} else if _, s, ok := fuzzymatch.Match(q, d.Path); ok {
			row.Score = s
		} else {
			continue
		}
		vm.Docs = append(vm.Docs, row)
	}
	sort.SliceStable(vm.Docs, func(i, j int) bool { return vm.Docs[i].Score > vm.Docs[j].Score })
	if len(vm.Docs) > paletteMaxRows {
		vm.Docs = vm.Docs[:paletteMaxRows]
	}
	return vm
}
