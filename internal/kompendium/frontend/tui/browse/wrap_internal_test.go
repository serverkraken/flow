package browse

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
)

func TestSoftWrap(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		in       string
		width    int
		maxLines int
		want     []string
	}{
		{name: "empty input → nil", in: "", width: 20, maxLines: 2, want: nil},
		{name: "fits one line", in: "short text here", width: 40, maxLines: 2, want: []string{"short text here"}},
		{
			name:     "wraps to two lines",
			in:       "the quick brown fox jumps over the lazy dog",
			width:    20,
			maxLines: 2,
			want:     []string{"the quick brown fox", "jumps over the lazy…"},
		},
		{
			name:     "ellipsis when content overflows cap",
			in:       "alpha bravo charlie delta echo foxtrot golf hotel",
			width:    12,
			maxLines: 1,
			want:     []string{"alpha bravo…"},
		},
		{
			name:     "hard-truncates oversized words",
			in:       "verylongwordthatexceedswidth tail",
			width:    10,
			maxLines: 2,
			want:     []string{"verylongw…", "tail"},
		},
		{name: "width below floor returns nil", in: "a b c", width: 4, maxLines: 2, want: nil},
		{name: "maxLines zero returns nil", in: "a b c", width: 30, maxLines: 0, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := softWrap(tc.in, tc.width, tc.maxLines)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("softWrap got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestAppendEllipsis(t *testing.T) {
	t.Parallel()
	if got := appendEllipsis("hello", 20); got != "hello…" {
		t.Errorf("room for ellipsis got %q", got)
	}
	if got := appendEllipsis("filledtothebrim", 10); got != "filledtoth…" && got != "filledtot…" {
		// truncateText keeps maxCells-1 chars then adds "…", so for width 10 we expect "filledtoth…" (rune-truncate)
		t.Errorf("truncating got %q", got)
	}
}

func TestComputeListWindow(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		heights []int
		cursor  int
		budget  int
		wantS   int
		wantE   int
	}{
		{name: "empty list", heights: nil, cursor: 0, budget: 10, wantS: 0, wantE: 0},
		{name: "all fits", heights: []int{1, 1, 1}, cursor: 1, budget: 10, wantS: 0, wantE: 3},
		{name: "tight cursor at top", heights: []int{2, 2, 2, 2, 2}, cursor: 0, budget: 5, wantS: 0, wantE: 2},
		{name: "cursor in middle backfills upward", heights: []int{2, 2, 2, 2, 2}, cursor: 3, budget: 5, wantS: 3, wantE: 5},
		{name: "cursor past end clamps", heights: []int{1, 1}, cursor: 99, budget: 10, wantS: 0, wantE: 2},
		{name: "negative cursor clamps", heights: []int{1, 1}, cursor: -5, budget: 10, wantS: 0, wantE: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, e := computeListWindow(tc.heights, tc.cursor, tc.budget)
			if s != tc.wantS || e != tc.wantE {
				t.Errorf("computeListWindow(%v, %d, %d) = (%d, %d), want (%d, %d)",
					tc.heights, tc.cursor, tc.budget, s, e, tc.wantS, tc.wantE)
			}
		})
	}
}

func TestHumanizeAge(t *testing.T) {
	t.Parallel()
	cases := []struct {
		d    time.Duration
		want string
	}{
		{d: 200 * time.Millisecond, want: "<1s"},
		{d: 5 * time.Second, want: "5s"},
		{d: 90 * time.Second, want: "1m"},
		{d: 90 * time.Minute, want: "1h"},
		{d: 36 * time.Hour, want: "1d"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			if got := humanizeAge(tc.d); got != tc.want {
				t.Errorf("humanizeAge(%s) = %q, want %q", tc.d, got, tc.want)
			}
		})
	}
}

func TestExcerptParagraph_FallbackAndWrap(t *testing.T) {
	t.Parallel()
	m := Model{
		bodies: map[domain.ID][]byte{
			"daily/2026-04-25": []byte("First meaningful line here. Plus another paragraph with more content.\n"),
			"daily/empty":      []byte(""),
		},
	}
	e := ports.NoteEntry{
		ID:   "daily/2026-04-25",
		Meta: domain.Frontmatter{Type: domain.TypeDaily, Date: "2026-04-25"},
	}

	// Width too small → falls back to single-line excerptFor.
	if got := m.excerptParagraph(e, 0, 2); len(got) != 1 {
		t.Errorf("zero width should return single line, got %v", got)
	}

	// Generous width → multi-line wrap is allowed (may end up as one line if it fits).
	got := m.excerptParagraph(e, 30, 2)
	if len(got) == 0 {
		t.Fatalf("expected wrapped excerpt, got empty")
	}
	for _, line := range got {
		if line == "" {
			t.Errorf("wrapped line should not be empty")
		}
	}

	// Missing body → nil.
	missing := ports.NoteEntry{ID: "does/not/exist"}
	if got := m.excerptParagraph(missing, 30, 2); got != nil {
		t.Errorf("missing body should return nil, got %v", got)
	}

	// Body present but only redundant lines → nil.
	m2 := Model{bodies: map[domain.ID][]byte{"projects/x/2026-04-25": []byte("2026-04-25\n")}}
	e2 := ports.NoteEntry{
		ID:   "projects/x/2026-04-25",
		Meta: domain.Frontmatter{Type: domain.TypeProject, Date: "2026-04-25"},
	}
	if got := m2.excerptParagraph(e2, 40, 2); got != nil {
		t.Errorf("redundant-only body should return nil, got %v", got)
	}
}

func TestStatusBar_RendersInView(t *testing.T) {
	t.Parallel()
	// Status bar shows path + index age in normal mode (no badge);
	// transient modes (Search, ConfirmDelete) get a colored badge.
	m := Model{
		all: []ports.NoteEntry{
			{ID: "daily/2026-04-25", Meta: domain.Frontmatter{Type: domain.TypeDaily, Title: "x", Date: "2026-04-25"}},
		},
		visible:  []ports.NoteEntry{{ID: "daily/2026-04-25", Meta: domain.Frontmatter{Type: domain.TypeDaily, Title: "x", Date: "2026-04-25"}}},
		loaded:   true,
		width:    80,
		height:   24,
		indexAge: func() time.Time { return time.Now().Add(-90 * time.Second) },
	}
	view := m.View().Content
	if strings.Contains(view, "NORMAL") {
		t.Errorf("status bar must not render a NORMAL badge in normal mode:\n%s", view)
	}
	if !strings.Contains(view, "daily/2026-04-25") {
		t.Errorf("status bar should show selected note path:\n%s", view)
	}
	if !strings.Contains(view, "Index 1m") {
		t.Errorf("status bar should show humanized index age:\n%s", view)
	}

	// Welle 4 / Skill §German UI: Mode-Badges sind deutsch (SUCHE / LÖSCHEN).
	// Mode badges are rendered into the status bar before the overlay
	// kicks in, so we exercise the bar's mode helper directly — the
	// confirm-delete modal otherwise replaces the entire frame.
	m.mode = ModeSearch
	if !strings.Contains(m.statusBarMode(), "SUCHE") {
		t.Errorf("search mode should surface a SUCHE badge: %q", m.statusBarMode())
	}
	m.mode = ModeConfirmDelete
	if !strings.Contains(m.statusBarMode(), "LÖSCHEN") {
		t.Errorf("delete-confirm mode should surface a LÖSCHEN badge: %q", m.statusBarMode())
	}
}

func TestStatusBar_TruncatesLongPath(t *testing.T) {
	t.Parallel()
	longID := domain.ID("projects/github.com/serverkraken/homelab-study/_project")
	m := Model{
		visible: []ports.NoteEntry{{ID: longID, Meta: domain.Frontmatter{Type: domain.TypeProject}}},
		loaded:  true,
		width:   60,
		height:  24,
	}
	view := m.View().Content
	// Under lipgloss v2 every styled cell emits a full 24-bit SGR
	// sequence (38;2;R;G;B;) regardless of TTY detection — that alone
	// pushes a 60-cell row past the 200-byte budget even when the
	// visible width is fine. Strip ANSI first and check the actual
	// cell width via lipgloss.Width.
	for _, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > 60 {
			t.Errorf("status bar line ran past frame width — wrap risk (cells=%d):\n%q", w, ansi.Strip(line))
		}
	}
	if !strings.Contains(view, "…") {
		t.Errorf("long path should be truncated with an ellipsis, view:\n%s", view)
	}
}

// TestLayout_LongBodyDoesNotOverflowFrame guards against a regression
// where a very long Markdown body in the preview pane (or a long
// excerpt in the list pane) made the rendered View exceed the
// terminal's height, pushing the list off-screen. Both panes must
// stay within the frame's interior.
func TestLayout_LongBodyDoesNotOverflowFrame(t *testing.T) {
	t.Parallel()
	const width, height = 140, 30
	bigBody := strings.Repeat("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ", 200)
	m := Model{
		visible: []ports.NoteEntry{
			{ID: "daily/2026-04-25", Meta: domain.Frontmatter{Type: domain.TypeDaily, Title: "huge", Date: "2026-04-25"}},
			{ID: "daily/2026-04-24", Meta: domain.Frontmatter{Type: domain.TypeDaily, Title: "small", Date: "2026-04-24"}},
		},
		bodies: map[domain.ID][]byte{
			"daily/2026-04-25": []byte(bigBody),
			"daily/2026-04-24": []byte("short body"),
		},
		previewCached: map[domain.ID]string{},
		loaded:        true,
		width:         width,
		height:        height,
	}
	// Sync layout the way Bubble Tea would: WindowSizeMsg → layoutViewport.
	m.layoutViewport()
	m.refreshPreview()

	view := m.View().Content
	gotLines := strings.Count(view, "\n") + 1
	if gotLines > height {
		t.Errorf("View rendered %d lines into a %d-row terminal — preview/list overflow risk:\n%s",
			gotLines, height, view)
	}
	// Both rows must stay visible — the bug surfaced as the second row
	// being pushed past the bottom border.
	if !strings.Contains(view, "huge") || !strings.Contains(view, "small") {
		t.Errorf("both list rows must stay visible; got:\n%s", view)
	}
}

// TestBodyLoader_CapsHugeBodies guards the OOM-mitigation: notebooks
// with many large Markdown files used to load every body fully into
// the in-memory map and get killed by the OS. Bodies past the cap must
// be truncated; bodies under the cap pass through unchanged.
func TestBodyLoader_CapsHugeBodies(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	hugeBody := strings.Repeat("x", bodyExcerptLimit*4)
	huge, _ := domain.NewNote(
		domain.ID("daily/2026-04-25"),
		domain.Frontmatter{ID: "daily/2026-04-25", Type: domain.TypeDaily, Date: "2026-04-25"},
		[]byte(hugeBody),
	)
	store.Seed(huge, time.Unix(2, 0))

	tinyBody := []byte("just a few bytes\n")
	tiny, _ := domain.NewNote(
		domain.ID("daily/2026-04-24"),
		domain.Frontmatter{ID: "daily/2026-04-24", Type: domain.TypeDaily, Date: "2026-04-24"},
		tinyBody,
	)
	store.Seed(tiny, time.Unix(1, 0))

	cmd := loadBodiesCmd(store, []ports.NoteEntry{
		{ID: "daily/2026-04-25"},
		{ID: "daily/2026-04-24"},
	})
	msg, ok := cmd().(bodiesLoadedMsg)
	if !ok {
		t.Fatalf("loadBodiesCmd produced %T, want bodiesLoadedMsg", cmd())
	}
	if got := msg.bodies["daily/2026-04-25"]; len(got) != bodyExcerptLimit {
		t.Errorf("oversize body should be capped to %d bytes, got %d", bodyExcerptLimit, len(got))
	}
	if got := msg.bodies["daily/2026-04-24"]; len(got) != len(tinyBody) {
		t.Errorf("tiny body should pass through unchanged; got len=%d, want %d", len(got), len(tinyBody))
	}
}

func TestStatusBar_HidesIndexWhenUnwired(t *testing.T) {
	t.Parallel()
	m := Model{
		visible: []ports.NoteEntry{},
		loaded:  true,
		width:   80,
		height:  24,
	}
	view := m.View().Content
	if strings.Contains(view, "Index ") {
		t.Errorf("status bar must not render `Index Nm` when IndexAgeFunc is nil:\n%s", view)
	}
	if !strings.Contains(view, "—") {
		t.Errorf("status bar should fall back to em-dash for empty cursor:\n%s", view)
	}
}
