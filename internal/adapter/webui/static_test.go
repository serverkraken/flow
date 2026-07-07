package webui_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui"
)

// TestAssetVersion_DeterministicNonEmpty — Lesesaal L4 Task 7: the hash must
// be stable across calls within one process (sync.Once) and never empty, so
// AssetURL always produces a cache-busted URL.
func TestAssetVersion_DeterministicNonEmpty(t *testing.T) {
	v1 := webui.AssetVersion()
	v2 := webui.AssetVersion()
	if v1 == "" {
		t.Fatal("AssetVersion() must not be empty")
	}
	if v1 != v2 {
		t.Fatalf("AssetVersion() not stable: %q != %q", v1, v2)
	}
}

func TestAssetURL_CarriesVersionQuery(t *testing.T) {
	u := webui.AssetURL("app.css")
	want := "/static/app.css?v=" + webui.AssetVersion()
	if u != want {
		t.Fatalf("AssetURL(%q) = %q, want %q", "app.css", u, want)
	}
}

// TestStaticHandler_SetsImmutableCacheControl — the PROD bug (2026-07-06):
// embed.FS reports a zero modtime, so http.FileServerFS sets neither
// Last-Modified nor ETag, and Cloudflare's edge cache serves up to 4h-stale
// CSS/JS against fresh HTML after a deploy. Fix: every /static/ response
// carries a long-lived, immutable Cache-Control — safe because the URL
// itself is cache-busted with "?v=<hash>" (AssetURL).
func TestStaticHandler_SetsImmutableCacheControl(t *testing.T) {
	ts := httptest.NewServer(http.StripPrefix("/static/", webui.StaticHandler()))
	t.Cleanup(ts.Close)

	res, err := http.Get(ts.URL + "/static/app.css?v=" + webui.AssetVersion())
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	const want = "public, max-age=31536000, immutable"
	if got := res.Header.Get("Cache-Control"); got != want {
		t.Errorf("Cache-Control = %q, want %q", got, want)
	}
}

func TestStaticHandlerServesVendorAndFonts(t *testing.T) {
	ts := httptest.NewServer(http.StripPrefix("/static/", webui.StaticHandler()))
	t.Cleanup(ts.Close)

	for _, p := range []string{
		"/static/app.css",
		"/static/vendor/htmx.min.js",
		"/static/vendor/htmx-ext-sse.js",
		"/static/fonts/SchibstedGrotesk-Variable.woff2",
		"/static/fonts/JetBrainsMono-Variable.woff2",
		"/static/vendor/mermaid.min.js",
		"/static/js/mermaid-init.js",
	} {
		res, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status %d, want 200", p, res.StatusCode)
		}
		if len(body) == 0 {
			t.Errorf("GET %s: empty body", p)
		}
	}
}
