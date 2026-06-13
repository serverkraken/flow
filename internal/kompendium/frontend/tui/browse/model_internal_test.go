package browse

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

// testPalette returns the deterministic palette tests construct
// Models against. Tokyonight-Night is the canonical palette used
// across the screen suites.
func testPalette() theme.Palette { return theme.TokyonightNight }

// editFinishedMsg is unexported, so the reload-after-edit assertion lives
// in this internal test rather than the external _test.go.

func TestEditFinishedMsg_Success_TriggersReload(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustValidNote("daily/2026-04-25"), time.Unix(1, 0))

	m := New(testPalette(), usecase.NewListNotes(store), store, nil, "", nil, nil)
	model, cmd := m.Update(editFinishedMsg{err: nil})
	if cmd == nil {
		t.Fatal("editFinishedMsg{nil} should schedule a reload cmd")
	}
	pm := model.(Model)
	if pm.editErr != nil {
		t.Errorf("editErr should be cleared on success, got %v", pm.editErr)
	}
	// Sanity: cmd, when run, returns an entriesLoadedMsg.
	msg := cmd()
	if _, ok := msg.(entriesLoadedMsg); !ok {
		t.Errorf("reload cmd should produce entriesLoadedMsg, got %T", msg)
	}
}

func TestEditFinishedMsg_Error_StoredAndShownInView(t *testing.T) {
	t.Parallel()

	forced := errors.New("nvim crashed")
	m := New(testPalette(), usecase.NewListNotes(testutil.NewFakeNoteStore()), nil, nil, "", nil, nil)
	model, cmd := m.Update(editFinishedMsg{err: forced})
	if cmd != nil {
		t.Errorf("editFinishedMsg with err must not schedule another cmd, got %v", cmd)
	}
	pm := model.(Model)
	if pm.editErr == nil {
		t.Fatal("editErr should be set")
	}
	// Force loaded so View renders; otherwise it short-circuits to „lädt…".
	pm.loaded = true
	if !contains(pm.View().Content, "Fehler beim Bearbeiten") {
		t.Errorf("editErr should surface in View, got:\n%s", pm.View().Content)
	}
}

// TestBrowse_ChangedMsgReloads confirms that delivering a changedMsg to
// Update triggers a corpus reload AND re-arms the listener (two cmds in
// the returned Batch).
func TestBrowse_ChangedMsgReloads(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustValidNote("daily/2026-04-25"), time.Unix(1, 0))

	// Buffer 1 so listenForChanged can drain immediately when invoked.
	ch := make(chan struct{}, 1)
	ch <- struct{}{} // pre-signal so the re-arm leg returns without blocking
	m := New(testPalette(), usecase.NewListNotes(store), store, nil, "", nil, nil).
		WithChanged(ch)

	_, cmd := m.Update(changedMsg{})
	if cmd == nil {
		t.Fatal("changedMsg should schedule cmds (reload + re-arm)")
	}
	// tea.Batch returns a cmd that, when invoked, returns a tea.BatchMsg
	// ([]tea.Cmd). Run each sub-cmd and assert at least one produces an
	// entriesLoadedMsg (the corpus reload).
	batchResult := cmd()
	msgs, ok := batchResult.(tea.BatchMsg)
	if !ok {
		t.Fatalf("changedMsg Batch should return tea.BatchMsg, got %T", batchResult)
	}
	foundReload := false
	for _, subCmd := range msgs {
		if subCmd == nil {
			continue
		}
		result := subCmd()
		if _, ok := result.(entriesLoadedMsg); ok {
			foundReload = true
		}
	}
	if !foundReload {
		t.Error("changedMsg → Batch should include a reload that produces entriesLoadedMsg")
	}
}

// TestBrowse_RKeyReloads confirms that pressing 'r' in normal mode returns
// a cmd that produces entriesLoadedMsg (i.e. triggers a corpus reload).
func TestBrowse_RKeyReloads(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustValidNote("daily/2026-04-25"), time.Unix(1, 0))

	m := New(testPalette(), usecase.NewListNotes(store), store, nil, "", nil, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Text: "r"})
	if cmd == nil {
		t.Fatal("'r' in normal mode should schedule a reload cmd")
	}
	msg := cmd()
	if _, ok := msg.(entriesLoadedMsg); !ok {
		t.Errorf("'r' reload cmd should produce entriesLoadedMsg, got %T", msg)
	}
}

func mustValidNote(id string) domain.Note {
	n, err := domain.NewNote(
		domain.ID(id),
		domain.Frontmatter{ID: id, Type: domain.TypeDaily, Date: "2026-04-25"},
		[]byte{},
	)
	if err != nil {
		panic(err)
	}
	return n
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
