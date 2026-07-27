package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestRender_Callout_NoteWithTitle: a `> [!NOTE] Title` block
// renders the colored badge + title + bar-prefixed body.
func TestRender_Callout_NoteWithTitle(t *testing.T) {
	t.Parallel()
	src := "> [!NOTE] Wichtige Notiz\n>\n> Body steht hier.\n"
	out, err := Render(src, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "NOTE") {
		t.Errorf("badge text missing:\n%s", plain)
	}
	if !strings.Contains(plain, "Wichtige Notiz") {
		t.Errorf("title missing:\n%s", plain)
	}
	if !strings.Contains(plain, "Body steht hier") {
		t.Errorf("body missing:\n%s", plain)
	}
}

// TestRender_Callout_AllRecognisedKinds: every supported kind
// produces a badge — verifies the seven knownCallout switch cases.
func TestRender_Callout_AllRecognisedKinds(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"NOTE", "TIP", "INFO", "WARNING", "DANGER", "IMPORTANT", "SUCCESS"} {
		t.Run(kind, func(t *testing.T) {
			out, err := Render("> [!"+kind+"] x\n>\n> y\n", 60)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if !strings.Contains(ansi.Strip(out), kind) {
				t.Errorf("kind %q badge missing:\n%s", kind, ansi.Strip(out))
			}
		})
	}
}

// TestRender_Callout_UnknownKindFallsBackToPlainQuote: a
// `> [!HUSO]` (typo / custom kind not in the registry) renders as a
// plain blockquote — keeps the marker in the output so the user
// sees what they wrote and can fix the typo.
func TestRender_Callout_UnknownKindFallsBackToPlainQuote(t *testing.T) {
	t.Parallel()
	src := "> [!HUSO] foo\n> bar\n"
	out, err := Render(src, 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "[!HUSO]") {
		t.Errorf("unknown-kind marker should pass through verbatim:\n%s", plain)
	}
}

// TestRender_Blockquote_Plain: a quote without a callout marker
// renders with the muted │ leader.
func TestRender_Blockquote_Plain(t *testing.T) {
	t.Parallel()
	out, err := Render("> Eine ganz normale Quote\n> über zwei Zeilen.\n", 60)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "│") {
		t.Errorf("plain blockquote missing leader bar:\n%s", plain)
	}
	if !strings.Contains(plain, "ganz normale Quote") {
		t.Errorf("blockquote body missing:\n%s", plain)
	}
}
