package httpserver

import (
	"net/http"
	"net/url"
	"strings"
)

// csrfCookieWrite protects standalone cookie-mutating routes that do not need
// webAuth (currently logout). webAuth and authAny enforce the same check after
// selecting cookie authentication, so bearer-authenticated API writes remain
// independent of browser Origin headers.
func (s *Server) csrfCookieWrite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.validCookieWriteSource(r) {
			http.Error(w, "forbidden request origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) validCookieWriteSource(r *http.Request) bool {
	if isSafeMethod(r.Method) {
		return true
	}
	// Config.Load requires PublicBaseURL and cmd/flow-server wires it. Keeping
	// zero-value Servers permissive avoids turning every focused handler unit
	// test into configuration boilerplate without weakening the real binary.
	if strings.TrimSpace(s.PublicBaseURL) == "" {
		return true
	}
	want, ok := parseHTTPOrigin(s.PublicBaseURL)
	if !ok {
		return false
	}

	if origins := r.Header.Values("Origin"); len(origins) > 0 {
		if len(origins) != 1 {
			return false
		}
		got, valid := parseHTTPOrigin(origins[0])
		return valid && got == want
	}
	if referers := r.Header.Values("Referer"); len(referers) > 0 {
		if len(referers) != 1 {
			return false
		}
		got, valid := parseHTTPOrigin(referers[0])
		return valid && got == want
	}
	return false
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

type httpOrigin struct {
	scheme string
	host   string
	port   string
}

func parseHTTPOrigin(raw string) (httpOrigin, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.User != nil || u.Host == "" {
		return httpOrigin{}, false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return httpOrigin{}, false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return httpOrigin{}, false
	}
	port := u.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return httpOrigin{scheme: scheme, host: host, port: port}, true
}
