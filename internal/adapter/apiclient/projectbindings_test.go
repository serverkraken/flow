package apiclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

func TestResolveProject_200(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/nodes/resolve" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"p1","name":"Flow","slug":"flow-repo","status":"active"}`))
	}))
	defer ts.Close()

	c := apiclient.New(ts.URL, "tok")
	p, ok, err := c.ResolveNode(context.Background(), "flow-repo", "mac1", "/home/user/flow")
	if err != nil {
		t.Fatalf("ResolveNode: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if p.ID != "p1" || p.Slug != "flow-repo" {
		t.Fatalf("project = %+v", p)
	}
	for _, want := range []string{"slug=flow-repo", "machine=mac1"} {
		if !containsStr(gotQuery, want) {
			t.Errorf("query %q missing %q", gotQuery, want)
		}
	}
}

func TestResolveProject_404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/nodes/resolve" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	c := apiclient.New(ts.URL, "tok")
	p, ok, err := c.ResolveNode(context.Background(), "no-such", "mac1", "/tmp")
	if err != nil {
		t.Fatalf("ResolveNode 404 should not error, got: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false on 404")
	}
	if p.ID != "" {
		t.Fatalf("expected zero Project, got %+v", p)
	}
}

func TestBindRemote(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/nodes/p1/bindings" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"b1","projectId":"p1","kind":"remote","remoteSlug":"flow-repo"}`))
	}))
	defer ts.Close()

	c := apiclient.New(ts.URL, "tok")
	b, err := c.BindRemote(context.Background(), "p1", "flow-repo")
	if err != nil {
		t.Fatalf("BindRemote: %v", err)
	}
	if b.ID != "b1" || b.Kind != domain.BindingRemote || b.RemoteSlug != "flow-repo" {
		t.Fatalf("binding = %+v", b)
	}
	if gotBody["kind"] != "remote" || gotBody["remoteSlug"] != "flow-repo" {
		t.Fatalf("body = %+v", gotBody)
	}
}

func TestUnbindRemote(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/nodes/bindings" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	c := apiclient.New(ts.URL, "tok")
	if err := c.UnbindRemote(context.Background(), "flow-repo"); err != nil {
		t.Fatalf("UnbindRemote: %v", err)
	}
	for _, want := range []string{"kind=remote", "slug=flow-repo"} {
		if !containsStr(gotQuery, want) {
			t.Errorf("query %q missing %q", gotQuery, want)
		}
	}
}

func TestListBindings(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/nodes/bindings" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"b1","projectId":"p1","kind":"remote","remoteSlug":"flow-repo"}]`))
	}))
	defer ts.Close()

	c := apiclient.New(ts.URL, "tok")
	bs, err := c.ListBindings(context.Background())
	if err != nil {
		t.Fatalf("ListBindings: %v", err)
	}
	if len(bs) != 1 || bs[0].ID != "b1" || bs[0].Kind != domain.BindingRemote {
		t.Fatalf("bindings = %+v", bs)
	}
}

func TestBindPath(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/nodes/p1/bindings" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"b2","projectId":"p1","kind":"path","machineId":"mac1","machineLabel":"my-mac","path":"/home/user/flow"}`))
	}))
	defer ts.Close()

	c := apiclient.New(ts.URL, "tok")
	b, err := c.BindPath(context.Background(), "p1", "mac1", "my-mac", "/home/user/flow")
	if err != nil {
		t.Fatalf("BindPath: %v", err)
	}
	if b.ID != "b2" || b.Kind != domain.BindingPath || b.MachineID != "mac1" {
		t.Fatalf("binding = %+v", b)
	}
	if gotBody["kind"] != "path" || gotBody["machineId"] != "mac1" ||
		gotBody["machineLabel"] != "my-mac" || gotBody["path"] != "/home/user/flow" {
		t.Fatalf("body = %+v", gotBody)
	}
}

func TestUnbindPath(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/nodes/bindings" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	c := apiclient.New(ts.URL, "tok")
	if err := c.UnbindPath(context.Background(), "mac1", "/home/user/flow"); err != nil {
		t.Fatalf("UnbindPath: %v", err)
	}
	for _, want := range []string{"kind=path", "machine=mac1", "path=%2Fhome%2Fuser%2Fflow"} {
		if !containsStr(gotQuery, want) {
			t.Errorf("query %q missing %q", gotQuery, want)
		}
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStrHelper(s, substr))
}

func containsStrHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
