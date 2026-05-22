package browse_test

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/browse"
	"github.com/serverkraken/flow/internal/kompendium/frontend/tui/writepicker"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

// Polish-coverage tests: help overlay, two-pane preview, mouse wheel,
// page navigation, tag chips. These exercise branches the original
// browse_test.go didn't, so the coverage gate stays above 90% after the
// TUI rewrite.

func TestPolish_HelpOverlayToggles(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "today"), time.Unix(1, 0))
	model := initialised(newModel(usecase.NewListNotes(store)))

	// `?` opens the overlay; the overlay carries the "Tastenbelegung" header.
	model, _ = model.Update(runeKey('?'))
	if !strings.Contains(model.View(), "Tastenbelegung") {
		t.Errorf("? should open help overlay:\n%s", model.View())
	}

	// `?` again closes it.
	model, _ = model.Update(runeKey('?'))
	if strings.Contains(model.View(), "Tastenbelegung") {
		t.Errorf("? again should close help overlay:\n%s", model.View())
	}

	// re-open then close with esc.
	model, _ = model.Update(runeKey('?'))
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if strings.Contains(model.View(), "Tastenbelegung") {
		t.Errorf("esc should close help overlay:\n%s", model.View())
	}
}

func TestPolish_TwoPanePreviewWithSize(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	body, _ := domain.NewNote(
		domain.ID("daily/2026-04-25"),
		domain.Frontmatter{ID: "daily/2026-04-25", Type: domain.TypeDaily, Date: "2026-04-25", Title: "header", Tags: []string{"tmux", "setup"}},
		[]byte("# heading\n\nfirst line of body\n"),
	)
	store.Seed(body, time.Unix(1, 0))

	noopCmd := func(_ string) *exec.Cmd { return exec.Command("true") }
	noopWrite := func(writepicker.Result) *exec.Cmd { return exec.Command("true") }
	m := browse.New(usecase.NewListNotes(store), store, nil, "", noopCmd, noopWrite)

	// Drive entries → bodies, then resize wide enough to enable the two-pane layout.
	model, cmd := m.Update(m.Init()())
	if cmd != nil {
		model, _ = model.Update(cmd())
	}
	model, _ = model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})

	view := model.View()
	if !strings.Contains(view, "vorschau") {
		t.Errorf("two-pane layout should render the preview pane title:\n%s", view)
	}
	// Tag chips render the tag text verbatim somewhere in the row block.
	if !strings.Contains(view, "tmux") || !strings.Contains(view, "setup") {
		t.Errorf("tag chips missing from row:\n%s", view)
	}
}

func TestPolish_MouseWheelNavigatesCursor(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "first"), time.Unix(2, 0))
	store.Seed(mustNote("daily/2026-04-22", domain.TypeDaily, "second"), time.Unix(1, 0))

	model := initialised(newModel(usecase.NewListNotes(store)))

	model, _ = model.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	if !cursorOnLineWith(model.View(), "second") {
		t.Errorf("wheel down should move cursor down:\n%s", model.View())
	}
	model, _ = model.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	if !cursorOnLineWith(model.View(), "first") {
		t.Errorf("wheel up should move cursor up:\n%s", model.View())
	}
}

func TestPolish_MouseNonPressIgnored(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustNote("daily/2026-04-25", domain.TypeDaily, "first"), time.Unix(2, 0))
	store.Seed(mustNote("daily/2026-04-22", domain.TypeDaily, "second"), time.Unix(1, 0))

	model := initialised(newModel(usecase.NewListNotes(store)))
	model, _ = model.Update(tea.MouseMsg{Action: tea.MouseActionRelease, Button: tea.MouseButtonWheelDown})
	if !cursorOnLineWith(model.View(), "first") {
		t.Errorf("non-press mouse events should not move the cursor:\n%s", model.View())
	}
}

func TestPolish_PageDownThenPageUp(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	for i := range 30 {
		id := domain.ID("daily/2026-04-" + twoDigit(i+1))
		store.Seed(mustNoteAt(id, "n"+twoDigit(i+1)), time.Unix(int64(100-i), 0))
	}
	model := initialised(newModel(usecase.NewListNotes(store)))
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 30})

	// PageDown then PageUp must end on a valid cursor (some entry rendered).
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	if !strings.Contains(model.View(), "▶") {
		t.Errorf("page nav left no cursor on screen:\n%s", model.View())
	}
}

func TestPolish_GotoBottomThenTopWithScroll(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	for i := range 25 {
		id := domain.ID("daily/2026-04-" + twoDigit(i+1))
		store.Seed(mustNoteAt(id, "row"+twoDigit(i+1)), time.Unix(int64(100-i), 0))
	}
	model := initialised(newModel(usecase.NewListNotes(store)))
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 20})

	model, _ = model.Update(runeKey('G'))
	// Long lists carry a paginator dot indicator + an "N/M" counter; the
	// older "showing N–M of K" textual line was dropped when the dots
	// went in.
	if !strings.Contains(model.View(), "/25") {
		t.Errorf("paginator counter missing on bottom of long list:\n%s", model.View())
	}
	model, _ = model.Update(runeKey('g'))
	if !cursorOnLineWith(model.View(), "row01") {
		t.Errorf("g should jump to the first row, got:\n%s", model.View())
	}
}

func twoDigit(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	tens := n / 10
	ones := n % 10
	return string(rune('0'+tens)) + string(rune('0'+ones))
}

func mustNoteAt(id domain.ID, title string) domain.Note {
	n, err := domain.NewNote(id, domain.Frontmatter{
		ID: id.String(), Type: domain.TypeDaily, Title: title, Date: string(id)[len("daily/"):],
	}, []byte(title+" body\n"))
	if err != nil {
		panic(err)
	}
	return n
}
