package oidcverify

// VerifierPairs derives issuer→audience bindings from flow's OIDC config.
//
// Prod (Authentik per_provider): webIssuer and cliIssuer differ, so each issuer
// is bound to ITS OWN client only — a web-issued token may not carry the CLI
// audience and vice-versa (per-issuer tightness).
//
// Dev (a single Dex issuer fronting both clients): cliIssuer is empty, or equal
// to webIssuer, so one issuer is returned that accepts BOTH client audiences —
// preserving the pre-multi-issuer single-issuer behaviour.
func VerifierPairs(webIssuer, webClient, cliIssuer, cliClient string) []IssuerAudiences {
	if cliIssuer != "" && cliIssuer != webIssuer {
		return []IssuerAudiences{
			{Issuer: webIssuer, Audiences: []string{webClient}},
			{Issuer: cliIssuer, Audiences: []string{cliClient}},
		}
	}
	return []IssuerAudiences{
		{Issuer: webIssuer, Audiences: []string{webClient, cliClient}},
	}
}
