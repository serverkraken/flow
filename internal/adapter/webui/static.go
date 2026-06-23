package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:static
var staticFS embed.FS

// StaticHandler serves the embedded static assets (mount under /static/).
func StaticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err) // embedded path is a compile-time constant; cannot fail at runtime
	}
	return http.FileServerFS(sub)
}
