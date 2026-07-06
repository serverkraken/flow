package components_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

// TestAssetURL_WithoutVersion_FallsBackToBarePath — a components test that
// never calls SetAssetVersion (e.g. never went through
// httpserver.Server.Routes()) must still get a servable URL, just without
// the cache-busting query.
func TestAssetURL_WithoutVersion_FallsBackToBarePath(t *testing.T) {
	components.SetAssetVersion("")
	if got, want := components.AssetURL("app.css"), "/static/app.css"; got != want {
		t.Fatalf("AssetURL() = %q, want %q", got, want)
	}
}

// TestAssetURL_WithVersion_CarriesQuery — Lesesaal L4 Task 7: once
// SetAssetVersion has run (httpserver.Server.Routes(), once at startup with
// webui.AssetVersion()), every components-rendered /static/ URL must carry
// "?v=<hash>" so Cloudflare's long-lived Cache-Control never serves a stale
// asset against fresh HTML after a deploy.
func TestAssetURL_WithVersion_CarriesQuery(t *testing.T) {
	components.SetAssetVersion("abc123")
	t.Cleanup(func() { components.SetAssetVersion("") })

	if got, want := components.AssetURL("app.css"), "/static/app.css?v=abc123"; got != want {
		t.Fatalf("AssetURL() = %q, want %q", got, want)
	}
	if got := components.AssetVersion(); got != "abc123" {
		t.Fatalf("AssetVersion() = %q, want %q", got, "abc123")
	}
}
