package usecase_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestSetThenGetTags(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := testutil.NewFakeTagStore()
	set := usecase.SetTags{Tags: ts}
	get := usecase.GetTags{Tags: ts}
	if _, err := set.Execute(ctx, "u1", domain.TaggableNode, "n1", []string{"infra", "terraform"}); err != nil {
		t.Fatal(err)
	}
	got, err := get.Execute(ctx, "u1", domain.TaggableNode, "n1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 tags, got %+v", got)
	}
}
