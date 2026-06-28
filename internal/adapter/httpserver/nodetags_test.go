package httpserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestNodeTags_SetThenGet(t *testing.T) {
	t.Parallel()
	srv, _ := newDocServer(t)
	tags := testutil.NewFakeTagStore()
	srv.SetTags = usecase.SetTags{Tags: tags}
	srv.GetTags = usecase.GetTags{Tags: tags}

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)

	// PUT tags on node "n1"
	res := doDoc(t, ts, "PUT", "/api/v1/nodes/n1/tags", `{"tags":["infra","terraform"]}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT want 200, got %d", res.StatusCode)
	}
	_ = res.Body.Close()

	res = doDoc(t, ts, "GET", "/api/v1/nodes/n1/tags", "")
	defer func() { _ = res.Body.Close() }()
	var got []domain.Tag
	_ = json.NewDecoder(res.Body).Decode(&got)
	if len(got) != 2 {
		t.Fatalf("want 2 node tags, got %+v", got)
	}
}

func TestNodeTags_GetEmpty(t *testing.T) {
	t.Parallel()
	srv, _ := newDocServer(t)
	tags := testutil.NewFakeTagStore()
	srv.SetTags = usecase.SetTags{Tags: tags}
	srv.GetTags = usecase.GetTags{Tags: tags}

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	primeUser(t, ts.URL)

	res := doDoc(t, ts, "GET", "/api/v1/nodes/n-unknown/tags", "")
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET empty want 200, got %d", res.StatusCode)
	}
	var got []domain.Tag
	_ = json.NewDecoder(res.Body).Decode(&got)
	if got == nil {
		t.Fatal("want non-nil empty slice, got nil")
	}
	if len(got) != 0 {
		t.Fatalf("want 0 tags for unknown node, got %+v", got)
	}
}
