package apiclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

func TestClient_AddAndListDayOffs(t *testing.T) {
	var gotPost bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/dayoffs":
			gotPost = true
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/dayoffs":
			_, _ = w.Write([]byte(`[{"day":"2026-06-15","kind":"vacation","label":"Sommer","targetMin":0,"holiday":false}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)

	c := apiclient.New(ts.URL, "tok")
	if err := c.AddDayOffs(context.Background(), "2026-06-15", "2026-06-19", "vacation", "Sommer", 0, true); err != nil {
		t.Fatalf("add: %v", err)
	}
	if !gotPost {
		t.Fatal("POST not issued")
	}
	list, err := c.ListDayOffs(context.Background(), "2026-06-01", "2026-06-30")
	if err != nil || len(list) != 1 || list[0].Label != "Sommer" {
		t.Fatalf("list = %+v err=%v", list, err)
	}
}
