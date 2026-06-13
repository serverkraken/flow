// internal/adapter/httpserver/dayoffs_settings_api_test.go
package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
)

func newMiscAPIEnv(t *testing.T, sub string) chi.Router {
	t.Helper()
	u, err := pgstore.NewUsers(pgTestStore).EnsureBySub(sub, sub+"@test.de", sub)
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(WithUser(req.Context(), u)))
		})
	})
	MountDayOffsSettingsAPI(r, DayOffsSettingsAPIDeps{
		DayOffs:  pgstore.NewDayOffs(pgTestStore),
		Settings: pgstore.NewSettings(pgTestStore),
	})
	return r
}

func miscReq(t *testing.T, r chi.Router, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestDayOffsAPI_PutListDelete(t *testing.T) {
	r := newMiscAPIEnv(t, "api-dayoff-1")

	rec := miscReq(t, r, "PUT", "/day-offs/2026-06-15", `{"kind":"vacation","label":"Sommer"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}
	// ungültiger Kind → 422
	rec = miscReq(t, r, "PUT", "/day-offs/2026-06-16", `{"kind":"feiertag?"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad kind: want 422, got %d", rec.Code)
	}
	// ungültiges Datum → 422
	rec = miscReq(t, r, "PUT", "/day-offs/morgen", `{"kind":"sick"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad date: want 422, got %d", rec.Code)
	}

	rec = miscReq(t, r, "GET", "/day-offs?year=2026", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var page struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if len(page.Items) != 1 || page.Items[0]["kind"] != "vacation" {
		t.Fatalf("list payload: %v", page.Items)
	}

	rec = miscReq(t, r, "DELETE", "/day-offs/2026-06-15", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d", rec.Code)
	}
}

func TestSettingsAPI_GetPut(t *testing.T) {
	r := newMiscAPIEnv(t, "api-settings-1")

	rec := miscReq(t, r, "GET", "/settings", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get empty: %d", rec.Code)
	}

	rec = miscReq(t, r, "PUT", "/settings", `{"daily_target":"7h30m","timezone":"Europe/Berlin"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}
	// kaputte Zeitzone → 422
	rec = miscReq(t, r, "PUT", "/settings", `{"timezone":"Nicht/Existent"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad tz: want 422, got %d", rec.Code)
	}

	rec = miscReq(t, r, "GET", "/settings", "")
	var got map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["daily_target"] != "7h30m" || got["timezone"] != "Europe/Berlin" {
		t.Errorf("roundtrip: %v", got)
	}
}
