package oidcverify_test

import (
	"reflect"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/oidcverify"
)

func TestVerifierPairs(t *testing.T) {
	type IA = oidcverify.IssuerAudiences
	cases := []struct {
		name                                       string
		webIssuer, webClient, cliIssuer, cliClient string
		want                                       []IA
	}{
		{
			name:      "prod two distinct issuers → per-issuer audiences",
			webIssuer: "https://id/o/flow/", webClient: "flow",
			cliIssuer: "https://id/o/flow-cli/", cliClient: "flow-cli",
			want: []IA{
				{Issuer: "https://id/o/flow/", Audiences: []string{"flow"}},
				{Issuer: "https://id/o/flow-cli/", Audiences: []string{"flow-cli"}},
			},
		},
		{
			name:      "dev empty cli issuer → one issuer accepts both clients",
			webIssuer: "http://localhost:5556/dex", webClient: "flow-dev",
			cliIssuer: "", cliClient: "flow-cli",
			want: []IA{
				{Issuer: "http://localhost:5556/dex", Audiences: []string{"flow-dev", "flow-cli"}},
			},
		},
		{
			name:      "explicit equal issuers → collapse to one with both audiences",
			webIssuer: "http://localhost:5556/dex", webClient: "flow-dev",
			cliIssuer: "http://localhost:5556/dex", cliClient: "flow-cli",
			want: []IA{
				{Issuer: "http://localhost:5556/dex", Audiences: []string{"flow-dev", "flow-cli"}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := oidcverify.VerifierPairs(tc.webIssuer, tc.webClient, tc.cliIssuer, tc.cliClient)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("VerifierPairs = %+v, want %+v", got, tc.want)
			}
		})
	}
}
