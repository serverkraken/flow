package i18n

import (
	"net/http"

	"golang.org/x/text/language"
)

// supported lists the locales we can serve, most-preferred first; the matcher
// maps an Accept-Language header onto one of these.
var supported = []language.Tag{
	language.German,  // de — index 0 → also the matcher's default
	language.English, // en
}

var matcher = language.NewMatcher(supported)

// Resolve picks the request locale: flow_lang cookie → Accept-Language → Default.
func Resolve(r *http.Request) Locale {
	if c, err := r.Cookie("flow_lang"); err == nil {
		switch Locale(c.Value) {
		case DE:
			return DE
		case EN:
			return EN
		}
	}
	if al := r.Header.Get("Accept-Language"); al != "" {
		tag, _ := language.MatchStrings(matcher, al)
		base, _ := tag.Base()
		switch base.String() {
		case "en":
			return EN
		case "de":
			return DE
		}
	}
	return Default
}

// Middleware injects the resolved locale into the request context for T/Tn.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(WithLocale(r.Context(), Resolve(r))))
	})
}
