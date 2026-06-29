package httpserver_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestHandleRedesignDocTypes(t *testing.T) {
	// Build the server using the shared doc-server helper and bolt on the
	// RedesignDocTypes usecase with its own FakeDocumentStore so we can
	// seed legacy agent docs directly (type `agent` is accepted but bypasses
	// the create-document validation path).
	srv, _ := newDocServer(t)
	myDocs := testutil.NewFakeDocumentStore()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)}
	srv.RedesignDocTypes = usecase.RedesignDocTypes{Docs: myDocs, Clock: clk}

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// Prime EnsureUser so user "id-1" is created (FakeIDGen starts at 0 → first
	// call returns "id-1").
	primeUser(t, ts.URL)

	// Seed two legacy agent docs directly under owner "id-1".
	ctx := context.Background()
	for _, d := range []domain.Document{
		{ID: "d1", OwnerID: "id-1", Type: domain.DocAgent, Path: "plans/p"},
		{ID: "d2", OwnerID: "id-1", Type: domain.DocAgent, Path: "specs/s-design"},
	} {
		if _, err := myDocs.Create(ctx, d); err != nil {
			t.Fatalf("seed doc %s: %v", d.ID, err)
		}
	}

	res := doDoc(t, ts, "POST", "/api/v1/maintenance/redesign-doctypes?dry_run=true", "")
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)
	var rep domain.RedesignReport
	if err := json.Unmarshal(body, &rep); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if rep.Scanned != 2 {
		t.Fatalf("scanned=%d want 2", rep.Scanned)
	}
}
