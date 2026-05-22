package browse

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

// TestEditFinishedReloadsPreview reproduces the "preview stays
// stale after editing" bug reported in P1.12: after the
// editor exits, the preview pane has to re-render against the
// just-saved file, not return the cached pre-edit body. Drives the
// scenario through the public Update path with a real fake store —
// the editFinishedMsg landing must invalidate previewCached +
// previewID and the follow-up loadEntries cycle must repopulate
// the rendered preview from the new store contents.
func TestEditFinishedReloadsPreview(t *testing.T) {
	t.Parallel()

	const id = "daily/2026-04-25"
	store := testutil.NewFakeNoteStore()
	original, err := domain.NewNote(
		domain.ID(id),
		domain.Frontmatter{ID: id, Type: domain.TypeDaily, Date: "2026-04-25"},
		[]byte("original-body-marker\n"),
	)
	if err != nil {
		t.Fatalf("seed original: %v", err)
	}
	store.Seed(original, time.Unix(1, 0))

	entry := ports.NoteEntry{ID: domain.ID(id), Meta: original.Meta}
	m := Model{
		store:         store,
		previewCached: map[domain.ID]string{},
		all:           []ports.NoteEntry{entry},
		visible:       []ports.NoteEntry{entry},
		loaded:        true,
		width:         140,
		height:        30,
		list:          usecase.NewListNotes(store),
	}
	m.layoutViewport()
	m.refreshPreview()
	if !strings.Contains(m.View().Content, "original-body-marker") {
		t.Fatalf("baseline preview should show original body:\n%s", m.View().Content)
	}

	// Mutate the store underneath — simulates the editor saving
	// a new version of the note file.
	updated, err := domain.NewNote(
		domain.ID(id),
		original.Meta,
		[]byte("edited-body-marker\n"),
	)
	if err != nil {
		t.Fatalf("seed updated: %v", err)
	}
	store.Seed(updated, time.Unix(2, 0))

	// Send editFinishedMsg through Update — this is where the
	// cache-invalidation fix kicks in. Then drive the follow-up
	// loadEntriesCmd it returns so refreshPreview sees the new
	// store contents.
	model, cmd := m.Update(editFinishedMsg{})
	if cmd != nil {
		model, _ = model.Update(cmd())
	}
	got := model.View().Content
	if strings.Contains(got, "original-body-marker") {
		t.Errorf("preview still shows original body after editFinishedMsg:\n%s", got)
	}
	if !strings.Contains(got, "edited-body-marker") {
		t.Errorf("preview missing edited body after edit return:\n%s", got)
	}
}

// TestResizeReloadsPreview reproduces the "tmux pane resize ist
// dem Modell egal"-Bug: a WindowSizeMsg arriving while the preview
// is already rendered must drop the cached body so the next
// refreshPreview re-renders against the new width. The bug was the
// same shape as the post-edit one — clearing previewCached without
// resetting previewID let refreshPreview short-circuit.
func TestResizeReloadsPreview(t *testing.T) {
	t.Parallel()

	const id = "daily/2026-04-25"
	const longBody = "This is a paragraph long enough that it must wrap differently at width 80 versus width 40, so its rendered byte sequence will differ between the two widths.\n"

	store := testutil.NewFakeNoteStore()
	note, err := domain.NewNote(
		domain.ID(id),
		domain.Frontmatter{ID: id, Type: domain.TypeDaily, Date: "2026-04-25"},
		[]byte(longBody),
	)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	store.Seed(note, time.Unix(1, 0))

	entry := ports.NoteEntry{ID: domain.ID(id), Meta: note.Meta}
	m := Model{
		store:         store,
		previewCached: map[domain.ID]string{},
		all:           []ports.NoteEntry{entry},
		visible:       []ports.NoteEntry{entry},
		loaded:        true,
		width:         140,
		height:        40,
		list:          usecase.NewListNotes(store),
	}
	m.layoutViewport()
	m.refreshPreview()
	wide := m.View().Content

	// Resize within the two-pane regime (both widths >= twoPaneMinWidth)
	// so the preview pane stays visible — otherwise the assertion would
	// catch the layout collapse instead of the wrap-cache bug.
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	narrow := model.View().Content

	if wide == narrow {
		t.Errorf("View output unchanged after resize 140 → 100; resize ignored")
	}
	if !strings.Contains(narrow, "paragraph") {
		t.Errorf("narrow view dropped body content:\n%s", narrow)
	}
}

// keep tea import alive — both tests reference its types via Update.
var _ tea.Msg = editFinishedMsg{}
