package httpserver

import (
	"errors"
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

func (s *Server) handleWebDocumentView(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	doc, err := s.GetDocument.Execute(r.Context(), u.ID, id)
	if errors.Is(err, ports.ErrDocumentNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	vm, err := s.buildDocumentVM(r, u.ID, doc)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DocumentPage(vm).Render(r.Context(), w)
}

// buildDocumentVM builds the Lesesaal DocumentVM shared by the full page view
// (handleWebDocumentView) and the Anpinnen round-trip (handleWebDocPin),
// which both render the same #document-fragment.
func (s *Server) buildDocumentVM(r *http.Request, ownerID string, doc domain.Document) (webui.DocumentVM, error) {
	all, err := s.ListDocuments.Execute(r.Context(), ownerID, nil, nil)
	if err != nil {
		return webui.DocumentVM{}, err
	}
	resolve := func(target string) (string, string, bool) {
		if t, ok := domain.ResolveWikilink(doc, target, all); ok {
			return "/wissen/" + t.ID, t.Title, true
		}
		return "", "", false
	}

	// Artefact-Resolver (Task 3): built ALWAYS, even for a free doc
	// (doc.NodeID == nil → nodeID == ""), since ListArtifacts.Execute("")
	// resolves to that owner's free library alone (free-artifacts Task 3) —
	// a free doc's ![[slug]] embed must resolve against it, matching Spec
	// E1 ("a free doc sees only the free library"). Before this fix the
	// resolver was only built inside `if doc.NodeID != nil`, so a free doc's
	// embed always stayed unresolved regardless of a matching free artifact.
	// Fire-and-forget like the crumbs lookup just below, which reuses the
	// SAME chain — an ancestor-lookup or artifact-list failure just degrades
	// every ![[slug]] embed to "unresolved" rather than a hard 500 for what
	// is, worst case, a broken inline figure.
	var chain []domain.Node
	nodeID := ""
	if doc.NodeID != nil {
		nodeID = *doc.NodeID
		chain, _ = s.NodeAncestors.Execute(r.Context(), ownerID, nodeID)
	}
	var resolveArtifact webui.ArtifactResolver
	if arts, aerr := s.ListArtifacts.Execute(r.Context(), ownerID, nodeID); aerr == nil {
		resolveArtifact = buildArtifactResolver(chain, arts)
	}
	rendered, _ := webui.RenderDocument(r.Context(), doc.Body, resolve, resolveArtifact)

	vm := webui.DocumentVM{
		ID:            doc.ID,
		Title:         doc.Title,
		Path:          doc.Path,
		HTML:          rendered,
		UpdatedByKind: doc.UpdatedByKind,
		UpdatedByRef:  doc.UpdatedByRef,
		UpdatedRel:    webui.FmtRelTime(doc.UpdatedAt, s.Clock.Now()),
		ReadMinutes:   webui.ReadingTime(doc.Body),
		Pinned:        doc.Pinned,
		Outgoing:      buildOutgoingRefs(doc, all),
	}
	if refs, rerr := s.BacklinksDocument.Execute(r.Context(), ownerID, doc.ID); rerr == nil {
		for _, b := range refs {
			vm.Backlinks = append(vm.Backlinks, webui.RefRow{Title: b.Title, Href: "/wissen/" + b.ID, Dir: "document.ref.to"})
		}
	}
	if doc.NodeID != nil {
		for i := len(chain) - 1; i >= 0; i-- {
			n := chain[i]
			vm.Crumbs = append(vm.Crumbs, webui.DocCrumb{Label: n.Name, Href: "/nodes/" + n.ID})
		}
	}
	if s.GetEmbedStatus.Docs != nil {
		if st, serr := s.GetEmbedStatus.Execute(r.Context(), ownerID, doc.ID); serr == nil {
			vm.Embed = &webui.EmbedView{
				State:     string(st.State),
				LastError: truncateError(st.LastError),
				ShowRetry: st.State == domain.EmbedFailed,
			}
		}
	}

	// Kontext-Rang (L5, Modus-Umschalter Task 4): only context-eligible docs
	// participate in Compose. Compose the OWNING node's context by ID (nil
	// node → global/unresolved context via Execute). Guarded/owner-scoped; a
	// compose error just omits the block. BuildDocContext now always returns
	// a non-nil VM (never nil for absent/nie) so the mode switcher stays
	// reachable in every standing.
	if isContextType(doc.Type) && s.ComposeContext.Nodes != nil {
		budget := s.ContextBudget
		if budget <= 0 {
			budget = 12000
		}
		var cc usecase.ComposedContext
		var cerr error
		if doc.NodeID != nil {
			cc, cerr = s.ComposeContext.ExecuteForNode(r.Context(), ownerID, *doc.NodeID, budget)
		} else {
			cc, cerr = s.ComposeContext.Execute(r.Context(), ownerID, usecase.ContextResolveInput{}, budget)
		}
		if cerr == nil {
			nodeName := ""
			if len(vm.Crumbs) > 0 {
				nodeName = vm.Crumbs[len(vm.Crumbs)-1].Label // leaf crumb = doc's node
			}
			vm.Context = webui.BuildDocContext(usecase.StandingOf(cc, doc.ID), nodeName, doc.ContextMode.OrAuto())
		}
	}
	return vm, nil
}

// isContextType reports whether t is one of the three context-eligible
// document types that participate in Compose (memory/instruction/
// activecontext). All other types never show the "Im Agenten-Kontext" block —
// buildDocumentVM skips the Compose call entirely for them.
func isContextType(t domain.DocumentType) bool {
	switch t {
	case domain.DocMemory, domain.DocInstruction, domain.DocActiveContext:
		return true
	default:
		return false
	}
}

// buildArtifactResolver turns the flat, newest-first list ListArtifacts
// returns into a slug-keyed webui.ArtifactResolver, applying the
// nearest-ancestor-wins rule (Spec §6.1): chain is the document node's own
// ancestor chain in NodeStore.Ancestors order (leaf→root, chain[0] = the
// document's own node), so its slice index IS the "how close" ranking. When
// the same slug exists on more than one node in the chain (e.g. one on the
// document's own node, another on an ancestor), the artifact at the LOWEST
// chain index — i.e. nearest to the document — wins; List's own
// created-at-desc ordering is irrelevant here and deliberately not relied
// upon. Href always points at the winning artifact's OWN NodeID, which can
// be an ancestor's, never the document's node unconditionally — otherwise
// the serve route 404s for artifacts inherited from a parent. Free (node-
// less, Task 2) artifacts rank at position len(chain) — root-lowest, below
// every chain node — and get an /artefakte/{slug} href instead; nearest-
// wins still applies unchanged, so a chain node's artifact always beats a
// free artifact of the same slug.
func buildArtifactResolver(chain []domain.Node, arts []domain.Artifact) webui.ArtifactResolver {
	pos := make(map[string]int, len(chain))
	for i, n := range chain {
		pos[n.ID] = i
	}
	freePos := len(chain) // free artifacts rank below the root (lowest priority, E1)
	best := make(map[string]domain.Artifact, len(arts))
	bestPos := make(map[string]int, len(arts))
	for _, a := range arts {
		p, ok := pos[a.NodeID]
		if !ok {
			if a.NodeID != "" {
				continue // artifact on a non-ancestor node — not reachable here
			}
			p = freePos // free (node-less) artifact — lowest priority
		}
		if cur, seen := bestPos[a.Slug]; !seen || p < cur {
			best[a.Slug] = a
			bestPos[a.Slug] = p
		}
	}
	if len(best) == 0 {
		return nil
	}
	return func(slug string) (webui.ArtifactRef, bool) {
		a, ok := best[slug]
		if !ok {
			return webui.ArtifactRef{}, false
		}
		href := "/nodes/" + a.NodeID + "/artifacts/" + a.Slug
		if a.NodeID == "" {
			href = "/artefakte/" + a.Slug
		}
		return webui.ArtifactRef{
			Href:    href,
			Ref:     a.Ref,
			Name:    a.Name,
			Mime:    a.Mime,
			SizeStr: webui.FormatArtifactSize(a.SizeBytes),
			IsImage: a.IsImage(),
			Width:   a.Width,
			Height:  a.Height,
		}, true
	}
}

// buildOutgoingRefs resolves doc's own wikilink targets against the
// already-loaded all slice (no extra store call — Task 6) into the Verweise
// rail's "von hier" rows. Only resolved targets appear (an unresolved target
// stays a `.wikilink-broken` span in the prose, never listed here); results
// are de-duplicated by resolved document ID since distinct wikilink targets
// can resolve to the same document.
func buildOutgoingRefs(doc domain.Document, all []domain.Document) []webui.RefRow {
	seen := make(map[string]bool)
	var out []webui.RefRow
	for _, target := range domain.WikilinkTargets(doc.Body) {
		t, ok := domain.ResolveWikilink(doc, target, all)
		if !ok || seen[t.ID] {
			continue
		}
		seen[t.ID] = true
		out = append(out, webui.RefRow{Title: t.Title, Href: "/wissen/" + t.ID, Dir: "document.ref.from"})
	}
	return out
}

func (s *Server) handleWebDocReembed(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	if err := s.RetryEmbedding.Execute(r.Context(), u.ID, id); err != nil {
		if errors.Is(err, ports.ErrDocumentNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if r.Header.Get("HX-Request") == "" {
		http.Redirect(w, r, "/wissen/"+id, http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DocumentEmbedBadge(id, webui.EmbedView{State: "pending"}).Render(r.Context(), w)
}

// handleWebDocPin toggles a document's pinned state (POST /wissen/{id}/pin —
// the Anpinnen button in the Provenance row), emits document.updated so the
// #document-fragment SSE-refreshes everywhere else the doc is open, and
// returns the fresh fragment for the button's own hx-swap="outerHTML".
func (s *Server) handleWebDocPin(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	doc, err := s.GetDocument.Execute(r.Context(), u.ID, id)
	if errors.Is(err, ports.ErrDocumentNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if err := s.SetPinned.Execute(r.Context(), u.ID, id, !doc.Pinned); err != nil {
		if errors.Is(err, ports.ErrDocumentNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	doc.Pinned = !doc.Pinned
	s.emitEvent(r.Context(), domain.Event{Type: domain.EventDocumentUpdated, UserID: u.ID, Data: map[string]any{"id": id}})

	vm, err := s.buildDocumentVM(r, u.ID, doc)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DocumentFragment(vm).Render(r.Context(), w)
}

// handleWebDocMode toggles a document's agent-context mode (POST
// /wissen/{id}/mode — the docrail context block's Auto/Immer/Nie switcher,
// Task 4), emits document.updated so #document-fragment SSE-refreshes
// everywhere else the doc is open, and returns the fresh fragment for the
// switcher's own hx-swap="outerHTML". Mirrors handleWebDocPin's shape: an
// unknown/foreign doc id (ErrDocumentNotFound) and an invalid mode string
// (ErrInvalidDocument, belt-and-suspenders with the DB CHECK) both degrade to
// a clean no-op re-render — never a 500.
func (s *Server) handleWebDocMode(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := r.PathValue("id")
	_ = r.ParseForm()
	mode := r.FormValue("mode")

	doc, err := s.GetDocument.Execute(r.Context(), u.ID, id)
	if errors.Is(err, ports.ErrDocumentNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	if err := s.SetContextMode.Execute(r.Context(), u.ID, id, domain.ContextMode(mode)); err == nil {
		doc.ContextMode = domain.ContextMode(mode)
		s.emitEvent(r.Context(), domain.Event{Type: domain.EventDocumentUpdated, UserID: u.ID, Data: map[string]any{"id": id}})
	} else if !contextModeErrKnown(err) {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	vm, err := s.buildDocumentVM(r, u.ID, doc)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.DocumentFragment(vm).Render(r.Context(), w)
}

// wissenShelfHref is no longer used by the document page itself (Lesesaal
// Spine replaces the old "Zurück zu Kategorie" breadcrumb), but
// webui_editor.go still uses it as the post-delete redirect target — kept
// for that caller (Bestand gewinnt; rg-verified). Replaces the old
// wissenCategoryHrefAndLabel now that Wissen groups documents by type-shelf
// (Task 7) instead of the four-way daily/projekte/frei/system split.
func wissenShelfHref(doc domain.Document) string {
	if shelf, ok := webui.WissenShelfForType(doc.Type); ok {
		return "/wissen/typ?type=" + shelf.TypeKey
	}
	return "/wissen"
}

func truncateError(s string) string {
	const max = 80
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
