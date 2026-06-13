// Package settings renders the WebUI settings surface at `/settings`.
// All data resolution happens in the handler; templates only render
// formatted strings off a flat view-model.
//
// M6 is read-only — Phase 2 will wire device telemetry, sync state,
// bearer-token management, and export/import. The "Abmelden" form
// posts to /logout, handled by the M1 browser-auth middleware.
package settings
