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
	ReposIndex  http.Handler
	RepoNote    http.Handler
	Projects    http.Handler
	Settings    http.Handler
	AuthLanding http.Handler
	StaticFS    fs.FS

	// R1 — documents-backed notes surface.
	DocumentsIndex http.Handler
	DocumentView   http.Handler
	DocumentEdit   http.Handler
	DocumentPut    http.Handler

	// M7 — Plan E · Task 11. HTMX write surface for the worktime today
	// tab. Each handler returns a templ partial fragment; HTMX swaps
	// the targeted DOM node in-place. All five share the same
	// SessionActionsDeps bag in cmd/flow-server/main.go.
	SessionEdit   http.Handler
	SessionPut    http.Handler
	SessionDelete http.Handler
	ActiveStart   http.Handler
	ActiveStop    http.Handler

	// R1 — Pause-Statemachine im Today-Banner.
	ActivePause  http.Handler
	ActiveResume http.Handler

	// M7 — Plan E · Task 12. repo-notes (DB-synced with OCC).
	RepoNoteEdit http.Handler
	RepoNotePut  http.Handler

	// M7 — Plan E · Task 13. HTMX write surface for the /projects
	// page: create new projects via inline form, rename via inline
	// per-row form, soft-delete (archive) via per-row button. Each
	// handler returns a templ partial fragment; HTMX swaps the
	// targeted DOM node in-place. All six share the same
	// ProjectActionsDeps bag in cmd/flow-server/main.go.
	ProjectNewForm   http.Handler
	ProjectNewCancel http.Handler
	ProjectCreate    http.Handler
	ProjectEdit      http.Handler
	ProjectPut       http.Handler
	ProjectArchive   http.Handler

	// M7 — Plan E · Task 14. Server-Sent-Events stream for live
	// dashboard updates. Mounted at GET /api/v1/events — dual
	// Bearer+Cookie auth so both browser and TUI/MCP can subscribe.
	Events http.Handler
}
