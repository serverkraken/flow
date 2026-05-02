package domain_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

func TestSearchQuery_IsEmpty(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		q    domain.SearchQuery
		want bool
	}{
		{"zero value", domain.SearchQuery{}, true},
		{"limit only is empty", domain.SearchQuery{Limit: 50}, true},
		{"order only is empty", domain.SearchQuery{Order: domain.OrderRecent}, true},
		{"with text", domain.SearchQuery{Text: "foo"}, false},
		{"with type", domain.SearchQuery{Type: domain.TypeDaily}, false},
		{"with project", domain.SearchQuery{Project: "github.com/foo/bar"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.q.IsEmpty(); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
