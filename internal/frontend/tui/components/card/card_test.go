package card_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/components/card"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func TestRender_TitleAndBody(t *testing.T) {
	t.Parallel()
	out := card.Render(card.Opts{Title: "Note", Body: "first line\nsecond"}, theme.TokyonightNight)
	if !strings.Contains(out, "Note") {
		t.Errorf("missing title in %q", out)
	}
	if !strings.Contains(out, "first line") || !strings.Contains(out, "second") {
		t.Errorf("missing body lines in %q", out)
	}
}

func TestRender_BadgeAndMeta(t *testing.T) {
	t.Parallel()
	out := card.Render(card.Opts{
		Badge: "[NOTE]", Title: "Hello", Meta: "2026-05-03",
		Width: 50,
	}, theme.TokyonightNight)
	if !strings.Contains(out, "[NOTE]") {
		t.Errorf("missing badge in %q", out)
	}
	if !strings.Contains(out, "Hello") {
		t.Errorf("missing title in %q", out)
	}
	if !strings.Contains(out, "2026-05-03") {
		t.Errorf("missing meta in %q", out)
	}
}

func TestRender_Separator(t *testing.T) {
	t.Parallel()
	out := card.Render(card.Opts{Title: "T", Body: "B", Separator: true, Width: 20}, theme.TokyonightNight)
	if !strings.Contains(out, "─") {
		t.Errorf("missing separator rule in %q", out)
	}
}

func TestRender_Empty(t *testing.T) {
	t.Parallel()
	if got := card.Render(card.Opts{}, theme.TokyonightNight); got != "" {
		t.Errorf("empty Opts should render empty, got %q", got)
	}
}
