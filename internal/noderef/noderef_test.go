package noderef_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/noderef"
)

func ref(s string) *string { return &s }

func nodes() []domain.Node {
	return []domain.Node{
		{ID: "eng-work", Slug: "work", Kind: domain.KindEngagement},
		{ID: "repo-work", Slug: "api", Kind: domain.KindRepo, ParentID: ref("eng-work")},
		{ID: "eng-private", Slug: "private", Kind: domain.KindEngagement},
		{ID: "repo-private", Slug: "api", Kind: domain.KindRepo, ParentID: ref("eng-private")},
	}
}

func TestResolveUsesIDAndQualifiedPathToDisambiguate(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		ref, want string
	}{
		{ref: "repo-private", want: "repo-private"},
		{ref: "work/api", want: "repo-work"},
		{ref: "/private/api", want: "repo-private"},
	} {
		t.Run(tt.ref, func(t *testing.T) {
			t.Parallel()
			got, err := noderef.Resolve(nodes(), tt.ref)
			if err != nil || got.ID != tt.want {
				t.Fatalf("Resolve(%q) = (%q, %v), want %q", tt.ref, got.ID, err, tt.want)
			}
		})
	}
}

func TestResolveRejectsAmbiguousBareSlugWithCandidatePaths(t *testing.T) {
	t.Parallel()
	_, err := noderef.Resolve(nodes(), "api")
	if !errors.Is(err, noderef.ErrAmbiguous) {
		t.Fatalf("Resolve(api) error = %v, want ErrAmbiguous", err)
	}
	for _, path := range []string{"private/api", "work/api"} {
		if !strings.Contains(err.Error(), path) {
			t.Errorf("ambiguity error %q missing path %q", err, path)
		}
	}
}

func TestResolveUnknownIsDistinctFromAmbiguous(t *testing.T) {
	t.Parallel()
	_, err := noderef.Resolve(nodes(), "missing")
	if !errors.Is(err, noderef.ErrNotFound) || errors.Is(err, noderef.ErrAmbiguous) {
		t.Fatalf("Resolve(missing) error = %v, want only ErrNotFound", err)
	}
}
