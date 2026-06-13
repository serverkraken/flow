// Package webui mounts the Phase-1 M6+M7 browser UI on top of flow-server.
// Templates are written in github.com/a-h/templ and pre-compiled to .go via
// `make webui-templ`; static assets (Tailwind output, Alpine, ApexCharts,
// CodeMirror) are vendored under internal/webui/static and embedded so the
// production binary needs no on-disk asset directory.
//
// Design system: see DESIGN.md ("Editorial Terminal" — Tokyonight, JetBrains
// Mono, status-spine, restrained motion).
package webui

import (
	"embed"
	"io/fs"
)

//go:embed static
var staticAssets embed.FS

// StaticFS returns the embedded /static directory rooted so that
// `http.FileServer(http.FS(StaticFS()))` serves `/static/foo.js`
// from `internal/webui/static/foo.js`.
func StaticFS() fs.FS {
	sub, err := fs.Sub(staticAssets, "static")
	if err != nil {
		// embed.FS.Sub never fails for a literal path that exists at
		// compile time, so a panic here means the //go:embed directive
		// drifted from the directory layout — fix the embed, not the call
		// site.
		panic("webui: static embed root missing: " + err.Error())
	}
	return sub
}
