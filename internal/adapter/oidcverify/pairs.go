package oidcverify

// Audience is one accepted client_id for an issuer. Machine marks the audience
// minted by the machine-credential provider: a token carrying it is a headless
// client, never a human.
//
// The flag sits on the AUDIENCE and not on IssuerAudiences on purpose. Verify
// gives the first issuer whose signature, iss and exp check out OWNERSHIP of
// the token, and then fails HARD on an audience miss instead of trying the next
// entry — so two entries sharing one issuer are unusable. That is exactly the
// dev topology (a single Dex issuer fronting every client), which is why the
// distinction cannot live at the issuer level.
type Audience struct {
	ClientID string
	Machine  bool
}

// IssuerAudiences binds one trusted OIDC issuer to the audiences (client_ids)
// accepted for tokens minted by that issuer.
type IssuerAudiences struct {
	Issuer    string
	Audiences []Audience
}

// PairConfig is flow's OIDC client topology: a browser provider, a device-code
// CLI provider, and an optional machine-credential provider.
type PairConfig struct {
	WebIssuer, WebClient         string
	CLIIssuer, CLIClient         string
	MachineIssuer, MachineClient string
}

// VerifierPairs derives issuer→audience bindings from flow's OIDC config.
//
// Prod (Authentik per_provider): the issuers differ, so each issuer is bound to
// ITS OWN client only — a web-issued token may not carry the CLI audience and
// vice-versa (per-issuer tightness).
//
// Dev (a single Dex issuer fronting several clients): an issuer that is empty
// or equal to WebIssuer folds its client into the web issuer's audience set,
// preserving the pre-multi-issuer single-issuer behaviour.
//
// Machine auth is OFF whenever MachineClient is empty: no machine audience is
// registered at all, so Identity.Machine can never be true.
func VerifierPairs(c PairConfig) []IssuerAudiences {
	out := []IssuerAudiences{{
		Issuer:    c.WebIssuer,
		Audiences: []Audience{{ClientID: c.WebClient}},
	}}
	add := func(issuer, client string, machine bool) {
		if client == "" {
			return
		}
		aud := Audience{ClientID: client, Machine: machine}
		if issuer == "" || issuer == c.WebIssuer {
			out[0].Audiences = append(out[0].Audiences, aud)
			return
		}
		out = append(out, IssuerAudiences{Issuer: issuer, Audiences: []Audience{aud}})
	}
	add(c.CLIIssuer, c.CLIClient, false)
	add(c.MachineIssuer, c.MachineClient, true)
	return out
}
