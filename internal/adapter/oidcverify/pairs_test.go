package oidcverify_test

import (
	"reflect"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/oidcverify"
)

func TestVerifierPairs(t *testing.T) {
	type IA = oidcverify.IssuerAudiences
	type Aud = oidcverify.Audience
	cases := []struct {
		name string
		in   oidcverify.PairConfig
		want []IA
	}{
		{
			name: "prod three distinct issuers → per-issuer audiences",
			in: oidcverify.PairConfig{
				WebIssuer: "https://id/o/flow/", WebClient: "flow",
				CLIIssuer: "https://id/o/flow-cli/", CLIClient: "flow-cli",
				MachineIssuer: "https://id/o/flow-machine/", MachineClient: "flow-machine",
			},
			want: []IA{
				{Issuer: "https://id/o/flow/", Audiences: []Aud{{ClientID: "flow"}}},
				{Issuer: "https://id/o/flow-cli/", Audiences: []Aud{{ClientID: "flow-cli"}}},
				{Issuer: "https://id/o/flow-machine/", Audiences: []Aud{{ClientID: "flow-machine", Machine: true}}},
			},
		},
		{
			name: "machine auth off → no machine audience anywhere",
			in: oidcverify.PairConfig{
				WebIssuer: "https://id/o/flow/", WebClient: "flow",
				CLIIssuer: "https://id/o/flow-cli/", CLIClient: "flow-cli",
			},
			want: []IA{
				{Issuer: "https://id/o/flow/", Audiences: []Aud{{ClientID: "flow"}}},
				{Issuer: "https://id/o/flow-cli/", Audiences: []Aud{{ClientID: "flow-cli"}}},
			},
		},
		{
			name: "dev empty cli issuer → one issuer accepts both clients",
			in: oidcverify.PairConfig{
				WebIssuer: "http://localhost:5556/dex", WebClient: "flow-dev",
				CLIIssuer: "", CLIClient: "flow-cli",
			},
			want: []IA{
				{Issuer: "http://localhost:5556/dex", Audiences: []Aud{
					{ClientID: "flow-dev"}, {ClientID: "flow-cli"},
				}},
			},
		},
		{
			name: "dev one issuer, three clients → machine flag survives the collapse",
			in: oidcverify.PairConfig{
				WebIssuer: "http://localhost:5556/dex", WebClient: "flow-dev",
				CLIIssuer: "http://localhost:5556/dex", CLIClient: "flow-cli",
				MachineIssuer: "http://localhost:5556/dex", MachineClient: "flow-machine",
			},
			want: []IA{
				{Issuer: "http://localhost:5556/dex", Audiences: []Aud{
					{ClientID: "flow-dev"}, {ClientID: "flow-cli"},
					{ClientID: "flow-machine", Machine: true},
				}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := oidcverify.VerifierPairs(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("VerifierPairs = %+v, want %+v", got, tc.want)
			}
		})
	}
}
