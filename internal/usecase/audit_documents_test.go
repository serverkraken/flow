package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestAuditDocuments_ReportsActiveAndArchivedAnomaliesWithoutMutation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	nodes := testutil.NewFakeNodeStore()
	if _, err := nodes.Create(ctx, domain.Node{ID: "owned", OwnerID: "u1", Kind: domain.KindEngagement, Name: "Owned", Slug: "owned", Status: domain.NodeActive}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	foreign := "foreign"
	if _, err := nodes.Create(ctx, domain.Node{ID: foreign, OwnerID: "u2", Kind: domain.KindEngagement, Name: "Foreign", Slug: "foreign", Status: domain.NodeActive}); err != nil {
		t.Fatalf("seed foreign node: %v", err)
	}
	day := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	seedAuditDoc(t, docs, domain.Document{ID: "ok", OwnerID: "u1", Type: domain.DocFree, Path: "notes/ok", Title: "OK"})
	seedAuditDoc(t, docs, domain.Document{ID: "wrong-type", OwnerID: "u1", Type: domain.DocFree, Path: "daily/2026-07-13", Title: "Wrong type"})
	seedAuditDoc(t, docs, domain.Document{ID: "daily", OwnerID: "u1", NodeID: strPtr("owned"), Type: domain.DocDaily, Path: "daily/wrong", Date: &day, Title: "Daily"})
	seedAuditDoc(t, docs, domain.Document{ID: "project", OwnerID: "u1", NodeID: &foreign, Type: domain.DocProject, Path: "projects/foreign", Title: "Project"})
	seedAuditDoc(t, docs, domain.Document{ID: "legacy", OwnerID: "u1", Type: domain.DocAgent, Path: "specs/legacy", Title: "Legacy"})
	seedAuditDoc(t, docs, domain.Document{ID: "archived", OwnerID: "u1", Type: domain.DocFree, Path: "notes/archived", Title: "Archived", Archived: true, Pinned: true})

	report, err := (usecase.AuditDocuments{Docs: docs, Nodes: nodes}).Execute(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if report.Scanned != 6 || report.Active != 5 || report.Archived != 1 {
		t.Fatalf("unexpected counts: %+v", report)
	}
	wantCodes := map[string]bool{
		"daily_path_mismatch":        false,
		"daily_path_on_non_daily":    false,
		"daily_with_project":         false,
		"node_not_owned_or_missing":  false,
		"deprecated_agent_type":      false,
		"archived_and_pinned":        false,
		"archived_missing_timestamp": false,
	}
	for _, issue := range report.Issues {
		if _, ok := wantCodes[issue.Code]; ok {
			wantCodes[issue.Code] = true
		}
	}
	for code, found := range wantCodes {
		if !found {
			t.Errorf("missing issue %q in %+v", code, report.Issues)
		}
	}
	active, _ := docs.List(ctx, "u1", nil)
	archived, _ := docs.ListArchived(ctx, "u1")
	if len(active) != 5 || len(archived) != 1 || !archived[0].Pinned {
		t.Fatalf("audit mutated documents: active=%d archived=%+v", len(active), archived)
	}
}

func seedAuditDoc(t *testing.T, docs *testutil.FakeDocumentStore, doc domain.Document) {
	t.Helper()
	if _, err := docs.Create(context.Background(), doc); err != nil {
		t.Fatalf("seed document %s: %v", doc.ID, err)
	}
}

func strPtr(v string) *string { return &v }
