package domain_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestStats_TopTags(t *testing.T) {
	mkStats := func(byTag map[string]time.Duration, count map[string]int) domain.Stats {
		return domain.Stats{ByTag: byTag, CountByTag: count}
	}

	tests := []struct {
		name string
		s    domain.Stats
		n    int
		want []domain.TagDur
	}{
		{
			name: "empty",
			s:    mkStats(map[string]time.Duration{}, map[string]int{}),
			n:    0,
			want: []domain.TagDur{},
		},
		{
			name: "excludes empty-tag bucket",
			s: mkStats(
				map[string]time.Duration{"deep": 2 * time.Hour, "": 30 * time.Minute},
				map[string]int{"deep": 2, "": 1},
			),
			n: 0,
			want: []domain.TagDur{
				{Tag: "deep", Total: 2 * time.Hour, Count: 2},
			},
		},
		{
			name: "sorts by duration desc",
			s: mkStats(
				map[string]time.Duration{"deep": 2 * time.Hour, "meet": 30 * time.Minute, "ops": time.Hour},
				map[string]int{"deep": 2, "meet": 3, "ops": 1},
			),
			n: 0,
			want: []domain.TagDur{
				{Tag: "deep", Total: 2 * time.Hour, Count: 2},
				{Tag: "ops", Total: time.Hour, Count: 1},
				{Tag: "meet", Total: 30 * time.Minute, Count: 3},
			},
		},
		{
			name: "ties broken by tag name ascending",
			s: mkStats(
				map[string]time.Duration{"zeta": time.Hour, "alpha": time.Hour, "mid": time.Hour},
				map[string]int{"zeta": 1, "alpha": 1, "mid": 1},
			),
			n: 0,
			want: []domain.TagDur{
				{Tag: "alpha", Total: time.Hour, Count: 1},
				{Tag: "mid", Total: time.Hour, Count: 1},
				{Tag: "zeta", Total: time.Hour, Count: 1},
			},
		},
		{
			name: "n limits result",
			s: mkStats(
				map[string]time.Duration{"a": 3 * time.Hour, "b": 2 * time.Hour, "c": time.Hour},
				map[string]int{"a": 1, "b": 1, "c": 1},
			),
			n: 2,
			want: []domain.TagDur{
				{Tag: "a", Total: 3 * time.Hour, Count: 1},
				{Tag: "b", Total: 2 * time.Hour, Count: 1},
			},
		},
		{
			name: "n larger than result is harmless",
			s: mkStats(
				map[string]time.Duration{"a": time.Hour},
				map[string]int{"a": 1},
			),
			n: 10,
			want: []domain.TagDur{
				{Tag: "a", Total: time.Hour, Count: 1},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.s.TopTags(tc.n)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("TopTags(%d) = %#v, want %#v", tc.n, got, tc.want)
			}
		})
	}
}
