package webui

import "github.com/serverkraken/flow/internal/domain"

// WissenArtifactsVM is the view-model for the free-artifact gallery page
// (/wissen/artefakte, Task 4) — the owner-global counterpart of the cockpit's
// NodeCockpit.Artifacts/PanelErr pair, but scoped to its own small VM since
// this page has no node/rail/main context at all.
type WissenArtifactsVM struct {
	Cards    []ArtifactCardVM
	PanelErr string
	// Screen 34: Bestand und Sicht.
	Total, Unreferenced int
	CountBild, CountPDF, CountDaten int
	Filter                          string // "" | bild | pdf | daten
}

// FilterArtifactCards zählt und filtert nach der Sicht.
func FilterArtifactCards(vm WissenArtifactsVM, filter string) WissenArtifactsVM {
	all := vm.Cards
	vm.Total = len(all)
	vm.Filter = filter
	kept := make([]ArtifactCardVM, 0, len(all))
	for _, c := range all {
		switch artifactTypeKey(c) {
		case "bild":
			vm.CountBild++
		case "pdf":
			vm.CountPDF++
		default:
			vm.CountDaten++
		}
		if c.Refs == 0 {
			vm.Unreferenced++
		}
		if filter == "" || artifactTypeKey(c) == filter {
			kept = append(kept, c)
		}
	}
	vm.Cards = kept
	return vm
}

// BuildWissenArtifactsVM converts ListArtifacts' free-library result
// (Execute(owner, "") == ListFree(owner)) into the gallery VM. Calling
// BuildArtifactCards with nodeID=="" marks every card Inherited==false — a
// free artifact's own NodeID is "" too, so it always matches — which is
// exactly what this page needs: it's the owner's OWN library, not a
// read-only inherited view like the cockpit gallery's free cards. The names
// map is nil because Inherited is always false here, so FromNode is never
// read (see BuildArtifactCards).
func BuildWissenArtifactsVM(arts []domain.Artifact, panelErr string) WissenArtifactsVM {
	return WissenArtifactsVM{
		Cards:    BuildArtifactCards(arts, "", nil),
		PanelErr: panelErr,
	}
}
