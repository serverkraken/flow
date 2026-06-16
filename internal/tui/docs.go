package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
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
		m.viewing = &d
		m.mode = modeView
		m.viewLinks = buildBodyLinks(d.Body, d, m.docs)
		m.linkFocus = -1
		m.backlinks = nil
		return m, m.loadBacklinks(d.ID)
	case backlinksMsg:
		m.backlinks = msg.refs
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
		switch {
		case k.Code == tea.KeyEsc:
			if n := len(m.viewStack); n > 0 {
				prev := m.viewStack[n-1]
				m.viewStack = m.viewStack[:n-1]
				return m, m.loadDocNoPush(prev)
			}
			m.viewing = nil
			m.mode = modeList
			m.viewLinks = nil
			m.linkFocus = -1
			return m, nil
		case k.Text == "q" || (k.Code == 'c' && k.Mod == tea.ModCtrl):
			return m, tea.Quit
		case k.Code == tea.KeyTab && k.Mod == tea.ModShift:
			m.linkFocus = cycleLink(m.linkFocus, len(m.viewLinks), -1)
			return m, nil
		case k.Code == tea.KeyTab:
			m.linkFocus = cycleLink(m.linkFocus, len(m.viewLinks), +1)
			return m, nil
		case k.Code == tea.KeyEnter:
			return m.followFocusedLink()
		case k.Text == "e":
			if m.viewing == nil {
				return m, nil
			}
			return m, m.buildEditorCmd(m.viewing.ID)
		}
		return m, nil
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

// cycleLink advances idx within [0,n) by delta, wrapping; -1/empty stays -1.
func cycleLink(idx, n, delta int) int {
	if n == 0 {
		return -1
	}
	if idx < 0 {
		if delta > 0 {
			return 0
		}
		return n - 1
	}
	return (idx + delta + n) % n
}

// followFocusedLink acts on the focused link: load a wikilink target in-TUI
// (pushing the current doc onto the back-stack) or open a weblink externally.
func (m DocsModel) followFocusedLink() (tea.Model, tea.Cmd) {
	if m.linkFocus < 0 || m.linkFocus >= len(m.viewLinks) {
		return m, nil
	}
	lt := m.viewLinks[m.linkFocus]
	switch lt.kind {
	case linkWeb:
		url := lt.url
		op := m.opener
		return m, func() tea.Msg {
			if op != nil {
				_ = op.Open(url)
			}
			return nil
		}
	case linkWiki:
		if m.viewing != nil {
			m.viewStack = append(m.viewStack, m.viewing.ID)
		}
		return m, m.loadDoc(lt.docID, false)
	}
	return m, nil
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
		m.renderView(&b)
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

	if m.mode == modeView && m.linkFocus >= 0 && m.linkFocus < len(m.viewLinks) {
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
	b.WriteString(styleHeader.Render("Suchen") + styleMuted.Render("  /"+m.searchQuery) + "\n")
	if len(m.searchHits) == 0 {
		b.WriteString(styleMuted.Render("  Keine Treffer.") + "\n")
		return
	}
	for i, h := range m.searchHits {
		var title string
		if i == m.searchSel {
			title = styleSel.Render("▸ " + h.Title)
		} else {
			title = "  " + h.Title
		}
		b.WriteString(title + styleMuted.Render("  "+h.Path) + "\n")
		b.WriteString("    " + highlightSnippet(h.Snippet) + "\n")
	}
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
