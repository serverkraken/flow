package domain_test

import (
	"reflect"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

func TestExtractLinks(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
		want []domain.Link
	}{
		{
			name: "no links",
			body: "plain text without any wikilinks",
			want: []domain.Link{},
		},
		{
			name: "single bare link",
			body: "see [[daily/2026-04-25]] for context",
			want: []domain.Link{{Target: "daily/2026-04-25"}},
		},
		{
			name: "single link with display",
			body: "see [[daily/2026-04-25|today]] for context",
			want: []domain.Link{{Target: "daily/2026-04-25", Display: "today"}},
		},
		{
			name: "multiple mixed links",
			body: "[[a]] and [[b|B]] and [[c]]",
			want: []domain.Link{
				{Target: "a"},
				{Target: "b", Display: "B"},
				{Target: "c"},
			},
		},
		{
			name: "newline inside brackets is not a link",
			body: "[[broken\nlink]] text",
			want: []domain.Link{},
		},
		{
			name: "single bracket pair is not a link",
			body: "[label](url) and [other]",
			want: []domain.Link{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := domain.ExtractLinks([]byte(tc.body))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}
