package browse

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

// editFinishedMsg is unexported, so the reload-after-edit assertion lives
// in this internal test rather than the external _test.go.

func TestEditFinishedMsg_Success_TriggersReload(t *testing.T) {
	t.Parallel()

	store := testutil.NewFakeNoteStore()
	store.Seed(mustValidNote("daily/2026-04-25"), time.Unix(1, 0))

	m := New(usecase.NewListNotes(store), store, nil, "", nil, nil)
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
	m := New(usecase.NewListNotes(testutil.NewFakeNoteStore()), nil, nil, "", nil, nil)
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
	if !contains(pm.View(), "Fehler beim Bearbeiten") {
		t.Errorf("editErr should surface in View, got:\n%s", pm.View())
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
