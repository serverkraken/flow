package i18n_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/i18n"
)

func TestResolve_CookieWins(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: "flow_lang", Value: "en"})
	r.Header.Set("Accept-Language", "de")
	if got := i18n.Resolve(r); got != i18n.EN {
		t.Fatalf("cookie should win: got %q", got)
	}
}

func TestResolve_AcceptLanguageFallback(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept-Language", "en-US,en;q=0.9,de;q=0.5")
	if got := i18n.Resolve(r); got != i18n.EN {
		t.Fatalf("accept-language en should resolve EN: got %q", got)
	}
}

func TestResolve_DefaultGerman(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if got := i18n.Resolve(r); got != i18n.DE {
		t.Fatalf("default should be DE: got %q", got)
	}
}

func TestMiddleware_InjectsLocale(t *testing.T) {
	var seen i18n.Locale
	h := i18n.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = i18n.FromContext(r.Context())
	}))
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: "flow_lang", Value: "en"})
	h.ServeHTTP(httptest.NewRecorder(), r.WithContext(context.Background()))
	if seen != i18n.EN {
		t.Fatalf("middleware did not inject EN, got %q", seen)
	}
}
