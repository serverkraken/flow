package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestTagTimeReport(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	// FakeSessionStore.TagTimes returns a deterministic fixture (empty store -> empty slice)
	uc := usecase.TagTimeReport{Sessions: ss}
	_, err := uc.Execute(ctx, "u1", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
}
