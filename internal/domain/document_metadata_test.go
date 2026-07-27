package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestReclassifyDocumentMetadata_DailyIsCanonical(t *testing.T) {
	nodeID := "node-1"
	day := time.Date(2026, 7, 3, 14, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	doc := domain.Document{Type: domain.DocFree, NodeID: &nodeID, Path: "notes/old"}

	got, err := domain.ReclassifyDocumentMetadata(doc, domain.DocumentMetadata{
		Type: domain.DocDaily, NodeID: &nodeID, Path: "ignored/path", Date: &day,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "daily/2026-07-03" {
		t.Fatalf("Path = %q, want canonical daily path", got.Path)
	}
	if got.NodeID != nil {
		t.Fatalf("daily NodeID = %v, want nil", got.NodeID)
	}
	if got.Date == nil || !got.Date.Equal(day) {
		t.Fatalf("Date = %v, want %v", got.Date, day)
	}
}

func TestReclassifyDocumentMetadata_NonDailyClearsDate(t *testing.T) {
	oldDay := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	nodeID := "node-1"
	doc := domain.Document{Type: domain.DocDaily, Path: "daily/2026-07-03", Date: &oldDay}

	got, err := domain.ReclassifyDocumentMetadata(doc, domain.DocumentMetadata{
		Type: domain.DocFree, NodeID: &nodeID, Path: "notes/reclassified", Date: &oldDay,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Date != nil {
		t.Fatalf("non-daily Date = %v, want nil", got.Date)
	}
	if got.NodeID != nil {
		t.Fatalf("free NodeID = %v, want nil", got.NodeID)
	}
}

func TestReclassifyDocumentMetadata_ValidatesRequiredFields(t *testing.T) {
	doc := domain.Document{Type: domain.DocFree, Path: "notes/old"}

	_, err := domain.ReclassifyDocumentMetadata(doc, domain.DocumentMetadata{Type: domain.DocDaily})
	if !errors.Is(err, domain.ErrInvalidDocument) {
		t.Fatalf("daily without date: want ErrInvalidDocument, got %v", err)
	}

	_, err = domain.ReclassifyDocumentMetadata(doc, domain.DocumentMetadata{Type: domain.DocProject, Path: "readme"})
	if !errors.Is(err, domain.ErrInvalidDocument) {
		t.Fatalf("project without node: want ErrInvalidDocument, got %v", err)
	}

	_, err = domain.ReclassifyDocumentMetadata(doc, domain.DocumentMetadata{Type: domain.DocFree, Path: "Bad Path"})
	if !errors.Is(err, domain.ErrInvalidDocument) {
		t.Fatalf("invalid path: want ErrInvalidDocument, got %v", err)
	}
}
