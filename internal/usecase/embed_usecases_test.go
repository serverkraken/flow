package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

type spyNotifier struct{ n int }

func (s *spyNotifier) DocumentChanged() { s.n++ }

func TestRetryEmbedding_ClearsAndKicks(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	_, _ = docs.Create(ctx, domain.Document{ID: "d", OwnerID: "u", Type: domain.DocFree, Path: "d", Title: "D", Body: "b"})
	_ = docs.RecordEmbedFailure(ctx, "d", "u", docs.SnapshotHash("d"), 5, time.Now(), true, "boom")
	spy := &spyNotifier{}

	uc := usecase.RetryEmbedding{Docs: docs, Notifier: spy}
	if err := uc.Execute(ctx, "u", "d"); err != nil {
		t.Fatal(err)
	}
	if spy.n != 1 {
		t.Fatalf("want notifier kicked once, got %d", spy.n)
	}
	if s, _ := docs.EmbedStatus(ctx, "u", "d"); s.State != domain.EmbedPending {
		t.Fatalf("after retry want pending, got %v", s.State)
	}
}

func TestRetryEmbedding_UnknownDoc(t *testing.T) {
	uc := usecase.RetryEmbedding{Docs: testutil.NewFakeDocumentStore(), Notifier: &spyNotifier{}}
	if err := uc.Execute(context.Background(), "u", "nope"); err == nil || !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Fatalf("want ErrDocumentNotFound, got %v", err)
	}
}

func TestGetEmbedStatus_PassThrough(t *testing.T) {
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	_, _ = docs.Create(ctx, domain.Document{ID: "d", OwnerID: "u", Type: domain.DocFree, Path: "d", Title: "D", Body: "b"})
	uc := usecase.GetEmbedStatus{Docs: docs}
	s, err := uc.Execute(ctx, "u", "d")
	if err != nil || s.State != domain.EmbedPending {
		t.Fatalf("want pending, got %v err=%v", s.State, err)
	}
}
