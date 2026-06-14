package apiclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

func TestStartSessionAndListProjects(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing bearer")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/sessions":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"s1","start":"2026-06-14T09:00:00Z"}`))
		case "/api/v1/projects":
			_, _ = w.Write([]byte(`[{"id":"p1","name":"Flow","slug":"flow","status":"active"}]`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")

	s, err := c.StartSession(context.Background(), nil, "", "")
	if err != nil || s.ID != "s1" {
		t.Fatalf("StartSession = %+v err=%v", s, err)
	}
	ps, err := c.ListProjects(context.Background())
	if err != nil || len(ps) != 1 || ps[0].Name != "Flow" {
		t.Fatalf("ListProjects = %+v err=%v", ps, err)
	}
}

func TestEventsStream(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("event: session.started\ndata: {\"type\":\"session.started\",\"data\":{\"id\":\"s1\"}}\n\n"))
		w.(http.Flusher).Flush()
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := c.Events(ctx)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	ev := <-ch
	if ev.Type != "session.started" || ev.Data["id"] != "s1" {
		t.Fatalf("event = %+v", ev)
	}
}
