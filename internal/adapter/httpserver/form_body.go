package httpserver

import (
	"errors"
	"mime"
	"net/http"
)

// parseWebFormBody validates and bounds every URL-encoded WebUI write before
// a handler can observe FormValue. Multipart uploads keep their tighter,
// route-specific readers and parsers.
func parseWebFormBody(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}
	contentType := r.Header.Get("Content-Type")
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			http.Error(w, "bad content type", http.StatusBadRequest)
			return false
		}
		if mediaType == "multipart/form-data" {
			return true
		}
		if mediaType != "application/x-www-form-urlencoded" {
			http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
			return false
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxWebFormBodyBytes)
	if err := r.ParseForm(); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return false
		}
		http.Error(w, "bad request", http.StatusBadRequest)
		return false
	}
	return true
}
