package handlers

// project_actions_internal_test.go — Plan E · Task 15.
//
// White-box unit tests for unexported helpers in project_actions.go.
// Black-box tests in project_actions_test.go cover the HTTP-handler
// path; this file covers branches that only fire under conditions a
// black-box test can't easily synthesise (concurrent INSERT collisions
// from the underlying sqlite UNIQUE constraint).

import (
	"errors"
	"testing"
)

func TestIsUniqueConstraintErr(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"unrelated error", errors.New("oops"), false},
		{
			"modernc sqlite UNIQUE failure",
			errors.New("constraint failed: UNIQUE constraint failed: projects.user_id, projects.slug (2067)"),
			true,
		},
		{
			"generic constraint failed",
			errors.New("constraint failed: NOT NULL constraint failed"),
			true,
		},
		{"unrelated UNIQUE-ish substring missing", errors.New("primary key violation"), false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isUniqueConstraintErr(tc.err); got != tc.want {
				t.Errorf("isUniqueConstraintErr(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestParseProjectVersion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"   ", 0},
		{"0", 0},
		{"1", 1},
		{"42", 42},
		{"  42  ", 42},
		{"abc", 0},
		{"-1", -1}, // ParseInt accepts negatives — caller treats it as a write-conflict candidate
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := parseProjectVersion(tc.in); got != tc.want {
				t.Errorf("parseProjectVersion(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
