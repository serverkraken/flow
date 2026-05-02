package glamourrenderer_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/glamourrenderer"
	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.MarkdownRenderer = glamourrenderer.Renderer{}

func TestRender_PreservesContent(t *testing.T) {
	got, err := glamourrenderer.New().Render("# heading\n\nbody text\n", 80)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got == "" {
		t.Fatal("output is empty")
	}
	// glamour interleaves ANSI styling between words, so check tokens
	// individually rather than the literal "body text" pair.
	for _, want := range []string{"heading", "body", "text"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered output missing %q: %q", want, got)
		}
	}
}

func TestRender_NarrowWidth_NoCrash(t *testing.T) {
	// Width <= 2 must not blow up; glamour requires width >= 3.
	for _, w := range []int{0, 1, 2} {
		got, err := glamourrenderer.New().Render("hello", w)
		if err != nil {
			t.Errorf("width %d: %v", w, err)
		}
		if got == "" {
			t.Errorf("width %d: empty output", w)
		}
	}
}

func TestRender_EmptyContent(t *testing.T) {
	got, err := glamourrenderer.New().Render("", 80)
	if err != nil {
		t.Fatal(err)
	}
	// glamour returns its own ANSI envelope even for empty input; we just
	// verify the call doesn't error.
	_ = got
}

func TestRender_StylesHeadings(t *testing.T) {
	got, err := glamourrenderer.New().Render("# heading\n", 80)
	if err != nil {
		t.Fatal(err)
	}
	// glamour's dark theme injects ANSI escape sequences (the leading ESC
	// byte) for heading styling — verify they are present.
	if !strings.ContainsRune(got, 0x1b) {
		t.Errorf("expected ANSI escapes in styled output, got %q", got)
	}
}
