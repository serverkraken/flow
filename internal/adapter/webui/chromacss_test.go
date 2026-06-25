package webui

import (
	"os"
	"strings"
	"testing"
)

// TestChromaCSSUpToDate fails if static/chroma.css drifts from the generator.
// Regenerate with: go generate ./internal/adapter/webui/...
func TestChromaCSSUpToDate(t *testing.T) {
	want := GenerateChromaCSS()
	got, err := os.ReadFile("static/chroma.css")
	if err != nil {
		t.Fatalf("read chroma.css: %v (run go generate)", err)
	}
	if string(got) != want {
		t.Fatalf("static/chroma.css is stale - run: go generate ./internal/adapter/webui/...")
	}
}

func TestGenerateChromaCSSScopesThemes(t *testing.T) {
	got := GenerateChromaCSS()
	if !strings.Contains(got, "/* Background */ :root .bg {") {
		t.Fatalf("light theme selector is not scoped: %s", got[:200])
	}
	if !strings.Contains(got, `/* Background */ :root[data-theme="dark"] .bg {`) {
		t.Fatalf("dark theme selector is not scoped")
	}
}
