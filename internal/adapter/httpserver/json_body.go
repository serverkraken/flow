package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const (
	maxJSONBodyBytes         int64 = 64 * 1024
	maxDocumentJSONBodyBytes int64 = 2 * 1024 * 1024
	maxWebFormBodyBytes      int64 = 2 * 1024 * 1024
)

// decodeJSONBody decodes exactly one JSON value, rejects unknown fields and
// bounds the body before decoding. allowEmpty treats an empty body like {}.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64, allowEmpty bool) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		if allowEmpty && errors.Is(err, io.EOF) {
			return true
		}
		writeJSONDecodeError(w, err)
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSONDecodeError(w, err)
		return false
	}
	return true
}

func writeJSONDecodeError(w http.ResponseWriter, err error) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, "bad request", http.StatusBadRequest)
}
