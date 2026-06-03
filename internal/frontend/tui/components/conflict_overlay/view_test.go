package conflict_overlay_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/conflict_overlay"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// plainView strips ANSI escape codes from View() output so tests can
// check for string content without dealing with interleaved SGR sequences.
func plainView(m conflict_overlay.Model) string {
	return ansi.Strip(m.View())
}

// ── VariantSessionEdit view tests ─────────────────────────────────────────

// TestView_SessionEdit_ContainsTitle verifies the "Sync-Konflikt" title
// appears in the view.
func TestView_SessionEdit_ContainsTitle(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	v := plainView(m)
	if !strings.Contains(v, "Sync-Konflikt") {
		t.Errorf("expected 'Sync-Konflikt' in view, got:\n%s", v)
	}
}

// TestView_SessionEdit_ContainsChoiceLabels verifies all three choice
// labels appear in the view.
func TestView_SessionEdit_ContainsChoiceLabels(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	v := plainView(m)
	for _, want := range []string{
		"Server-Version übernehmen",
		"Lokal überschreiben",
		"abbrechen",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("expected %q in view, got:\n%s", want, v)
		}
	}
}

// TestView_SessionEdit_ContainsChoiceKeys verifies the key brackets
// [s], [l], [esc] appear in the view.
func TestView_SessionEdit_ContainsChoiceKeys(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	v := plainView(m)
	for _, want := range []string{"[s]", "[l]", "[esc]"} {
		if !strings.Contains(v, want) {
			t.Errorf("expected %q in view, got:\n%s", want, v)
		}
	}
}

// TestView_SessionEdit_ContainsBodyHeaders verifies the "Lokal:" /
// "Server:" column headers appear in the body.
func TestView_SessionEdit_ContainsBodyHeaders(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	v := plainView(m)
	for _, want := range []string{"Lokal:", "Server:"} {
		if !strings.Contains(v, want) {
			t.Errorf("expected %q in body, got:\n%s", want, v)
		}
	}
}

// TestView_SessionEdit_ContainsSessionDetails verifies that time and tag
// information from the sessions appear in the body.
func TestView_SessionEdit_ContainsSessionDetails(t *testing.T) {
	t.Parallel()
	m := newEditModel()
	v := plainView(m)
	for _, want := range []string{"10:00", "deep", "morning focus"} {
		if !strings.Contains(v, want) {
			t.Errorf("expected %q in body, got:\n%s", want, v)
		}
	}
}

// TestView_SessionEdit_TooSmall verifies that View() returns "" when
// the terminal is too small.
func TestView_SessionEdit_TooSmall(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	local := domain.Session{Start: t0, Stop: t0.Add(90 * time.Minute)}
	server := domain.Session{Start: t0, Stop: t0.Add(90 * time.Minute)}
	m := conflict_overlay.NewSessionEditConflict(
		local, server, theme.Default,
		func(bool) tea.Msg { return nil },
	)
	// Tiny terminal — below minWidth or chromeVertical.
	m = m.SetSize(5, 3)
	if m.View() != "" {
		t.Errorf("expected empty View() for tiny terminal, got non-empty output")
	}
}

// ── VariantActiveRace view tests ──────────────────────────────────────────

// TestView_ActiveRace_ContainsTitle verifies the conflict title appears.
func TestView_ActiveRace_ContainsTitle(t *testing.T) {
	t.Parallel()
	m := newRaceModel()
	v := plainView(m)
	if !strings.Contains(v, "Aktive Session") {
		t.Errorf("expected 'Aktive Session' in view, got:\n%s", v)
	}
}

// TestView_ActiveRace_ContainsChoiceLabels verifies all three choice
// labels appear in the view.
func TestView_ActiveRace_ContainsChoiceLabels(t *testing.T) {
	t.Parallel()
	m := newRaceModel()
	v := plainView(m)
	for _, want := range []string{
		"Übernehmen",
		"Neue parallele Session starten",
		"abbrechen",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("expected %q in view, got:\n%s", want, v)
		}
	}
}

// TestView_ActiveRace_ContainsChoiceKeys verifies the key brackets
// [t], [n], [esc] appear in the view.
func TestView_ActiveRace_ContainsChoiceKeys(t *testing.T) {
	t.Parallel()
	m := newRaceModel()
	v := plainView(m)
	for _, want := range []string{"[t]", "[n]", "[esc]"} {
		if !strings.Contains(v, want) {
			t.Errorf("expected %q in view, got:\n%s", want, v)
		}
	}
}

// TestView_ActiveRace_ContainsBodyDetails verifies the "Auf einem anderen
// Gerät" header and device name appear in the body.
func TestView_ActiveRace_ContainsBodyDetails(t *testing.T) {
	t.Parallel()
	m := newRaceModel()
	v := plainView(m)
	for _, want := range []string{
		"Auf einem anderen Gerät",
		"notebook-b",
		"flow",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("expected %q in body, got:\n%s", want, v)
		}
	}
}

// TestView_ActiveRace_TooSmall verifies that View() returns "" for tiny
// terminal dimensions.
func TestView_ActiveRace_TooSmall(t *testing.T) {
	t.Parallel()
	srv := domain.ActiveSession{
		ProjectID:       "flow",
		StartedAt:       time.Now().Add(-5 * time.Minute),
		StartedOnDevice: "notebook-b",
	}
	m := conflict_overlay.NewActiveRaceConflict(
		srv, theme.Default,
		func() tea.Msg { return nil },
		func() tea.Msg { return nil },
	)
	m = m.SetSize(5, 3)
	if m.View() != "" {
		t.Errorf("expected empty View() for tiny terminal, got non-empty output")
	}
}
