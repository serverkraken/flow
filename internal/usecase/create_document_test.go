package usecase_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestCreateDocument_ExplicitTagsParam(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	docs := testutil.NewFakeDocumentStore()
	tags := testutil.NewFakeTagStore()
	uc := usecase.CreateDocument{Docs: docs, Tags: tags, IDs: &testutil.FakeIDGen{}, Clock: testutil.FakeClock{T: time.Now()}}
	got, err := uc.Execute(ctx, "u1", usecase.CreateDocumentInput{
		Type: domain.DocFree, Path: "p", Title: "T", Body: "pure content, no frontmatter",
		Tags: []string{"Go", "tui"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"go", "tui"}
	if !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("response tags = %v, want %v", got.Tags, want)
	}
	stored, _ := tags.TagsFor(ctx, "u1", domain.TaggableDocument, got.ID)
	if len(stored) != 2 {
		t.Fatalf("taggings want 2, got %+v", stored)
	}
}
