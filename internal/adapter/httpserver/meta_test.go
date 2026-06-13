// internal/adapter/httpserver/meta_test.go
package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetaHandler(t *testing.T) {
	t.Parallel()
	h := NewMetaHandler(MetaResponse{ServerVersion: "1.2.3-test", MinClientVersion: "0.0.0"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
	var got MetaResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got.ServerVersion != "1.2.3-test" || got.MinClientVersion != "0.0.0" {
		t.Errorf("payload: %+v", got)
	}
}
