package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

func sampleDocs() []domain.Document {
	return []domain.Document{
		{ID: "d1", Type: domain.DocFree, Path: "docs/architecture", Title: "Arch", Body: "the body text"},
		{ID: "d2", Type: domain.DocAgent, Path: "agents/reviewer", Title: "Reviewer"},
	}
}

func TestDocsLoadedPopulatesAndRenders(t *testing.T) {
	m := NewDocs(nil, nil, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: sampleDocs()})
	m = next.(DocsModel)
	if len(m.docs) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(m.docs))
	}
	out := m.View().Content
	if !strings.Contains(out, "docs/architecture") || !strings.Contains(out, "agents/reviewer") {
		t.Fatalf("list view missing paths:\n%s", out)
	}
}

func TestDocsJKNavigation(t *testing.T) {
	m := NewDocs(nil, nil, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: sampleDocs()})
	m = next.(DocsModel)

	next2, _ := m.Update(tea.KeyPressMsg{Text: "j"})
	m = next2.(DocsModel)
	if m.sel != 1 {
		t.Fatalf("j: sel = %d, want 1", m.sel)
	}
	// clamp at end
	next3, _ := m.Update(tea.KeyPressMsg{Text: "j"})
	m = next3.(DocsModel)
	if m.sel != 1 {
		t.Fatalf("j clamp: sel = %d, want 1", m.sel)
	}
	next4, _ := m.Update(tea.KeyPressMsg{Text: "k"})
	m = next4.(DocsModel)
	if m.sel != 0 {
		t.Fatalf("k: sel = %d, want 0", m.sel)
	}
}

func TestDocsEnterShowsBody(t *testing.T) {
	m := NewDocs(nil, nil, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: sampleDocs()})
	m = next.(DocsModel)

	// Enter with nil client returns no cmd; simulate the resulting docViewMsg.
	next2, _ := m.Update(docViewMsg{doc: sampleDocs()[0]})
	m = next2.(DocsModel)
	if m.mode != modeView || m.viewing == nil {
		t.Fatal("docViewMsg should switch to view mode with a viewing doc")
	}
	out := m.View().Content
	if !strings.Contains(out, "the body text") {
		t.Fatalf("view mode missing body:\n%s", out)
	}
}

func TestDocsEscReturnsToList(t *testing.T) {
	m := NewDocs(nil, nil, "tester")
	next, _ := m.Update(docViewMsg{doc: sampleDocs()[0]})
	m = next.(DocsModel)
	next2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = next2.(DocsModel)
	if m.mode != modeList || m.viewing != nil {
		t.Fatal("esc should return to list and clear viewing")
	}
}

func TestDocsEventTriggersReload(t *testing.T) {
	// non-nil client so reload() returns a real cmd.
	c := apiclient.New("http://example.invalid", "tok")
	m := NewDocs(c, nil, "tester")
	// arm events channel
	ch := make(chan apiclient.ClientEvent, 1)
	next, _ := m.Update(eventsReadyMsg{ch: ch})
	m = next.(DocsModel)

	_, cmd := m.Update(eventMsg{ev: apiclient.ClientEvent{Type: string(domain.EventDocumentCreated)}})
	if cmd == nil {
		t.Fatal("document event should return a reload+listen batch cmd")
	}
}

func TestDocsNKeyOpensCreate(t *testing.T) {
	m := NewDocs(nil, nil, "tester")
	next, _ := m.Update(tea.KeyPressMsg{Text: "n"})
	m = next.(DocsModel)
	if m.mode != modeCreating {
		t.Fatal("n should open create mode")
	}
	out := m.View().Content
	if !strings.Contains(out, "New document") {
		t.Fatalf("create view missing header:\n%s", out)
	}
}

func TestDocsCreateFormTypingAndCancel(t *testing.T) {
	m := NewDocs(nil, nil, "tester")
	next, _ := m.Update(tea.KeyPressMsg{Text: "n"})
	m = next.(DocsModel)
	// advance to slug field
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(DocsModel)
	if m.field != fldPath {
		t.Fatalf("tab: field = %d, want %d", m.field, fldPath)
	}
	for _, r := range "docs/x" {
		next, _ = m.Update(tea.KeyPressMsg{Text: string(r)})
		m = next.(DocsModel)
	}
	if m.newPath != "docs/x" {
		t.Fatalf("slug = %q, want docs/x", m.newPath)
	}
	// esc cancels back to list
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = next.(DocsModel)
	if m.mode != modeList {
		t.Fatal("esc should cancel create")
	}
}

func TestDocsDeleteConfirmFlow(t *testing.T) {
	m := NewDocs(nil, nil, "tester")
	next, _ := m.Update(docsLoadedMsg{docs: sampleDocs()})
	m = next.(DocsModel)
	next, _ = m.Update(tea.KeyPressMsg{Text: "d"})
	m = next.(DocsModel)
	if m.mode != modeDeleting {
		t.Fatal("d should enter delete-confirm mode")
	}
	// n cancels
	next, _ = m.Update(tea.KeyPressMsg{Text: "n"})
	m = next.(DocsModel)
	if m.mode != modeList {
		t.Fatal("n should cancel delete")
	}
}

func TestDocsQuitKey(t *testing.T) {
	m := NewDocs(nil, nil, "tester")
	_, cmd := m.Update(tea.KeyPressMsg{Text: "q"})
	if cmd == nil {
		t.Fatal("q should return a quit command")
	}
}

func TestDocsNextDocType(t *testing.T) {
	if got := nextDocType(domain.DocFree); got != domain.DocDaily {
		t.Fatalf("free → %q, want daily", got)
	}
	if got := nextDocType(domain.DocAgent); got != domain.DocFree {
		t.Fatalf("agent → %q (wrap), want free", got)
	}
}

func TestDocsSavedReloads(t *testing.T) {
	c := apiclient.New("http://example.invalid", "tok")
	m := NewDocs(c, nil, "tester")
	_, cmd := m.Update(docSavedMsg{})
	if cmd == nil {
		t.Fatal("docSavedMsg should trigger a reload cmd")
	}
}
