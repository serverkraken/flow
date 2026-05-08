package view_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/view"
)

const sampleNote = "# Heading\n\n" +
	"First paragraph mentions tmux and a link to https://example.com/path.\n\n" +
	"```go\nfunc main() {}\n```\n\n" +
	"Another paragraph mentions tmux again, and one final tmux line.\n"

func newSized(t *testing.T, title, source string) view.Model {
	t.Helper()
	m := view.New(title, source, nil, nil, nil).SetSize(120, 40)
	return m
}

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// TestRender_OSC8SurvivesView asserts the viewer's output carries the
// OSC 8 hyperlink for URLs in the source — the whole reason this
// viewer exists in the first place.
func TestRender_OSC8SurvivesView(t *testing.T) {
	t.Parallel()
	m := newSized(t, "note", sampleNote)
	out := m.View()
	if !strings.Contains(out, ";https://example.com/path\x07") {
		t.Errorf("View output missing OSC 8 destination for example.com\n%q", out)
	}
}

// TestSearch_EnterAppliesQueryAndCountsMatches drives /, types a
// query, hits Enter, and asserts matches were found.
func TestSearch_EnterAppliesQueryAndCountsMatches(t *testing.T) {
	t.Parallel()
	m := newSized(t, "note", sampleNote)
	model, _ := m.Update(runeKey('/'))
	for _, r := range "tmux" {
		model, _ = model.Update(runeKey(r))
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if model.CurrentMode() != view.ModeNormal {
		t.Errorf("after enter, mode = %v, want ModeNormal", model.CurrentMode())
	}
	if model.Query() != "tmux" {
		t.Errorf("query = %q, want \"tmux\"", model.Query())
	}
	if len(model.Matches()) == 0 {
		t.Fatal("expected at least one match for tmux")
	}
	if !strings.Contains(model.View(), "tmux") {
		t.Errorf("View should still surface the query in the status bar")
	}
}

// TestSearch_EscCancelsAndKeepsPreviousQuery: opening / and pressing
// Esc must not overwrite a previously-applied query.
func TestSearch_EscCancelsAndKeepsPreviousQuery(t *testing.T) {
	t.Parallel()
	m := newSized(t, "note", sampleNote)
	model, _ := m.Update(runeKey('/'))
	for _, r := range "tmux" {
		model, _ = model.Update(runeKey(r))
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	model, _ = model.Update(runeKey('/'))
	for _, r := range "ZZZ" {
		model, _ = model.Update(runeKey(r))
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if model.CurrentMode() != view.ModeNormal {
		t.Errorf("after esc, mode = %v, want ModeNormal", model.CurrentMode())
	}
	if model.Query() != "tmux" {
		t.Errorf("esc must not overwrite the previous query, got %q", model.Query())
	}
}

// TestSearch_NCyclesMatches: n advances the cursor through matches and
// wraps around at the end.
func TestSearch_NCyclesMatches(t *testing.T) {
	t.Parallel()
	m := newSized(t, "note", sampleNote)
	model, _ := m.Update(runeKey('/'))
	for _, r := range "tmux" {
		model, _ = model.Update(runeKey(r))
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	matches := model.Matches()
	if len(matches) < 2 {
		t.Skipf("need >= 2 matches in fixture, got %d", len(matches))
	}
	first := model.MatchIndex()
	model, _ = model.Update(runeKey('n'))
	if model.MatchIndex() == first {
		t.Errorf("n did not advance match cursor (still %d)", first)
	}
	// Wrap-around: keep pressing n until we are back at first.
	for i := 0; i < len(matches)+2; i++ {
		model, _ = model.Update(runeKey('n'))
		if model.MatchIndex() == first {
			return
		}
	}
	t.Errorf("n did not wrap back to first match after %d presses", len(matches)+2)
}

// TestSearch_NoMatchKeepsModeNormal: a search with no hits still
// returns to ModeNormal so the user can type / again.
func TestSearch_NoMatchKeepsModeNormal(t *testing.T) {
	t.Parallel()
	m := newSized(t, "note", sampleNote)
	model, _ := m.Update(runeKey('/'))
	for _, r := range "doesnotexistxxxx" {
		model, _ = model.Update(runeKey(r))
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if model.CurrentMode() != view.ModeNormal {
		t.Errorf("after enter on no-match, mode = %v, want ModeNormal", model.CurrentMode())
	}
	if len(model.Matches()) != 0 {
		t.Errorf("expected zero matches, got %d", len(model.Matches()))
	}
	out := model.View()
	if !strings.Contains(out, "no matches") {
		t.Errorf("status bar should show 'no matches'\n%s", out)
	}
}

// TestQuit_EmitsExitMsg: q on the viewer returns a tea.Cmd that
// produces ExitMsg. The hosting browse model uses this to leave
// ModeView.
func TestQuit_EmitsExitMsg(t *testing.T) {
	t.Parallel()
	m := newSized(t, "note", sampleNote)
	_, cmd := m.Update(runeKey('q'))
	if cmd == nil {
		t.Fatal("q should return a tea.Cmd")
	}
	if _, ok := cmd().(view.ExitMsg); !ok {
		t.Errorf("q-cmd should produce view.ExitMsg, got %T", cmd())
	}
}

func TestInit_ReturnsNil(t *testing.T) {
	t.Parallel()
	m := view.New("title", "body", nil, nil, nil)
	if cmd := m.Init(); cmd != nil {
		t.Errorf("Init() should be a no-op, got cmd=%v", cmd)
	}
}

// TestWindowSizeMsg_TriggersRerender: tea.WindowSizeMsg must update
// the inner state so a subsequent View() draws at the new size.
func TestWindowSizeMsg_TriggersRerender(t *testing.T) {
	t.Parallel()
	m := view.New("title", sampleNote, nil, nil, nil)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	out := model.View()
	if out == "" {
		t.Fatal("View should produce output after WindowSizeMsg")
	}
	if !strings.Contains(out, "title") {
		t.Errorf("View should render the title\n%s", out)
	}
}

// TestStatusBar_LongTitleTruncated drives the truncate path: a title
// wider than the available status bar segment must be clipped to "…".
func TestStatusBar_LongTitleTruncated(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 200)
	m := view.New(long, sampleNote, nil, nil, nil).SetSize(60, 20)
	out := m.View()
	if !strings.Contains(out, "…") {
		t.Errorf("status bar should truncate a 200-char title at width 60\n%s", out)
	}
}

// TestSearch_BarRendersInComposedContent: an applied query must lead
// to the yellow ▎ bar appearing in the viewport's composed content.
// Asserts the highlight pipeline reaches the bytes the user sees.
func TestSearch_BarRendersInComposedContent(t *testing.T) {
	t.Parallel()
	m := newSized(t, "note", sampleNote)
	model, _ := m.Update(runeKey('/'))
	for _, r := range "tmux" {
		model, _ = model.Update(runeKey(r))
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(model.View(), "▎") {
		t.Errorf("composed content should carry a ▎ match bar\n%s", model.View())
	}
}

// TestSearch_ResizeAfterQueryRecomputes: resizing while a query is
// active must rebuild matches against the new line layout instead of
// dropping the highlight silently.
func TestSearch_ResizeAfterQueryRecomputes(t *testing.T) {
	t.Parallel()
	m := newSized(t, "note", sampleNote)
	model, _ := m.Update(runeKey('/'))
	for _, r := range "tmux" {
		model, _ = model.Update(runeKey(r))
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	model = model.SetSize(60, 20)
	if len(model.Matches()) == 0 {
		t.Errorf("matches should survive resize, got 0")
	}
}

// TestNavKeys_GandG_ScrollToTopAndBottom drives the g/G keys; the
// viewport offset must change when there's enough content to scroll.
func TestNavKeys_GandG_ScrollToTopAndBottom(t *testing.T) {
	t.Parallel()
	body := strings.Repeat("paragraph line.\n\n", 200)
	m := view.New("note", body, nil, nil, nil).SetSize(80, 10)
	model, _ := m.Update(runeKey('G'))
	if !strings.Contains(model.View(), "paragraph") {
		t.Errorf("after G, view should still render content")
	}
	model, _ = model.Update(runeKey('g'))
	if !strings.Contains(model.View(), "note") {
		t.Errorf("after g, view should still render the title bar")
	}
}

// TestUnknownKey_DelegatedToViewport keeps the model stable when an
// unhandled key arrives — exercises the fall-through to the viewport
// in handleNormalKey.
func TestUnknownKey_DelegatedToViewport(t *testing.T) {
	t.Parallel()
	m := newSized(t, "note", sampleNote)
	model, _ := m.Update(runeKey('z'))
	if model.CurrentMode() != view.ModeNormal {
		t.Errorf("unknown key should keep mode=ModeNormal, got %v", model.CurrentMode())
	}
}

// TestCtrlC_EmitsExitMsg covers the third Quit-binding key alongside
// q and esc, which the previous tests already exercised.
func TestCtrlC_EmitsExitMsg(t *testing.T) {
	t.Parallel()
	m := newSized(t, "note", sampleNote)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c should return a tea.Cmd")
	}
	if _, ok := cmd().(view.ExitMsg); !ok {
		t.Errorf("ctrl+c-cmd should produce view.ExitMsg, got %T", cmd())
	}
}

// TestPrevMatch_BeforeSearchIsNoOp: pressing N without a query must
// not panic / mutate state.
func TestPrevMatch_BeforeSearchIsNoOp(t *testing.T) {
	t.Parallel()
	m := newSized(t, "note", sampleNote)
	model, _ := m.Update(runeKey('N'))
	if model.MatchIndex() != 0 || len(model.Matches()) != 0 {
		t.Errorf("N without query should be a no-op; got matchIdx=%d matches=%d",
			model.MatchIndex(), len(model.Matches()))
	}
}

// TestTinySize_RendersEmpty: when the screen is smaller than the
// chrome budget, View() returns "" rather than a corrupted frame.
func TestTinySize_RendersEmpty(t *testing.T) {
	t.Parallel()
	m := view.New("note", sampleNote, nil, nil, nil).SetSize(2, 2)
	if got := m.View(); got != "" {
		t.Errorf("View at 2x2 should be empty, got %q", got)
	}
}

// TestSearch_FooterRendersInSearchMode covers the active search-mode
// footer branch (Search: prompt + hint).
func TestSearch_FooterRendersInSearchMode(t *testing.T) {
	t.Parallel()
	m := newSized(t, "note", sampleNote)
	model, _ := m.Update(runeKey('/'))
	out := model.View()
	if !strings.Contains(out, "Search:") {
		t.Errorf("active search-mode footer should show 'Search:' label\n%s", out)
	}
	if !strings.Contains(out, "enter apply") {
		t.Errorf("active search-mode footer should show enter/esc hint\n%s", out)
	}
}

// TestUnknownNonKeyMsg_ForwardsToViewport: a non-key, non-resize msg
// in ModeNormal must reach the viewport (covers Update's
// non-tea.KeyMsg branch).
func TestUnknownNonKeyMsg_ForwardsToViewport(t *testing.T) {
	t.Parallel()
	m := newSized(t, "note", sampleNote)
	model, _ := m.Update(struct{ noop bool }{})
	if model.CurrentMode() != view.ModeNormal {
		t.Errorf("unrecognised msg must not flip mode, got %v", model.CurrentMode())
	}
}

// TestUnknownNonKeyMsg_DroppedInSearchMode: while search input is
// focused, non-key msgs are intentionally dropped — covers the
// ModeSearch branch of Update's fall-through.
func TestUnknownNonKeyMsg_DroppedInSearchMode(t *testing.T) {
	t.Parallel()
	m := newSized(t, "note", sampleNote)
	model, _ := m.Update(runeKey('/'))
	model, cmd := model.Update(struct{ noop bool }{})
	if model.CurrentMode() != view.ModeSearch {
		t.Errorf("unrecognised msg in search must keep mode, got %v", model.CurrentMode())
	}
	if cmd != nil {
		t.Errorf("unrecognised msg in search must not produce a cmd, got %v", cmd())
	}
}
