package browse

import (
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
)

// TestRenderPreviewBody_EmitsOSC8 exercises the full preview-body path
// against a real Model — markdown.Render (glamour + URL wrap) + cache
// write — and asserts the cached output carries OSC 8 markers around
// the URL the note body contains. This is the on-the-wire bytes that
// get passed to the viewport, so if the assertion holds the rendered
// preview really is going out with hyperlinks. Failures here mean the
// integration regressed somewhere outside markdown.WrapURLs itself.
func TestRenderPreviewBody_EmitsOSC8(t *testing.T) {
	t.Parallel()
	const url = "https://example.com/some/path"
	const id = "daily/2026-04-25"

	store := testutil.NewFakeNoteStore()
	note, err := domain.NewNote(
		domain.ID(id),
		domain.Frontmatter{ID: id, Type: domain.TypeDaily, Date: "2026-04-25"},
		[]byte("Look here: "+url+"\n"),
	)
	if err != nil {
		t.Fatalf("seed note: %v", err)
	}
	store.Seed(note, time.Unix(1, 0))

	m := Model{
		store:         store,
		previewCached: map[domain.ID]string{},
		width:         140, // >= twoPaneMinWidth so previewSize > 0
		height:        30,
	}
	entry := ports.NoteEntry{
		ID:   domain.ID(id),
		Meta: domain.Frontmatter{ID: id, Type: domain.TypeDaily, Date: "2026-04-25"},
	}
	got := m.renderPreviewBody(entry)

	if !strings.Contains(got, ";"+url+"\x07") {
		t.Errorf("renderPreviewBody output missing OSC 8 destination for %q\n%q", url, got)
	}
	if !strings.Contains(got, "\x1b]8;;\x07") {
		t.Errorf("renderPreviewBody output missing OSC 8 closer\n%q", got)
	}
}

// TestView_PreservesOSC8ThroughLipglossAndViewport asserts that the
// OSC 8 hyperlink survives the *full* render path — from the cached
// preview body, through the bubbles viewport (which line-splits the
// content), through lipgloss/cellbuf (which width-wraps the panel), all
// the way to the terminal-bound bytes returned by Model.View. If this
// fails, the issue is downstream of markdown.WrapURLs and we lost the
// link somewhere in the rendering stack.
func TestView_PreservesOSC8ThroughLipglossAndViewport(t *testing.T) {
	t.Parallel()
	const url = "https://example.com/some/path/that/runs/long"
	const id = "daily/2026-04-25"

	store := testutil.NewFakeNoteStore()
	note, err := domain.NewNote(
		domain.ID(id),
		domain.Frontmatter{ID: id, Type: domain.TypeDaily, Date: "2026-04-25"},
		[]byte("Linkzeile: "+url+"\n"),
	)
	if err != nil {
		t.Fatalf("seed note: %v", err)
	}
	store.Seed(note, time.Unix(1, 0))

	entry := ports.NoteEntry{
		ID:   domain.ID(id),
		Meta: domain.Frontmatter{ID: id, Type: domain.TypeDaily, Date: "2026-04-25"},
	}
	m := Model{
		store:         store,
		previewCached: map[domain.ID]string{},
		all:           []ports.NoteEntry{entry},
		visible:       []ports.NoteEntry{entry},
		loaded:        true,
		width:         140,
		height:        30,
	}
	m.layoutViewport()
	m.refreshPreview()
	view := m.View()

	if !strings.Contains(view, "\x1b]8;") {
		t.Errorf("Model.View output missing any OSC 8 marker — link lost in render pipeline\n%q", view)
	}
}
