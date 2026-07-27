package domain_test

import (
	"reflect"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestNormalizeTag(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in       string
		wantSlug string
		wantOK   bool
	}{
		{"Django", "django", true},
		{"  Postgres  ", "postgres", true},
		{"TF", "tf", true},
		{"", "", false},
		{"   ", "", false},
		{"lang/python", "lang/python", true},
	}
	for _, c := range cases {
		slug, ok := domain.NormalizeTag(c.in)
		if slug != c.wantSlug || ok != c.wantOK {
			t.Errorf("NormalizeTag(%q) = (%q,%v), want (%q,%v)", c.in, slug, ok, c.wantSlug, c.wantOK)
		}
	}
}

func TestNormalizeTags_DedupLowerFirstSeen(t *testing.T) {
	t.Parallel()
	got := domain.NormalizeTags([]string{"Go", "go", " TUI ", "", "Go"})
	want := []string{"go", "tui"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NormalizeTags = %v, want %v", got, want)
	}
}

func TestTaggableType_Values(t *testing.T) {
	t.Parallel()
	if domain.TaggableDocument != "document" || domain.TaggableNode != "node" || domain.TaggableWorkSession != "work_session" {
		t.Errorf("taggable type string values drifted: %q %q %q",
			domain.TaggableDocument, domain.TaggableNode, domain.TaggableWorkSession)
	}
}
