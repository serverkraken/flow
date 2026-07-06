package webui_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui"
)

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
