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
	if !strings.Contains(got, `/* Background */ :root:not([data-theme="dark"]) .bg {`) {
		t.Fatalf("light theme selector is not scoped: %s", got[:200])
	}
	if !strings.Contains(got, `/* Background */ :root[data-theme="dark"] .bg {`) {
		t.Fatalf("dark theme selector is not scoped")
	}
}

func TestGenerateChromaCSSLightRulesDoNotApplyInDarkTheme(t *testing.T) {
	got := GenerateChromaCSS()
	if !strings.Contains(got, `:root:not([data-theme="dark"]) .chroma .k {`) {
		t.Fatalf("light token selectors must be excluded from dark theme")
	}
	if strings.Contains(got, `:root .chroma .k {`) {
		t.Fatalf("unqualified light token selector leaks dark keyword colors")
	}
}
