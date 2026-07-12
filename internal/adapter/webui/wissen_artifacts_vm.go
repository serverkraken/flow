package webui

import "github.com/serverkraken/flow/internal/domain"

// WissenArtifactsVM is the view-model for the free-artifact gallery page
// (/wissen/artefakte, Task 4) — the owner-global counterpart of the cockpit's
// NodeCockpit.Artifacts/PanelErr pair, but scoped to its own small VM since
// this page has no node/rail/main context at all.
type WissenArtifactsVM struct {
	Cards    []ArtifactCardVM
	PanelErr string
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
