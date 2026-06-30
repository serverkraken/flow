package apiclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

func TestTagTimes_ReturnsSlice(t *testing.T) {
	mux, base := newMux(t)
	mux.HandleFunc("GET /api/v1/sessions/tag-times", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.TagTime{
			{Tag: "dev", Minutes: 90},
		})
	})
	c := apiclient.New(base, "tok")
	out, err := c.TagTimes(context.Background(), time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Tag != "dev" {
		t.Fatalf("unexpected %+v", out)
	}
}

func TestListArchived_ReturnsSlice(t *testing.T) {
	mux, base := newMux(t)
	mux.HandleFunc("GET /api/v1/documents/archived", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Document{
			{ID: "d1", Title: "old"},
		})
	})
	c := apiclient.New(base, "tok")
	out, err := c.ListArchived(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].ID != "d1" {
		t.Fatalf("unexpected %+v", out)
	}
}

func TestStripFrontmatter_DryRun(t *testing.T) {
	mux, base := newMux(t)
	mux.HandleFunc("POST /api/v1/maintenance/strip-frontmatter", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.StripReport{Scanned: 3, Stripped: 1})
	})
	c := apiclient.New(base, "tok")
	rep, err := c.StripFrontmatter(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scanned != 3 || rep.Stripped != 1 {
		t.Fatalf("unexpected %+v", rep)
	}
}

func TestNodeTags_ReturnsSlice(t *testing.T) {
	mux, base := newMux(t)
	mux.HandleFunc("GET /api/v1/nodes/{id}/tags", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Tag{{ID: "t1", Slug: "backend"}})
	})
	c := apiclient.New(base, "tok")
	tags, err := c.NodeTags(context.Background(), "n1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0].Slug != "backend" {
		t.Fatalf("unexpected %+v", tags)
	}
}

func TestSetNodeTags_ReturnsUpdated(t *testing.T) {
	mux, base := newMux(t)
	mux.HandleFunc("PUT /api/v1/nodes/{id}/tags", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Tag{{ID: "t2", Slug: "go"}})
	})
	c := apiclient.New(base, "tok")
	tags, err := c.SetNodeTags(context.Background(), "n1", []string{"go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0].Slug != "go" {
		t.Fatalf("unexpected %+v", tags)
	}
}

func TestRedesignDocTypes_DryRun(t *testing.T) {
	mux, base := newMux(t)
	mux.HandleFunc("POST /api/v1/maintenance/redesign-doctypes", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.RedesignReport{Scanned: 5, Converted: 2})
	})
	c := apiclient.New(base, "tok")
	rep, err := c.RedesignDocTypes(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scanned != 5 || rep.Converted != 2 {
		t.Fatalf("unexpected %+v", rep)
	}
}
