package statusbar_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/httpapi"
	"github.com/serverkraken/flow/internal/frontend/tui/components/statusbar"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

func TestConnState(t *testing.T) {
	pal := theme.TokyonightNight
	tests := []struct {
		state httpapi.ConnState
		want  string
	}{
		{httpapi.StateOnline, "●"},
		{httpapi.StateOffline, "○ offline"},
		{httpapi.StateLoggedOut, "○ nicht angemeldet"},
		{httpapi.StateNotConfigured, "○ kein Server"},
		{httpapi.StateOutdated, "▲ Client veraltet"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			snap := httpapi.StatusSnapshot{State: tt.state}
			got := statusbar.ConnState(snap, pal)
			if !strings.Contains(stripANSI(got), tt.want) {
				t.Errorf("ConnState(%v) = %q, want to contain %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestConnState_Unknown(t *testing.T) {
	pal := theme.TokyonightNight
	snap := httpapi.StatusSnapshot{State: httpapi.StateUnknown}
	got := statusbar.ConnState(snap, pal)
	if got != "" {
		t.Errorf("ConnState(StateUnknown) = %q, want empty string", got)
	}
}

func TestConnState_OnlineHostOnly(t *testing.T) {
	pal := theme.TokyonightNight
	snap := httpapi.StatusSnapshot{State: httpapi.StateOnline, Host: "https://flow.example.com/"}
	got := statusbar.ConnState(snap, pal)
	raw := stripANSI(got)
	if !strings.Contains(raw, "flow.example.com") {
		t.Errorf("ConnState online host = %q, want to contain bare host", raw)
	}
	if strings.Contains(raw, "https://") {
		t.Errorf("ConnState online host = %q, should not contain scheme", raw)
	}
}

// stripANSI removes ANSI escape sequences from s so tests can compare
// plain text without lipgloss color codes.
func stripANSI(s string) string {
	var out strings.Builder
	inEsc := false
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b == 0x1b {
			inEsc = true
			continue
		}
		if inEsc {
			if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') {
				inEsc = false
			}
			continue
		}
		out.WriteByte(b)
	}
	return out.String()
}
