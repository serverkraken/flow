package webui

import (
	"embed"
	"regexp"
	"strings"
)

//go:embed icons/*.svg
var iconFS embed.FS

// iconDim rewrites Lucide's fixed 24px dimensions so the SVG fills whatever
// sized box the caller renders it into (viewBox is preserved).
var iconDim = regexp.MustCompile(`(width|height)="24"`)

// nodeIconSVG maps icon keys (= vendored filenames, = domain.NodeIcons) to
// render-ready inline SVG markup. Lucide strokes with currentColor, so the
// wrapper's text color tints the icon. Assets are ISC-licensed (icons/LICENSE).
var nodeIconSVG = func() map[string]string {
	m := map[string]string{}
	entries, err := iconFS.ReadDir("icons")
	if err != nil {
		panic(err) // embedded dir is a compile-time constant; cannot fail at runtime
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".svg") {
			continue
		}
		b, err := iconFS.ReadFile("icons/" + e.Name())
		if err != nil {
			panic(err)
		}
		m[strings.TrimSuffix(e.Name(), ".svg")] = iconDim.ReplaceAllString(string(b), `$1="100%"`)
	}
	return m
}()

// NodeIconSVG returns the inline SVG for a whitelisted icon key ("" when unknown).
func NodeIconSVG(key string) string { return nodeIconSVG[key] }

// NodeIconCount reports how many icon assets are embedded (drift-guard seam).
func NodeIconCount() int { return len(nodeIconSVG) }
