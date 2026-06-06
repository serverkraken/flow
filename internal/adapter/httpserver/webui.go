// webui.go declares the WebUI handler bag that AuthDeps.WebUI carries.
// The type lives in this package (not in internal/webui) to avoid a
// circular import: internal/webui/handlers imports httpserver for the
// UserFromContext helper, so httpserver cannot import internal/webui in
// return. The composition root in cmd/flow-server constructs the bag
// from the webui/handlers constructors and passes it in.
//
// WebUI may be nil — the WebUI route group is gated on `if d.WebUI != nil`
// so the existing /api/v1/* bearer routes still work when the WebUI is
// not configured (e.g. headless smoke tests).
package httpserver

import (
	"io/fs"
	"net/http"
)

// WebUIHandlers is the bag of every WebUI route handler. Each field
// being nil means "this route is not mounted" — the route registration
// in NewWithAuth skips a handler whose field is nil so partial wiring
// doesn't crash the server.
//
// StaticFS exposes the embedded /static directory; the server mounts it
// under /static/* via http.FileServer.
type WebUIHandlers struct {
	Dashboard   http.Handler
	Worktime    http.Handler
	NotesIndex  http.Handler
	NotesView   http.Handler
	ReposIndex  http.Handler
	RepoNote    http.Handler
	Projects    http.Handler
	Settings    http.Handler
	AuthLanding http.Handler
	StaticFS    fs.FS
}
