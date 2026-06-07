package oidcserver

import "testing"

func TestUnit_AudienceAccepted(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		token    []string
		accepted []string
		want     bool
	}{
		{
			name:     "single token aud matches single accepted",
			token:    []string{"flow-web"},
			accepted: []string{"flow-web"},
			want:     true,
		},
		{
			name:     "CLI token accepted when browser+cli both allowed",
			token:    []string{"flow-cli"},
			accepted: []string{"flow-web", "flow-cli"},
			want:     true,
		},
		{
			name:     "browser token accepted when browser+cli both allowed",
			token:    []string{"flow-web"},
			accepted: []string{"flow-web", "flow-cli"},
			want:     true,
		},
		{
			name:     "token aud not in accepted list rejected",
			token:    []string{"some-other-app"},
			accepted: []string{"flow-web", "flow-cli"},
			want:     false,
		},
		{
			name:     "JWT with multi-valued aud passes if any value matches",
			token:    []string{"some-other-app", "flow-cli"},
			accepted: []string{"flow-web", "flow-cli"},
			want:     true,
		},
		{
			name:     "empty token aud rejected (defensive — well-formed JWTs always have aud)",
			token:    nil,
			accepted: []string{"flow-web"},
			want:     false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := audienceAccepted(tc.token, tc.accepted); got != tc.want {
				t.Errorf("audienceAccepted(%v, %v) = %v, want %v", tc.token, tc.accepted, got, tc.want)
			}
		})
	}
}

func TestUnit_NewProvider_RejectsEmptyAcceptedClientIDs(t *testing.T) {
	t.Parallel()
	// Empty accepted-list means "trust any audience" which is an unsafe
	// default — boot must fail loudly rather than silently disabling the
	// audience check.
	_, err := NewProvider(t.Context(), ProviderConfig{
		Issuers:           []string{"https://example.com"},
		AcceptedClientIDs: nil,
	})
	if err == nil {
		t.Fatal("NewProvider with empty AcceptedClientIDs: expected error, got nil")
	}
}
