package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/markdown"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
	markdown_overlay "github.com/serverkraken/flow/internal/tui/ui/markdown_overlay"
	"github.com/serverkraken/flow/internal/tui/ui/titlebox"
)

// docEditor is the slice of the editor adapter the docs screen needs. Kept as a
// tui-local interface so DocsModel stays unit-testable with a nil editor (the
// $EDITOR path is never hit in tests). cmd/flow/docs.go passes editor.New().
type docEditor interface {
	Command(initial []byte) (*exec.Cmd, func() ([]byte, error), func(), error)
}

// urlOpener opens a URL in the OS default browser. tui-local so DocsModel stays
// testable with a nil/fake opener.
type urlOpener interface {
	Open(url string) error
}

// docMode is the docs screen sub-state.
type docMode int

const (
	modeList      docMode = iota // browsing the list
	modeView                     // reading one document's body
	modeCreating                 // entering slug/type/title before $EDITOR
	modeDeleting                 // confirming a delete
	modeFiltering                // tag-filter overlay
	modeSearch                   // / search input + results
)

// create-form fields, navigated with tab/enter.
const (
	fldType  = iota // 0
	fldPath         // 1
	fldTitle        // 2
)

// DocsModel is the `flow docs` screen: list/view compendium documents and
// create/edit their bodies via $EDITOR, live-synced via the SSE stream.
type DocsModel struct {
	client *apiclient.Client
	editor docEditor
	opener urlOpener
	user   string

	docs    []domain.Document
	sel     int
	viewing *domain.Document

	mode  docMode
	field int // active create-form field
	// create-form buffers
	newType  domain.DocumentType
	newPath  string
	newTitle string

	events    <-chan apiclient.ClientEvent
	status    string
	err       error
	viewLinks []linkTarget
	linkFocus int      // -1 = none focused
	viewStack []string // doc-id back-stack for in-TUI wikilink nav
	backlinks []domain.BacklinkRef

	// Fullscreen markdown viewer (modeView). viewer is a heap cell so the
	// RenderFunc closure sees focus updates across DocsModel value-copies.
	viewer       *viewerState
	overlay      markdown_overlay.Model
	overlayReady bool

	// Terminal size, learned from tea.WindowSizeMsg. Sizes the modeView overlay
	// in standalone `flow docs` (in `flow ui` the route's Frame→SetSize bridge
	// sizes it instead) and bounds the search-results box.
	width  int
	height int

	filterTags   []string          // applied filter (AND)
	filterWork   []string          // working set while in modeFiltering
	filterOpts   []domain.TagCount // available tags for the overlay
	filterCursor int

	searchQuery string             // current query buffer (input phase)
	searching   bool               // true once a query has been run (results phase)
	searchHits  []domain.SearchHit
	searchSel   int
}

// NewDocs builds the docs model. client/ed/op may be nil in tests that only drive
// Update and never trigger the network, $EDITOR, or URL-opener paths.
func NewDocs(client *apiclient.Client, ed docEditor, op urlOpener, user string) DocsModel {
	return DocsModel{client: client, editor: ed, opener: op, user: user, newType: domain.DocFree, linkFocus: -1}
}

// viewerState is the heap cell the fullscreen viewer's RenderFunc closes over,
// so a focus change (Tab) is visible across DocsModel value-copies without
// rebuilding the closure. focus = -1 means no wikilink focused.
type viewerState struct{ focus int }

// InViewMode reports the docs screen is reading a document fullscreen. It drives
// the shell's FullScreener takeover via the docs route adapter.
func (m DocsModel) InViewMode() bool { return m.mode == modeView }

// focusState exposes the current wikilink focus index (test-only accessor).
func (m DocsModel) focusState() int {
	if m.viewer == nil {
		return -1
	}
	return m.viewer.focus
}

// wikiAdapter resolves [[links]] against the loaded doc set for the renderer,
// emitting flow://docs/<id> URIs so the viewer can detect in-TUI navigation.
type wikiAdapter struct {
	src domain.Document
	all []domain.Document
}

func (w wikiAdapter) Resolve(target string) (uri, title string, ok bool) {
	d, ok := domain.ResolveWikilink(w.src, target, w.all)
	if !ok {
		return "", "", false
	}
	return "flow://docs/" + d.ID, d.Title, true
}

// buildRenderFunc returns a RenderFunc closing over the doc + the focus cell, so
// re-rendering after a focus change highlights the right wikilink.
func (m DocsModel) buildRenderFunc(doc domain.Document, vs *viewerState) markdown_overlay.RenderFunc {
	adapter := wikiAdapter{src: doc, all: m.docs}
	fm := &markdown.Frontmatter{
		ID:    doc.ID,
		Type:  markdown.NoteType(string(doc.Type)),
		Title: doc.Title,
		Tags:  doc.Tags,
	}
	if doc.ProjectID != nil {
		fm.Project = *doc.ProjectID
	}
	if doc.Date != nil {
		fm.Date = doc.Date.Format("2006-01-02")
	}
	bl := make([]markdown.BacklinkRef, 0, len(m.backlinks))
	for _, r := range m.backlinks {
		bl = append(bl, markdown.BacklinkRef{ID: r.ID, Title: r.Title})
	}
	return func(src string, width int) string {
		out, err := markdown.Render(src, width,
			markdown.WithWikilinks(adapter),
			markdown.WithFrontmatter(fm),
			markdown.WithBacklinks(bl),
			markdown.WithFocusedWikilink(vs.focus),
		)
		if err != nil {
			return src
		}
		return out
	}
}

// newViewerOverlay builds a fresh viewer overlay for doc, closing over vs.
func (m DocsModel) newViewerOverlay(doc domain.Document, vs *viewerState) markdown_overlay.Model {
	return markdown_overlay.New(m.buildRenderFunc(doc, vs),
		markdown_overlay.WithSource(doc.Body),
		markdown_overlay.WithSearch(),
		markdown_overlay.WithCodeCopy(),
		// DocsModel owns Esc / leaving the viewer, so the overlay must not
		// self-close. WithCloseKeys() with no args keeps the default keys
		// (q/esc/b), so pass a sentinel that never matches a real key press.
		markdown_overlay.WithCloseKeys("\x00noclose"),
	)
}

type docsLoadedMsg struct{ docs []domain.Document }
type docViewMsg struct{ doc domain.Document }
type docSavedMsg struct{}
type backlinksMsg struct{ refs []domain.BacklinkRef }

// editorReq carries an opened $EDITOR command back into Update so it can be run
// via tea.ExecProcess. readback/cleanup close over the temp file.
type editorReq struct {
	cmd      *exec.Cmd
	readback func() ([]byte, error)
	cleanup  func()
	editID   string // empty → create
	// create metadata (ignored on edit)
	typ   domain.DocumentType
	path  string
	title string
}

// editorDoneMsg is produced after $EDITOR exits: it carries the edited bytes
// and the request context so Update can persist via the apiclient.
type editorDoneMsg struct {
	body   []byte
	editID string
	typ    domain.DocumentType
	path   string
	title  string
	err    error
}

func (m DocsModel) Init() tea.Cmd {
	return tea.Batch(m.reload(), m.subscribe())
}

func (m DocsModel) reload() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		docs, err := m.client.ListDocuments(ctx, m.filterTags...)
		if err != nil {
			return errMsg{err}
		}
		return docsLoadedMsg{docs: docs}
	}
}

type tagsLoadedMsg struct{ tags []domain.TagCount }
type searchDoneMsg struct{ hits []domain.SearchHit }

func (m DocsModel) runSearch(q string) tea.Cmd {
	if m.client == nil {
		return nil
	}
	tags := m.filterTags
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		hits, err := m.client.Search(ctx, q, tags...)
		if err != nil {
			return errMsg{err}
		}
		return searchDoneMsg{hits: hits}
	}
}

func (m DocsModel) loadTags() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		tags, err := m.client.Tags(ctx)
		if err != nil {
			return errMsg{err}
		}
		return tagsLoadedMsg{tags: tags}
	}
}

func (m DocsModel) subscribe() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ch, err := m.client.Events(context.Background())
		if err != nil {
			return errMsg{err}
		}
		return eventsReadyMsg{ch}
	}
}

// loadDoc fetches the full document (body) for viewing or editing.
func (m DocsModel) loadDoc(id string, thenEdit bool) tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		doc, err := m.client.GetDocument(ctx, id)
		if err != nil {
			return errMsg{err}
		}
		if thenEdit {
			return m.buildEditor([]byte(doc.Body), doc.ID, doc.Type, doc.Path, doc.Title)
		}
		return docViewMsg{doc: doc}
	}
}

// loadBacklinks fetches backlinks for a document and emits a backlinksMsg.
func (m DocsModel) loadBacklinks(id string) tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		refs, err := m.client.Backlinks(ctx, id)
		if err != nil {
			return errMsg{err}
		}
		return backlinksMsg{refs: refs}
	}
}

// buildEditor opens an $EDITOR command seeded with initial; returns an editorReq
// msg (run via ExecProcess) or an errMsg.
func (m DocsModel) buildEditor(initial []byte, editID string, typ domain.DocumentType, path, title string) tea.Msg {
	if m.editor == nil {
		return errMsg{fmt.Errorf("no editor configured")}
	}
	cmd, readback, cleanup, err := m.editor.Command(initial)
	if err != nil {
		return errMsg{err}
	}
	return editorReq{cmd: cmd, readback: readback, cleanup: cleanup, editID: editID, typ: typ, path: path, title: title}
}

// runEditor wraps an editorReq into a tea.ExecProcess cmd: it suspends the TUI,
// runs $EDITOR, then reads the file back and emits an editorDoneMsg.
func runEditor(req editorReq) tea.Cmd {
	return tea.ExecProcess(req.cmd, func(err error) tea.Msg {
		defer req.cleanup()
		if err != nil {
			return editorDoneMsg{err: err}
		}
		body, err := req.readback()
		return editorDoneMsg{
			body:   body,
			editID: req.editID,
			typ:    req.typ,
			path:   req.path,
			title:  req.title,
			err:    err,
		}
	})
}

// persist creates or updates a document from edited body bytes.
func (m DocsModel) persist(d editorDoneMsg) tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if d.editID != "" {
			if _, err := m.client.UpdateDocument(ctx, d.editID, apiclient.UpdateDocumentInput{
				Title: d.title, Body: string(d.body),
			}); err != nil {
				return errMsg{err}
			}
			return docSavedMsg{}
		}
		if _, err := m.client.CreateDocument(ctx, apiclient.CreateDocumentInput{
			Type: string(d.typ), Path: d.path, Title: d.title, Body: string(d.body),
		}); err != nil {
			return errMsg{err}
		}
		return docSavedMsg{}
	}
}

func (m DocsModel) deleteCmd(id string) tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := m.client.DeleteDocument(ctx, id); err != nil {
			return errMsg{err}
		}
		return docSavedMsg{}
	}
}

func (m DocsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Standalone `flow docs` learns its size here and forwards it (overlay +
		// search-box width) via the same sink the `flow ui` route uses. In
		// `flow ui` the route re-sizes from its Frame after this, so its Frame
		// width wins for the chrome-bounded search box.
		return m.SetViewport(msg.Width, msg.Height), nil
	case docsLoadedMsg:
		m.docs = msg.docs
		if m.sel >= len(m.docs) {
			m.sel = max(0, len(m.docs)-1)
		}
		return m, nil
	case docViewMsg:
		d := msg.doc
		if _, start := domain.ParseFrontmatter(d.Body); start > 0 {
			d.Body = d.Body[start:]
		}
		// A same-id docViewMsg is an SSE live-reload of the doc the user is
		// already reading (a document.* event re-fired loadDocNoPush) — not a
		// navigation. Preserve the wikilink focus and the overlay scroll: just
		// refresh the source + render closure (so edited content and backlinks
		// show up) and Rerender in place. Only a genuine navigation (different
		// id, or no overlay yet) does the full reset (focus=-1, scroll 0).
		sameDoc := m.overlayReady && m.viewing != nil && m.viewing.ID == d.ID
		m.viewing = &d
		m.mode = modeView
		m.viewLinks = buildBodyLinks(d.Body, d, m.docs)
		if sameDoc {
			// Rebind the render closure to the refreshed doc (new title/tags/
			// body) against the EXISTING viewer cell so focus is preserved, and
			// push the new source — both via in-place setters that keep the
			// overlay scroll offset. backlinks refresh arrives via loadBacklinks.
			m.overlay = m.overlay.SetRender(m.buildRenderFunc(d, m.viewer))
			m.overlay = m.overlay.SetSource(d.Body)
			return m, m.loadBacklinks(d.ID)
		}
		m.linkFocus = -1
		m.backlinks = nil
		m.viewer = &viewerState{focus: -1}
		m.overlay = m.newViewerOverlay(d, m.viewer)
		m.overlayReady = true
		// Size the fresh overlay from the last known terminal size so standalone
		// `flow docs` renders the rich viewer immediately (the route's Frame
		// bridge re-sizes it every frame in `flow ui`).
		m = m.SetViewport(m.width, m.height)
		return m, m.loadBacklinks(d.ID)
	case backlinksMsg:
		m.backlinks = msg.refs
		// Rebind the render closure so the "Referenced by" footer appears. Use
		// SetRender (in place) rather than rebuilding the overlay so the scroll
		// offset and wikilink focus are preserved — important on a same-doc SSE
		// reload, where the user may already have scrolled/focused a link.
		if m.overlayReady && m.viewing != nil {
			m.overlay = m.overlay.SetRender(m.buildRenderFunc(*m.viewing, m.viewer))
		}
		seen := make(map[string]bool, len(m.viewLinks))
		for _, lt := range m.viewLinks {
			if lt.kind == linkWiki && lt.docID != "" {
				seen[lt.docID] = true
			}
		}
		for _, r := range msg.refs {
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			m.viewLinks = append(m.viewLinks, linkTarget{kind: linkWiki, docID: r.ID, label: r.Title})
		}
		return m, nil
	case tagsLoadedMsg:
		m.filterOpts = msg.tags
		m.filterWork = append([]string(nil), m.filterTags...)
		m.filterCursor = 0
		m.mode = modeFiltering
		return m, nil
	case searchDoneMsg:
		m.searchHits = msg.hits
		m.searching = true
		m.searchSel = 0
		return m, nil
	case eventsReadyMsg:
		m.events = msg.ch
		return m, waitForEvent(msg.ch)
	case eventMsg:
		// Any document.* event → reload list (or re-run active search) and re-arm.
		cmds := []tea.Cmd{waitForEvent(m.events)}
		if m.mode == modeSearch && m.searching && strings.TrimSpace(m.searchQuery) != "" {
			cmds = append(cmds, m.runSearch(m.searchQuery))
		} else {
			cmds = append(cmds, m.reload())
		}
		if m.mode == modeView && m.viewing != nil {
			cmds = append(cmds, m.loadDocNoPush(m.viewing.ID))
		}
		return m, tea.Batch(cmds...)
	case editorReq:
		return m, runEditor(msg)
	case editorDoneMsg:
		m.mode = modeList
		m.status = ""
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.status = "saving…"
		return m, m.persist(msg)
	case docSavedMsg:
		m.status = "✓ gespeichert"
		return m, m.reload()
	case errMsg:
		m.err = msg.err
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// CapturesInput reports that the docs screen owns the keyboard whenever it is
// in a sub-mode (creating / viewing / searching / filtering / deleting): every
// key then belongs to the model — New-Document field nav (Tab), link cycling
// (Tab/Shift+Tab in view), search/filter text entry, delete confirm — rather
// than the host shell's global Tab/digits/':' shortcuts. In the list it returns
// false so host navigation works. The docs route adapter exposes this as
// shell.InputCapturer.
func (m DocsModel) CapturesInput() bool { return m.mode != modeList }

func (m DocsModel) handleKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeCreating:
		return m.handleCreateKey(k)
	case modeDeleting:
		return m.handleDeleteKey(k)
	case modeFiltering:
		return m.handleFilterKey(k)
	case modeSearch:
		return m.handleSearchKey(k)
	case modeView:
		// While the overlay's in-document search input is active it owns every
		// key (typing the query, Enter/Esc inside search) — forward verbatim.
		if m.overlayReady && m.overlay.CapturesInput() {
			var cmd tea.Cmd
			m.overlay, cmd = m.overlay.Update(k)
			return m, cmd
		}
		switch {
		case k.Code == tea.KeyEsc:
			// Pop the in-TUI wikilink back-stack, or leave to the list.
			if n := len(m.viewStack); n > 0 {
				prev := m.viewStack[n-1]
				m.viewStack = m.viewStack[:n-1]
				return m, m.loadDocNoPush(prev)
			}
			m.viewing = nil
			m.mode = modeList
			m.viewLinks = nil
			m.linkFocus = -1
			m.overlayReady = false
			m.viewer = nil
			return m, nil
		case k.Code == tea.KeyTab && k.Mod == tea.ModShift:
			m.cycleWikiFocus(-1)
			return m, nil
		case k.Code == tea.KeyTab:
			m.cycleWikiFocus(+1)
			return m, nil
		case k.Code == tea.KeyEnter:
			return m.followFocusedWikilink()
		case k.Text == "e":
			if m.viewing == nil {
				return m, nil
			}
			return m, m.buildEditorCmd(m.viewing.ID)
		default:
			// Scroll / search-launch / code-copy belong to the overlay.
			var cmd tea.Cmd
			m.overlay, cmd = m.overlay.Update(k)
			return m, cmd
		}
	}
	// modeList
	switch {
	case k.Text == "q" || (k.Code == 'c' && k.Mod == tea.ModCtrl):
		return m, tea.Quit
	case k.Text == "j":
		if m.sel < len(m.docs)-1 {
			m.sel++
		}
		return m, nil
	case k.Text == "k":
		if m.sel > 0 {
			m.sel--
		}
		return m, nil
	case k.Code == tea.KeyEnter:
		if len(m.docs) == 0 {
			return m, nil
		}
		return m, m.loadDoc(m.docs[m.sel].ID, false)
	case k.Text == "n":
		m.mode = modeCreating
		m.field = fldType
		m.newType = domain.DocFree
		m.newPath = ""
		m.newTitle = ""
		m.err = nil
		return m, nil
	case k.Text == "e":
		if len(m.docs) == 0 {
			return m, nil
		}
		return m, m.buildEditorCmd(m.docs[m.sel].ID)
	case k.Text == "d":
		if len(m.docs) == 0 {
			return m, nil
		}
		m.mode = modeDeleting
		return m, nil
	case k.Text == "f":
		return m, m.loadTags()
	case k.Text == "/":
		m.mode = modeSearch
		m.searchQuery = ""
		m.searching = false
		m.searchHits = nil
		return m, nil
	}
	return m, nil
}

// buildEditorCmd loads the current body for id then opens $EDITOR on it.
func (m DocsModel) buildEditorCmd(id string) tea.Cmd {
	return m.loadDoc(id, true)
}

// validWikiTargets returns the resolvable wikilink target doc-ids in render
// order (one entry per valid [[link]] the renderer highlights). It drives off
// markdown.ValidWikilinkTargets — the SAME goldmark parse the renderer uses —
// so wikilinks inside code blocks/spans are excluded exactly as the renderer's
// validWikilinkIdx counting excludes them. This keeps cycleWikiFocus's cycle
// length and followFocusedWikilink's target index in lockstep with the
// highlight ordinal: Tab highlights link i ⇔ Enter follows ids[i].
func (m DocsModel) validWikiTargets() []string {
	var ids []string
	if m.viewing == nil {
		return ids
	}
	adapter := wikiAdapter{src: *m.viewing, all: m.docs}
	for _, target := range markdown.ValidWikilinkTargets(m.viewing.Body, adapter) {
		if d, ok := domain.ResolveWikilink(*m.viewing, target, m.docs); ok {
			ids = append(ids, d.ID)
		}
	}
	return ids
}

// cycleWikiFocus advances the focused wikilink by delta (wrapping) and rerenders
// the overlay so the highlight follows. No-op when there are no valid targets.
func (m *DocsModel) cycleWikiFocus(delta int) {
	if m.viewer == nil {
		return
	}
	n := len(m.validWikiTargets())
	if n == 0 {
		return
	}
	if m.viewer.focus < 0 {
		// First Tab focuses the first link (Shift+Tab the last).
		if delta > 0 {
			m.viewer.focus = 0
		} else {
			m.viewer.focus = n - 1
		}
	} else {
		m.viewer.focus = (m.viewer.focus + delta + n) % n
	}
	if m.overlayReady {
		m.overlay = m.overlay.Rerender()
	}
}

// followFocusedWikilink loads the focused wikilink target in-TUI, pushing the
// current doc onto the back-stack. No-op when nothing is focused.
func (m DocsModel) followFocusedWikilink() (tea.Model, tea.Cmd) {
	ids := m.validWikiTargets()
	if m.viewer == nil || m.viewer.focus < 0 || m.viewer.focus >= len(ids) {
		return m, nil
	}
	if m.viewing != nil {
		m.viewStack = append(m.viewStack, m.viewing.ID)
	}
	return m, m.loadDoc(ids[m.viewer.focus], false)
}

// loadDocNoPush loads a doc for the back-stack (Esc) without pushing again.
func (m DocsModel) loadDocNoPush(id string) tea.Cmd {
	return m.loadDoc(id, false)
}

var docTypeCycle = []domain.DocumentType{domain.DocFree, domain.DocDaily, domain.DocAgent}

func (m DocsModel) handleCreateKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		m.mode = modeList
		return m, nil
	case k.Code == tea.KeyTab:
		m.field = (m.field + 1) % 3
		return m, nil
	case k.Code == tea.KeyEnter:
		// On the last field, Enter launches $EDITOR for the body.
		if m.field < fldTitle {
			m.field++
			return m, nil
		}
		if !domain.SlugOK(m.newPath) {
			m.err = fmt.Errorf("invalid slug %q", m.newPath)
			return m, nil
		}
		if strings.TrimSpace(m.newTitle) == "" {
			m.err = fmt.Errorf("title required")
			return m, nil
		}
		m.err = nil
		return m, m.buildCreateEditorCmd()
	case m.field == fldType:
		// space/right cycles the type
		if k.Text == " " || k.Code == tea.KeyRight {
			m.newType = nextDocType(m.newType)
		}
		return m, nil
	case k.Code == tea.KeyBackspace:
		switch m.field {
		case fldPath:
			m.newPath = dropLast(m.newPath)
		case fldTitle:
			m.newTitle = dropLast(m.newTitle)
		}
		return m, nil
	case k.Text != "":
		switch m.field {
		case fldPath:
			m.newPath += k.Text
		case fldTitle:
			m.newTitle += k.Text
		}
		return m, nil
	}
	return m, nil
}

func (m DocsModel) buildCreateEditorCmd() tea.Cmd {
	typ, path, title := m.newType, m.newPath, m.newTitle
	return func() tea.Msg {
		seed := []byte("# " + title + "\n\n")
		return m.buildEditor(seed, "", typ, path, title)
	}
}

func (m DocsModel) handleDeleteKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case k.Text == "y":
		m.mode = modeList
		if len(m.docs) == 0 {
			return m, nil
		}
		return m, m.deleteCmd(m.docs[m.sel].ID)
	case k.Code == tea.KeyEsc || k.Text == "n":
		m.mode = modeList
		return m, nil
	}
	return m, nil
}

func (m DocsModel) handleFilterKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		m.mode = modeList // discard working changes
		return m, nil
	case k.Text == "j":
		if m.filterCursor < len(m.filterOpts)-1 {
			m.filterCursor++
		}
		return m, nil
	case k.Text == "k":
		if m.filterCursor > 0 {
			m.filterCursor--
		}
		return m, nil
	case k.Text == " ":
		if m.filterCursor < len(m.filterOpts) {
			m.filterWork = toggleStr(m.filterWork, m.filterOpts[m.filterCursor].Tag)
		}
		return m, nil
	case k.Text == "c":
		m.filterWork = nil
		return m, nil
	case k.Code == tea.KeyEnter:
		m.filterTags = append([]string(nil), m.filterWork...)
		m.mode = modeList
		m.sel = 0
		return m, m.reload()
	}
	return m, nil
}

func (m DocsModel) handleSearchKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEsc:
		m.mode = modeList
		m.searching = false
		m.searchHits = nil
		return m, nil
	case k.Code == tea.KeyEnter:
		if !m.searching {
			if strings.TrimSpace(m.searchQuery) == "" {
				return m, nil
			}
			return m, m.runSearch(m.searchQuery)
		}
		if len(m.searchHits) == 0 {
			return m, nil
		}
		return m, m.loadDoc(m.searchHits[m.searchSel].ID, false)
	case m.searching && k.Text == "j":
		if m.searchSel < len(m.searchHits)-1 {
			m.searchSel++
		}
		return m, nil
	case m.searching && k.Text == "k":
		if m.searchSel > 0 {
			m.searchSel--
		}
		return m, nil
	case !m.searching && k.Code == tea.KeyBackspace:
		m.searchQuery = dropLast(m.searchQuery)
		return m, nil
	case !m.searching && k.Text != "":
		m.searchQuery += k.Text
		return m, nil
	}
	return m, nil
}

// toggleStr adds s to xs if absent, removes it if present.
func toggleStr(xs []string, s string) []string {
	for i, x := range xs {
		if x == s {
			return append(xs[:i:i], xs[i+1:]...)
		}
	}
	return append(xs, s)
}

// containsStr reports whether xs contains s.
func containsStr(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func nextDocType(t domain.DocumentType) domain.DocumentType {
	for i, c := range docTypeCycle {
		if c == t {
			return docTypeCycle[(i+1)%len(docTypeCycle)]
		}
	}
	return docTypeCycle[0]
}

func dropLast(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return string(r[:len(r)-1])
}

// linkKind distinguishes an in-TUI wikilink jump from an external weblink.
type linkKind int

const (
	linkWiki linkKind = iota
	linkWeb
)

// linkTarget is one focusable link in the current view.
type linkTarget struct {
	kind  linkKind
	docID string // wikilink/backlink target document id
	url   string // weblink url
	label string
}

// buildBodyLinks returns the focusable links found in body, in reading order:
// resolved wikilinks (deduped per target doc) and weblinks. Broken wikilinks
// are not focusable.
func buildBodyLinks(body string, src domain.Document, all []domain.Document) []linkTarget {
	type pos struct {
		start int
		lt    linkTarget
	}
	var found []pos
	seenDoc := map[string]bool{}

	for _, sp := range domain.FindWikilinks(body) {
		if resolved, ok := domain.ResolveWikilink(src, sp.Target, all); ok {
			if seenDoc[resolved.ID] {
				continue
			}
			seenDoc[resolved.ID] = true
			label := sp.Display
			if label == "" {
				label = resolved.Title
			}
			if label == "" {
				label = sp.Target
			}
			found = append(found, pos{sp.Start, linkTarget{kind: linkWiki, docID: resolved.ID, label: label}})
		}
	}
	for _, ws := range findWeblinks(body) {
		found = append(found, pos{ws.Start, linkTarget{kind: linkWeb, url: ws.URL, label: ws.Display}})
	}
	for i := 0; i < len(found); i++ {
		for j := i + 1; j < len(found); j++ {
			if found[j].start < found[i].start {
				found[i], found[j] = found[j], found[i]
			}
		}
	}
	out := make([]linkTarget, 0, len(found))
	for _, p := range found {
		out = append(out, p.lt)
	}
	return out
}

// styleBodyLine renders one body line with styled wikilink + weblink segments.
// focusIdx / focusOf are used by Task 13 to highlight the focused link; here
// they are accepted but the highlight is wired later.
func styleBodyLine(line string, src domain.Document, all []domain.Document, focusIdx int, focusOf func(target string) int) string {
	type seg struct {
		start, end int
		text       string
	}
	var segs []seg

	for _, sp := range domain.FindWikilinks(line) {
		resolved, ok := domain.ResolveWikilink(src, sp.Target, all)
		label := sp.Display
		if label == "" && ok {
			label = resolved.Title
		}
		if label == "" {
			label = sp.Target
		}
		var styled string
		if ok {
			styled = styleWikiValid.Render("→ " + label)
		} else {
			styled = styleWikiBroken.Render("⊘ " + label)
		}
		segs = append(segs, seg{sp.Start, sp.End, styled})
	}
	for _, ws := range findWeblinks(line) {
		styled := osc8(ws.URL, styleWebLink.Render(ws.Display))
		segs = append(segs, seg{ws.Start, ws.End, styled})
	}
	if len(segs) == 0 {
		return line
	}
	for i := 0; i < len(segs); i++ {
		for j := i + 1; j < len(segs); j++ {
			if segs[j].start < segs[i].start {
				segs[i], segs[j] = segs[j], segs[i]
			}
		}
	}
	var b strings.Builder
	prev := 0
	for _, sg := range segs {
		if sg.start < prev {
			continue // overlap guard
		}
		b.WriteString(line[prev:sg.start])
		b.WriteString(sg.text)
		prev = sg.end
	}
	b.WriteString(line[prev:])
	return b.String()
}

// osc8 wraps text in an OSC 8 hyperlink so terminals that support it open the
// URL on click. Harmless where unsupported.
func osc8(url, text string) string {
	return "\x1b]8;;" + url + "\x07" + text + "\x1b]8;;\x07"
}

func (m DocsModel) View() tea.View {
	var b strings.Builder
	b.WriteString(styleHeader.Render("flow · docs") + styleMuted.Render("  "+m.user) + "\n\n")

	switch m.mode {
	case modeView:
		// Prefer the fullscreen markdown overlay; fall back to the legacy
		// line renderer when the overlay has no size yet (standalone
		// `flow docs` before its first WindowSizeMsg, or a screen too small
		// for chrome) so the body is never blank.
		if ov := m.overlayView(); ov != "" {
			b.WriteString(ov)
		} else {
			m.renderView(&b)
		}
	case modeCreating:
		m.renderCreate(&b)
	case modeDeleting:
		m.renderList(&b)
		b.WriteString("\n" + styleWarn.Render("  delete this document? y / n") + "\n")
	case modeFiltering:
		m.renderFilter(&b)
	case modeSearch:
		m.renderSearch(&b)
	default:
		m.renderList(&b)
	}

	if m.mode == modeView && !m.overlayReady && m.linkFocus >= 0 && m.linkFocus < len(m.viewLinks) {
		lt := m.viewLinks[m.linkFocus]
		tgt := lt.label
		if lt.kind == linkWeb {
			tgt = lt.url
		}
		b.WriteString("\n" + styleLinkFocus.Render(" ▸ "+tgt+" ") + styleMuted.Render("  enter to follow") + "\n")
	}

	b.WriteString("\n")
	if m.status != "" {
		b.WriteString(styleOk.Render("  "+m.status) + "\n")
	}
	if m.err != nil {
		b.WriteString(styleErr.Render("error: "+m.err.Error()) + "\n")
	}
	b.WriteString(styleMuted.Render(m.footer()) + "\n")

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// overlayView renders the fullscreen viewer body, or "" when the overlay is not
// ready or has no usable size (callers fall back to the legacy renderer).
func (m DocsModel) overlayView() string {
	if !m.overlayReady {
		return ""
	}
	return m.overlay.View()
}

// SetViewport records the host size and sizes the in-view overlay. It is the
// single sizing sink: in `flow ui` the docs route adapter calls it from View(f)
// every frame (the Frame→SetSize bridge — so width tracks the chrome-bounded
// content area, which also bounds the search box); in standalone `flow docs`
// the WindowSizeMsg handler calls it with the full terminal size.
func (m DocsModel) SetViewport(w, h int) DocsModel {
	m.width, m.height = w, h
	if m.overlayReady {
		m.overlay = m.overlay.SetSize(w, h)
	}
	return m
}

func (m DocsModel) footer() string {
	switch m.mode {
	case modeView:
		return "tab/⇧tab link · enter folgen/öffnen · e edit · esc zurück · q quit"
	case modeCreating:
		return "tab next · space type · enter next/open editor · esc cancel"
	case modeDeleting:
		return "y confirm · n/esc cancel"
	case modeFiltering:
		return "j/k move · space toggle · c clear · enter apply · esc cancel"
	case modeSearch:
		return "query eingeben · enter suchen · esc abbrechen"
	default:
		return "j/k move · enter view · n new · e edit · d delete · f filter · / suchen · q quit"
	}
}

// tagSuffix builds a "#tag1 #tag2" string for display.
func tagSuffix(tags []string) string {
	parts := make([]string, len(tags))
	for i, t := range tags {
		parts[i] = "#" + t
	}
	return strings.Join(parts, " ")
}

func (m DocsModel) renderList(b *strings.Builder) {
	b.WriteString(styleHeader.Render("Documents") + "\n")
	if len(m.filterTags) > 0 {
		b.WriteString(styleMuted.Render("  filter: "+tagSuffix(m.filterTags)) + "\n")
	}
	if len(m.docs) == 0 {
		b.WriteString(styleMuted.Render("  no documents yet — press n to create one") + "\n")
		return
	}
	for i, d := range m.docs {
		tags := ""
		if len(d.Tags) > 0 {
			tags = styleMuted.Render("  " + tagSuffix(d.Tags))
		}
		if i == m.sel {
			line := styleSel.Render(fmt.Sprintf("▸ %-7s %s  %s", d.Type, d.Path, d.Title)) + tags
			b.WriteString(line + "\n")
		} else {
			line := fmt.Sprintf("  %-7s %s  %s", d.Type, d.Path, d.Title) + tags
			b.WriteString(line + "\n")
		}
	}
}

func (m DocsModel) renderView(b *strings.Builder) {
	if m.viewing == nil {
		b.WriteString(styleMuted.Render("  (nothing to show)") + "\n")
		return
	}
	d := m.viewing
	hdr := styleHeader.Render(d.Title) + styleMuted.Render("  "+string(d.Type)+" · "+d.Path)
	if len(d.Tags) > 0 {
		hdr += styleMuted.Render("  " + tagSuffix(d.Tags))
	}
	b.WriteString(hdr + "\n\n")
	body := d.Body
	if _, start := domain.ParseFrontmatter(body); start > 0 {
		body = body[start:]
	}
	if strings.TrimSpace(body) == "" {
		b.WriteString(styleMuted.Render("  (empty)") + "\n")
		return
	}
	for _, ln := range strings.Split(body, "\n") {
		b.WriteString("  " + styleBodyLine(ln, *d, m.docs, m.linkFocus, func(string) int { return -1 }) + "\n")
	}
	m.renderBacklinks(b)
}

func (m DocsModel) renderBacklinks(b *strings.Builder) {
	if len(m.backlinks) == 0 {
		return
	}
	b.WriteString("\n" + styleMuted.Render("  ↩ Referenced by") + "\n")
	for _, r := range m.backlinks {
		label := r.Title
		line := "  " + styleWikiValid.Render("→ "+label)
		if label == "" {
			line = "  " + styleWikiValid.Render("→ "+r.Path)
		} else {
			line += styleMuted.Render("  " + r.Path)
		}
		b.WriteString(line + "\n")
	}
}

func (m DocsModel) renderFilter(b *strings.Builder) {
	b.WriteString(styleHeader.Render("Filter by tag") + "\n")
	if len(m.filterOpts) == 0 {
		b.WriteString(styleMuted.Render("  no tags yet") + "\n")
		return
	}
	for i, tc := range m.filterOpts {
		mark := "  [ ] "
		if containsStr(m.filterWork, tc.Tag) {
			mark = "  [x] "
		}
		line := fmt.Sprintf("%s#%s (%d)", mark, tc.Tag, tc.Count)
		if i == m.filterCursor {
			line = styleSel.Render("▸ " + strings.TrimLeft(line, " "))
		}
		b.WriteString(line + "\n")
	}
}

func (m DocsModel) renderSearch(b *strings.Builder) {
	if !m.searching {
		b.WriteString(styleHeader.Render("Suchen") + styleMuted.Render("  /") + m.searchQuery + "▏" + "\n")
		return
	}
	if len(m.searchHits) == 0 {
		b.WriteString(styleHeader.Render("Suchen") + styleMuted.Render("  /"+m.searchQuery) + "\n")
		b.WriteString(styleMuted.Render("  Keine Treffer.") + "\n")
		return
	}

	bw := m.width
	if bw > 80 {
		bw = 80
	}
	if bw < 24 {
		bw = 72 // standalone fallback before the first WindowSizeMsg
	}
	inner := bw - 2    // titlebox interior, between the │ pipes
	snipW := inner - 4 // snippet column: 1 lead space + 3-space hanging indent
	if snipW < 8 {
		snipW = 8
	}

	var body strings.Builder
	for i, h := range m.searchHits {
		marker := "  "
		title := h.Title
		if i == m.searchSel {
			marker = glyphs.AccentBar + " "
			title = lipgloss.NewStyle().Foreground(colAccent).Bold(true).Render(h.Title)
		}
		body.WriteString(" " + marker + title + styleMuted.Render("  "+h.Path) + "\n")
		for _, ln := range wrapSnippet(h.Snippet, snipW, 2) {
			body.WriteString("   " + ln + "\n")
		}
		if i < len(m.searchHits)-1 {
			body.WriteString("\n")
		}
	}
	box := titlebox.Render("Suchen · /"+m.searchQuery, strings.TrimRight(body.String(), "\n"), bw, theme.Default)
	b.WriteString(box + "\n")
}

// cleanSnippet collapses a search snippet's whitespace and newlines into a
// single line and drops the markdown punctuation that would otherwise leak into
// the preview (heading #, list/quote markers, code backticks). The highlight
// sentinels are preserved so the matched term can still be emphasised.
func cleanSnippet(s string) string {
	s = strings.ReplaceAll(s, "`", "")
	fields := strings.Fields(s) // collapses runs of whitespace incl. newlines
	out := fields[:0]
	for _, f := range fields {
		switch f {
		case "#", "##", "###", "####", "#####", "-", "*", ">", "•":
			continue // standalone markdown marker — not useful in a preview
		}
		out = append(out, f)
	}
	return strings.Join(out, " ")
}

// snippetVisibleWidth measures a snippet word's on-screen width, ignoring the
// highlight sentinels (zero-width control bytes that become ANSI on render).
func snippetVisibleWidth(s string) int {
	s = strings.ReplaceAll(s, domain.HighlightStart, "")
	s = strings.ReplaceAll(s, domain.HighlightEnd, "")
	return lipgloss.Width(s)
}

// wrapSnippet cleans a snippet and word-wraps it to at most maxLines lines of
// the given visible width, returning each line already highlight-rendered. A
// trailing "…" on the last line marks content that did not fit.
func wrapSnippet(snippet string, width, maxLines int) []string {
	cleaned := cleanSnippet(snippet)
	if cleaned == "" {
		return nil
	}
	var raw []string
	var cur strings.Builder
	curW := 0
	truncated := false
	for _, word := range strings.Fields(cleaned) {
		ww := snippetVisibleWidth(word)
		if curW > 0 && curW+1+ww > width {
			raw = append(raw, cur.String())
			cur.Reset()
			curW = 0
			if len(raw) == maxLines {
				truncated = true
				break
			}
		}
		if curW > 0 {
			cur.WriteByte(' ')
			curW++
		}
		cur.WriteString(word)
		curW += ww
	}
	if !truncated && cur.Len() > 0 {
		raw = append(raw, cur.String())
	}
	if truncated && len(raw) > 0 {
		raw[len(raw)-1] = strings.TrimRight(raw[len(raw)-1], " ") + " …"
	}
	out := make([]string, len(raw))
	for i, ln := range raw {
		out[i] = highlightSnippet(ln)
	}
	return out
}

// highlightSnippet replaces the shared sentinels with a lipgloss highlight.
func highlightSnippet(s string) string {
	var out strings.Builder
	for {
		i := strings.Index(s, domain.HighlightStart)
		if i < 0 {
			out.WriteString(styleMuted.Render(s))
			break
		}
		j := strings.Index(s, domain.HighlightEnd)
		if j < 0 || j < i {
			// Stray/unmatched sentinel — strip it so no raw control char reaches
			// the terminal (belt-and-suspenders: write boundary already strips them).
			safe := strings.ReplaceAll(s, domain.HighlightStart, "")
			safe = strings.ReplaceAll(safe, domain.HighlightEnd, "")
			out.WriteString(styleMuted.Render(safe))
			break
		}
		out.WriteString(styleMuted.Render(s[:i]))
		out.WriteString(styleSearchHit.Render(s[i+len(domain.HighlightStart) : j]))
		s = s[j+len(domain.HighlightEnd):]
	}
	return out.String()
}

func (m DocsModel) renderCreate(b *strings.Builder) {
	b.WriteString(styleHeader.Render("New document") + "\n")
	b.WriteString(fieldLine("type ", string(m.newType), m.field == fldType) + "\n")
	b.WriteString(fieldLine("slug ", m.newPath, m.field == fldPath) + "\n")
	b.WriteString(fieldLine("title", m.newTitle, m.field == fldTitle) + "\n")
}

func fieldLine(label, val string, active bool) string {
	caret := ""
	if active {
		caret = "▏"
	}
	prefix := "  "
	if active {
		prefix = styleSel.Render("▸ ")
	}
	return prefix + styleMuted.Render(label+": ") + val + caret
}
