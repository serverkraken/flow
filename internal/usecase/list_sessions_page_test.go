package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestListSessionsPage(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	for i := 0; i < 5; i++ {
		st := time.Date(2026, 6, 15, 8+i, 0, 0, 0, time.UTC)
		sp := st.Add(time.Hour)
		if _, err := ss.Create(ctx, domain.WorkSession{
			ID: "s" + string(rune('0'+i)), OwnerID: "u1", Start: st, Stop: &sp}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	uc := usecase.ListSessionsPage{Sessions: ss}
	items, total, err := uc.Execute(ctx, "u1", 2, 0)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if total != 5 || len(items) != 2 {
		t.Fatalf("got total=%d len=%d, want 5 and 2", total, len(items))
	}
}
