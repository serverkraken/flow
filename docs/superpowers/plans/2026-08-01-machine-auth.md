# Maschinen-Auth für headless Clients — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** flow akzeptiert `client_credentials`-Tokens eines Authentik-Service-Accounts, delegiert sie auf einen bestehenden Besitzer und lässt sie ausschließlich Dokumente lesen und schreiben — damit der wartung-Runner seinen Lauf-Report endlich schreiben kann.

**Architecture:** „Maschine" ist eine vom Verifier bewiesene Eigenschaft des Tokens: nur eine Audience aus dem neuen `flow-machine`-Provider trägt sie. Die Middleware verweigert Maschinen-Token per Default auf **jeder** Route; ein eigener `authMachineOK`-Wrapper lässt sie auf sechs Dokument-Routen zu. Ein Maschinen-Token löst über eine Server-Env-Tabelle auf einen bestehenden User auf — es entsteht **kein** Maschinen-User, weil das ein zweiter Tenant wäre.

**Tech Stack:** Go 1.x, `github.com/coreos/go-oidc/v3`, `net/http.ServeMux` (Go-1.22-Muster), goose-Migrationen (hier **keine** nötig), Ansible (Runner-Seite).

**Spec:** `docs/superpowers/specs/2026-08-01-machine-auth-design.md` (Commits `c4db5c2`, `dad03a4`)

## Global Constraints

- **Branch:** `machine-auth`, Worktree `../flow-machine-auth`, von `main` @ `532f842`. Nie auf `main` committen.
- **Multi-Tenant ist bindend.** Jeder Datenzugriff bleibt owner-scoped. „Ist nur ein User, also egal" ist als Begründung unzulässig.
- **`make ci`** = `lint verify-generate verify-css verify-no-popups cover build` muss am Ende jeder Task grün sein. Coverage-Gate 75 %.
- **Keine Migration.** Dieser Slice fasst kein Schema an.
- **Keine neue Store-Methode.** Die Besitzerauflösung nutzt `ports.UserStore.GetBySub` (`internal/ports/ports.go:20`), das es bereits gibt.
- **Fehlerkörper sind Produkt, nicht Debug-Ausgabe.** Der Runner zitiert sie wörtlich in eine Chat-Nachricht, die morgens um 06:00 auf einem Telefon gelesen wird. Exakte Wortlaute stehen in Task 4 und dürfen nicht umformuliert werden.
- **Exakte Fehlertexte:**
  - `invalid token` (401)
  - `machine token not mapped to an owner` (403)
  - `machine account "<label>" maps to an unknown owner` (403)
  - `machine tokens are not accepted on this route` (403)
- **Hexagonale Grenze:** `internal/adapter/httpserver` importiert **nicht** `internal/config`. Die Übersetzung passiert in `cmd/flow-server/main.go` (Task 6).

---

### Task 1: Authentik-Provider anlegen und die Token-Form verifizieren

**Kein Code in diesem Repo.** Dies ist ein Gate: der gesamte Entwurf steht auf der Annahme, dass Authentiks `client_credentials`-Access-Token ein JWT mit `iss` = Provider-Issuer und `aud` = `flow-machine` ist. Trifft das nicht zu, wird Task 2 neu geschnitten, statt sechs Tasks auf falschem Fundament zu bauen.

**Files:**
- Modify (Infra-Repo, nicht hier): Authentik-Blueprint für Provider + Service-Account
- Create: `docs/superpowers/plans/2026-08-01-machine-auth-task1-befund.md` (der festgehaltene Befund)

**Interfaces:**
- Produces: die zwei Werte, die Task 6 in die Server-Env schreibt — **Maschinen-Sub** (`sub` aus dem Token) und **Besitzer-Sub** (Soennes eigener Sub, steht schon in `FLOW_ALLOWED_SUBS`). Außerdem den bestätigten Issuer-String und die bestätigte Audience.

- [ ] **Step 1: Provider + Service-Account in Authentik anlegen**

Deklarativ per Blueprint, App-Passwort aus sops via `!Env` (Präzedenz `32-users-mail.yaml.j2`):

- OAuth2/OIDC-Provider `flow-machine`, `grant_types: [client_credentials]` — **kein** `device_code`, **kein** `authorization_code`.
- Issuer-Modus `per_provider` wie die bestehenden beiden.
- Eigene Anwendung mit Policy-Binding, das den Service-Account autorisiert.
- Service-Account `wartung-agent`. **Keine** `flow-users`-Gruppenmitgliedschaft nötig.
- Der `flow-cli`-Provider bleibt unangetastet.

- [ ] **Step 2: Token holen und den Payload dekodieren**

```bash
curl -fsS -X POST https://id.thebackend.org/application/o/token/ \
  -d grant_type=client_credentials \
  -d client_id=flow-machine \
  -d username=wartung-agent \
  -d password="$FLOW_SA_PASSWORD" \
  -d scope=openid | jq -r .access_token | cut -d. -f2 | base64 -d | jq
```

Erwartet: ein dekodierbarer JSON-Payload mit `iss`, `aud`, `sub`, `exp`.

- [ ] **Step 3: Den Befund gegen die Spec prüfen**

Drei Fragen, jede mit einer Konsequenz:

1. **Ist der Access-Token ein JWT?** Wenn `cut -d. -f2 | base64 -d` keinen JSON-Payload ergibt, ist er opak. → **STOP**, Plan neu schneiden; §4 der Spec trägt dann nicht.
2. **Ist `iss` der Provider-Issuer** (`…/application/o/flow-machine/`)? Wenn nicht, ist der notierte Wert das, was in `FLOW_OIDC_MACHINE_ISSUER` gehört.
3. **Enthält `aud` den String `flow-machine`?** Wenn `aud` etwas anderes trägt, ist *dieser* Wert `FLOW_OIDC_MACHINE_CLIENT_ID`. `aud` darf ein String oder ein Array sein — `go-oidc` behandelt beides.

- [ ] **Step 4: Befund festhalten**

`docs/superpowers/plans/2026-08-01-machine-auth-task1-befund.md`:

```markdown
# Task-1-Befund: Authentik-Token-Form (2026-08-__)

- JWT statt opak: JA / NEIN
- iss: <wörtlich>
- aud: <wörtlich>
- sub (Maschinen-Sub): <wörtlich>
- exp - iat (Token-Lebensdauer): <sekunden>

Daraus folgt für Task 6:
- FLOW_OIDC_MACHINE_ISSUER=<iss>
- FLOW_OIDC_MACHINE_CLIENT_ID=<aud>
- FLOW_MACHINE_ACCOUNTS=<sub>=<soennes-sub>:wartung-agent
```

Der Besitzer-Sub ist der Wert, der bereits in `FLOW_ALLOWED_SUBS` steht — nicht neu erfinden, dort ablesen.

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/plans/2026-08-01-machine-auth-task1-befund.md
git commit -m "docs(plan): Task-1-Befund — Authentik-Token-Form verifiziert"
```

---

### Task 2: `oidcverify` — Maschinen-Flagge an der Audience

**Files:**
- Modify: `internal/adapter/oidcverify/pairs.go` (komplett ersetzt)
- Modify: `internal/adapter/oidcverify/verifier.go:20-24, 42-65, 88-102`
- Modify: `internal/ports/ports.go:24-31`
- Modify: `cmd/flow-server/main.go:61-62`
- Test: `internal/adapter/oidcverify/pairs_test.go` (komplett ersetzt)
- Test: `internal/adapter/oidcverify/verifier_multi_test.go:47-58, 60-105` + neuer Test

**Interfaces:**
- Produces:
  - `oidcverify.Audience{ClientID string; Machine bool}`
  - `oidcverify.IssuerAudiences{Issuer string; Audiences []Audience}`
  - `oidcverify.PairConfig{WebIssuer, WebClient, CLIIssuer, CLIClient, MachineIssuer, MachineClient string}`
  - `oidcverify.VerifierPairs(c PairConfig) []IssuerAudiences`
  - `ports.Identity.Machine bool`

- [ ] **Step 1: Den Pairs-Test schreiben (schlägt fehl)**

`internal/adapter/oidcverify/pairs_test.go` **vollständig ersetzen**:

```go
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
```

- [ ] **Step 2: Test laufen lassen, Fehlschlag bestätigen**

Run: `go test ./internal/adapter/oidcverify/ -run TestVerifierPairs`
Expected: FAIL — `undefined: oidcverify.PairConfig`, `undefined: oidcverify.Audience`

- [ ] **Step 3: `pairs.go` vollständig ersetzen**

```go
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
```

- [ ] **Step 4: Test laufen lassen, Erfolg bestätigen**

Run: `go test ./internal/adapter/oidcverify/ -run TestVerifierPairs`
Expected: PASS

- [ ] **Step 5: `ports.Identity` um `Machine` erweitern**

`internal/ports/ports.go`, im `Identity`-Struct nach `Groups`:

```go
	// Machine reports a token minted by the machine-credential provider. It is
	// proven by the issuer/audience pair the token verified against, never by
	// anything the caller sends.
	Machine bool
```

- [ ] **Step 6: Den Verifier-Test schreiben (schlägt fehl)**

In `internal/adapter/oidcverify/verifier_multi_test.go` die Helferfunktion ersetzen:

```go
// staticIssuerVerifier builds a network-free issuerVerifier bound to one issuer,
// one key, and one allowed audience set.
func staticIssuerVerifier(iss string, pub *rsa.PublicKey, auds ...Audience) issuerVerifier {
	allow := map[string]bool{}
	for _, a := range auds {
		allow[a.ClientID] = a.Machine
	}
	return issuerVerifier{
		v: oidc.NewVerifier(iss,
			&oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{pub}},
			&oidc.Config{SkipClientIDCheck: true}),
		auds: allow,
	}
}
```

Und in `TestVerifyMultiIssuerPerIssuerAudience` die beiden Aufrufe anpassen:

```go
	vr := &Verifier{verifiers: []issuerVerifier{
		staticIssuerVerifier(issA, &keyA.PublicKey, Audience{ClientID: "flow"}),
		staticIssuerVerifier(issB, &keyB.PublicKey, Audience{ClientID: "flow-cli"}),
	}}
```

Dann den neuen Test **ans Dateiende** anhängen:

```go
func TestVerifyStampsMachineFromAudience(t *testing.T) {
	keyWeb, keyMachine := genKey(t), genKey(t)
	const (
		issWeb     = "https://id.example/application/o/flow/"
		issMachine = "https://id.example/application/o/flow-machine/"
	)
	vr := &Verifier{verifiers: []issuerVerifier{
		staticIssuerVerifier(issWeb, &keyWeb.PublicKey, Audience{ClientID: "flow"}),
		staticIssuerVerifier(issMachine, &keyMachine.PublicKey,
			Audience{ClientID: "flow-machine", Machine: true}),
	}}
	ctx := context.Background()

	// A machine token is stamped.
	id, err := vr.Verify(ctx, mintRS256(t, keyMachine, issMachine, "flow-machine"))
	if err != nil {
		t.Fatalf("machine token: %v", err)
	}
	if !id.Machine {
		t.Fatal("machine token must set Identity.Machine")
	}

	// A human token is not.
	id, err = vr.Verify(ctx, mintRS256(t, keyWeb, issWeb, "flow"))
	if err != nil {
		t.Fatalf("web token: %v", err)
	}
	if id.Machine {
		t.Fatal("web token must not set Identity.Machine")
	}

	// The machine audience presented on the WEB issuer is rejected outright —
	// it must never be a route to Machine=true on a human-issued token.
	if _, err := vr.Verify(ctx, mintRS256(t, keyWeb, issWeb, "flow-machine")); err == nil {
		t.Fatal("expected reject: machine audience on web issuer")
	}
}

func TestVerifyDevSingleIssuerStampsOnlyTheMachineAudience(t *testing.T) {
	key := genKey(t)
	const iss = "http://localhost:5556/dex"
	vr := &Verifier{verifiers: []issuerVerifier{
		staticIssuerVerifier(iss, &key.PublicKey,
			Audience{ClientID: "flow-dev"},
			Audience{ClientID: "flow-cli"},
			Audience{ClientID: "flow-machine", Machine: true}),
	}}
	ctx := context.Background()

	for _, tc := range []struct {
		aud  string
		want bool
	}{
		{"flow-dev", false},
		{"flow-cli", false},
		{"flow-machine", true},
	} {
		id, err := vr.Verify(ctx, mintRS256(t, key, iss, tc.aud))
		if err != nil {
			t.Fatalf("aud %s: %v", tc.aud, err)
		}
		if id.Machine != tc.want {
			t.Fatalf("aud %s: Machine = %v, want %v", tc.aud, id.Machine, tc.want)
		}
	}
}
```

Außerdem `TestNewRejectsEmptyIssuer` und `TestNewRejectsEmptyAudiences` auf den neuen Typ heben:

```go
func TestNewRejectsEmptyIssuer(t *testing.T) {
	if _, err := New(context.Background(), []IssuerAudiences{{Issuer: "", Audiences: []Audience{{ClientID: "x"}}}}); err == nil {
		t.Fatal("expected error for empty issuer")
	}
}
```

(`TestNewRejectsEmptyAudiences` bleibt unverändert — `Audiences: nil` typisiert sich selbst.)

- [ ] **Step 7: Test laufen lassen, Fehlschlag bestätigen**

Run: `go test ./internal/adapter/oidcverify/`
Expected: FAIL — `cannot use Audience{…} as string` in `staticIssuerVerifier`, bzw. `id.Machine undefined`

- [ ] **Step 8: `verifier.go` anpassen**

Erstens die Kommentar- und Feldbedeutung von `auds` (`verifier.go:20-24`):

```go
// issuerVerifier is one issuer's token verifier plus its accepted audiences,
// mapped client_id → "this audience marks a machine token".
type issuerVerifier struct {
	v    *oidc.IDTokenVerifier
	auds map[string]bool
}
```

Zweitens der Aufbau in `New` (ersetzt `verifier.go:54-59`):

```go
		auds := make(map[string]bool, len(p.Audiences))
		for _, a := range p.Audiences {
			if a.ClientID != "" {
				auds[a.ClientID] = a.Machine
			}
		}
```

Drittens die Audience-Prüfung in `Verify` (ersetzt `verifier.go:88-94`):

```go
		ok, machine := false, false
		for _, a := range tok.Audience {
			if m, found := iv.auds[a]; found {
				ok, machine = true, m
				break
			}
		}
```

Viertens die Rückgabe (ersetzt `verifier.go:102`):

```go
		return ports.Identity{
			Subject:  c.Sub,
			Username: c.PreferredUsername,
			Email:    c.Email,
			Name:     c.Name,
			Groups:   c.Groups,
			Machine:  machine,
		}, nil
```

Die Zwei-Wert-Form `m, found := iv.auds[a]` ist nicht optional: eine Nicht-Maschinen-Audience steht als `false` in der Map, ein einwertiger Lookup würde sie von „nicht eingetragen" nicht unterscheiden und jeden menschlichen Token ablehnen.

- [ ] **Step 9: Test laufen lassen, Erfolg bestätigen**

Run: `go test ./internal/adapter/oidcverify/ ./internal/ports/`
Expected: PASS

- [ ] **Step 10: Aufrufstelle in `main.go` anpassen**

`cmd/flow-server/main.go:61-62` ersetzen:

```go
	verifier, err := oidcverify.New(ctx, oidcverify.VerifierPairs(oidcverify.PairConfig{
		WebIssuer: cfg.OIDCIssuer, WebClient: cfg.OIDCClientID,
		CLIIssuer: cfg.OIDCCliIssuer, CLIClient: cfg.OIDCCliClientID,
	}))
```

Die Maschinen-Felder kommen in Task 6 dazu — bis dahin ist das Feature inert, genau wie in einer Dev-Umgebung ohne Maschinen-Auth.

- [ ] **Step 11: Ganze Suite + Build**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 12: Commit**

```bash
git add internal/adapter/oidcverify/ internal/ports/ports.go cmd/flow-server/main.go
git commit -m "feat(oidcverify): Maschinen-Audience als bewiesene Token-Eigenschaft

Audiences tragen jetzt ein Machine-Flag, und Verify stempelt daraus
Identity.Machine. Die Flagge sitzt an der Audience statt am Issuer-Paar,
weil Verify dem ersten passenden Issuer die Ownership gibt und bei einem
Audience-Miss hart fehlschlägt — zwei Eintraege mit demselben Issuer
waeren unbrauchbar, und genau das ist die Dev-Topologie mit einem
einzelnen Dex-Issuer.

VerifierPairs nimmt ein PairConfig statt sechs Positionsstrings."
```

---

### Task 3: `config` — `FLOW_MACHINE_ACCOUNTS`

**Files:**
- Modify: `internal/config/config.go:9-22, 25-67`
- Test: `internal/config/config_machine_test.go` (neu)

**Interfaces:**
- Consumes: nichts aus Task 2.
- Produces:
  - `config.MachineAccount{Sub, OwnerSub, Label string}`
  - `config.Config.OIDCMachineIssuer string`
  - `config.Config.OIDCMachineClientID string`
  - `config.Config.MachineAccounts map[string]MachineAccount` — Schlüssel ist der **Maschinen-Sub**

- [ ] **Step 1: Den Test schreiben (schlägt fehl)**

`internal/config/config_machine_test.go`:

```go
package config

import "testing"

// baseEnv is the minimal valid environment; machine-auth cases layer on top.
func baseEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL":            "postgres://x",
		"FLOW_OIDC_ISSUER":        "https://issuer",
		"FLOW_OIDC_CLIENT_ID":     "flow",
		"FLOW_OIDC_CLI_CLIENT_ID": "flow-cli",
		"FLOW_OIDC_CLIENT_SECRET": "s",
		"FLOW_PUBLIC_BASE_URL":    "https://flow.example.com",
		"FLOW_SESSION_SECRET":     "secret",
	}
}

func withMachine(accounts string) map[string]string {
	env := baseEnv()
	env["FLOW_OIDC_MACHINE_ISSUER"] = "https://issuer/o/flow-machine/"
	env["FLOW_OIDC_MACHINE_CLIENT_ID"] = "flow-machine"
	env["FLOW_MACHINE_ACCOUNTS"] = accounts
	return env
}

func loadEnv(env map[string]string) (Config, error) {
	return Load(func(k string) string { return env[k] })
}

func TestLoadMachineAccounts(t *testing.T) {
	c, err := loadEnv(withMachine(" sa-1 = owner-1 : wartung-agent , sa-2=owner-1:backup-agent "))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.MachineAccounts) != 2 {
		t.Fatalf("want 2 accounts, got %v", c.MachineAccounts)
	}
	got, ok := c.MachineAccounts["sa-1"]
	if !ok {
		t.Fatalf("sa-1 missing from %v", c.MachineAccounts)
	}
	want := MachineAccount{Sub: "sa-1", OwnerSub: "owner-1", Label: "wartung-agent"}
	if got != want {
		t.Fatalf("sa-1 = %+v, want %+v", got, want)
	}
	if c.OIDCMachineClientID != "flow-machine" {
		t.Fatalf("OIDCMachineClientID = %q", c.OIDCMachineClientID)
	}
}

func TestLoadMachineAuthUnsetIsFine(t *testing.T) {
	c, err := loadEnv(baseEnv())
	if err != nil {
		t.Fatalf("Load without machine auth must succeed: %v", err)
	}
	if len(c.MachineAccounts) != 0 {
		t.Fatalf("MachineAccounts should be empty, got %v", c.MachineAccounts)
	}
	if c.OIDCMachineIssuer != "" || c.OIDCMachineClientID != "" {
		t.Fatal("machine issuer/client must stay empty when unset")
	}
}

func TestLoadMachineAuthRejectsPartialConfig(t *testing.T) {
	env := baseEnv()
	env["FLOW_OIDC_MACHINE_ISSUER"] = "https://issuer/o/flow-machine/"
	if _, err := loadEnv(env); err == nil {
		t.Fatal("partial machine config must fail loudly, not silently reject tokens later")
	}
}

func TestLoadMachineAccountsRejectsMalformed(t *testing.T) {
	cases := []struct {
		name, accounts string
	}{
		{"no equals", "sa-1:wartung-agent"},
		{"no colon", "sa-1=owner-1"},
		{"empty machine sub", "=owner-1:wartung-agent"},
		{"empty owner sub", "sa-1=:wartung-agent"},
		{"empty label", "sa-1=owner-1:"},
		{"duplicate machine sub", "sa-1=owner-1:a,sa-1=owner-2:b"},
		{"machine sub equals owner sub", "sa-1=sa-1:wartung-agent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := loadEnv(withMachine(tc.accounts)); err == nil {
				t.Fatalf("accounts %q must be rejected", tc.accounts)
			}
		})
	}
}
```

- [ ] **Step 2: Test laufen lassen, Fehlschlag bestätigen**

Run: `go test ./internal/config/`
Expected: FAIL — `undefined: MachineAccount`, `c.MachineAccounts undefined`

- [ ] **Step 3: Struct + Felder ergänzen**

In `internal/config/config.go`, vor `type Config struct`:

```go
// MachineAccount delegates a machine credential to a human owner's tenant.
//
// The owner is addressed by OIDC subject, not by username: only users.oidc_sub
// carries a UNIQUE constraint (migration 0001), while username is a plain
// TEXT NOT NULL DEFAULT ''. Resolving an owner by a non-unique column would be
// ambiguous, and an ambiguous owner in a multi-tenant app means a machine token
// can end up writing into the wrong tenant.
type MachineAccount struct {
	Sub      string // the service account's OIDC subject
	OwnerSub string // the OIDC subject of the flow user whose tenant it writes into
	Label    string // audit name, surfaced as the actor Ref
}
```

In `Config`, nach `OIDCCliClientID`:

```go
	OIDCMachineIssuer   string
	OIDCMachineClientID string
	// MachineAccounts is keyed by the MACHINE subject.
	MachineAccounts map[string]MachineAccount
```

- [ ] **Step 4: Parser schreiben**

Ans Ende von `internal/config/config.go`:

```go
// parseMachineAccounts reads a comma-separated list of
// <machine-sub>=<owner-sub>:<label>. Every malformed entry is an error rather
// than a skip: a typo'd mapping that silently becomes "not mapped" would show
// up as a 403 at 04:30 with nothing pointing at the real cause.
func parseMachineAccounts(raw string) (map[string]MachineAccount, error) {
	out := map[string]MachineAccount{}
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		sub, rest, hasEq := strings.Cut(entry, "=")
		ownerSub, label, hasColon := strings.Cut(rest, ":")
		if !hasEq || !hasColon {
			return nil, fmt.Errorf(
				"config: FLOW_MACHINE_ACCOUNTS entry %q: want <machine-sub>=<owner-sub>:<label>", entry)
		}
		sub = strings.TrimSpace(sub)
		ownerSub = strings.TrimSpace(ownerSub)
		label = strings.TrimSpace(label)
		if sub == "" || ownerSub == "" || label == "" {
			return nil, fmt.Errorf(
				"config: FLOW_MACHINE_ACCOUNTS entry %q: machine sub, owner sub and label must all be non-empty", entry)
		}
		if sub == ownerSub {
			return nil, fmt.Errorf(
				"config: FLOW_MACHINE_ACCOUNTS entry %q: machine sub and owner sub are identical — "+
					"this would demote a human token to machine rights", entry)
		}
		if _, dup := out[sub]; dup {
			return nil, fmt.Errorf("config: FLOW_MACHINE_ACCOUNTS: duplicate machine sub %q", sub)
		}
		out[sub] = MachineAccount{Sub: sub, OwnerSub: ownerSub, Label: label}
	}
	return out, nil
}
```

- [ ] **Step 5: In `Load` verdrahten**

In `Load`, im `Config`-Literal nach `OIDCCliClientID`:

```go
		OIDCMachineIssuer:   getenv("FLOW_OIDC_MACHINE_ISSUER"),
		OIDCMachineClientID: getenv("FLOW_OIDC_MACHINE_CLIENT_ID"),
		MachineAccounts:     map[string]MachineAccount{},
```

Und **vor** dem abschließenden `return c, nil`, nach der bestehenden Pflichtfeld-Schleife:

```go
	// Machine auth is all-or-nothing. A half-configured server would verify
	// machine tokens and then reject them with no hint that a variable is
	// missing — a loud start-up failure costs less diagnosis time.
	rawMachineAccounts := getenv("FLOW_MACHINE_ACCOUNTS")
	var machineSet, machineMissing []string
	for _, f := range []struct{ name, val string }{
		{"FLOW_OIDC_MACHINE_ISSUER", c.OIDCMachineIssuer},
		{"FLOW_OIDC_MACHINE_CLIENT_ID", c.OIDCMachineClientID},
		{"FLOW_MACHINE_ACCOUNTS", rawMachineAccounts},
	} {
		if strings.TrimSpace(f.val) == "" {
			machineMissing = append(machineMissing, f.name)
		} else {
			machineSet = append(machineSet, f.name)
		}
	}
	if len(machineSet) > 0 && len(machineMissing) > 0 {
		return Config{}, fmt.Errorf(
			"config: machine auth is partially configured (%s set); also required: %s",
			strings.Join(machineSet, ", "), strings.Join(machineMissing, ", "))
	}
	if len(machineSet) == 3 {
		accounts, err := parseMachineAccounts(rawMachineAccounts)
		if err != nil {
			return Config{}, err
		}
		if len(accounts) == 0 {
			return Config{}, fmt.Errorf("config: FLOW_MACHINE_ACCOUNTS is set but contains no entries")
		}
		c.MachineAccounts = accounts
	}
```

- [ ] **Step 6: Test laufen lassen, Erfolg bestätigen**

Run: `go test ./internal/config/`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/config/
git commit -m "feat(config): FLOW_MACHINE_ACCOUNTS — Maschinen-Sub auf Besitzer delegieren

Der Besitzer wird per OIDC-Sub adressiert, nicht per Username: nur
users.oidc_sub ist UNIQUE, username ist ein gewoehnliches TEXT-Feld. Eine
mehrdeutige Besitzerauflösung wäre in einer Multi-Tenant-App ein
Cross-Tenant-Risiko.

Maschinen-Auth ist all-or-nothing; jeder fehlerhafte Eintrag ist ein
Startfehler statt eines stillen Übersprungs."
```

---

### Task 4: Actor + Middleware — Delegation und Default-Deny

**Files:**
- Modify: `internal/actor/actor.go` (neue Funktion ans Dateiende)
- Modify: `internal/adapter/httpserver/server.go:151` (Umfeld — neues Feld)
- Modify: `internal/adapter/httpserver/middleware.go:38-80` (ersetzt)
- Modify: `internal/adapter/httpserver/webauth.go:136-152`
- Create: `internal/adapter/httpserver/machineauth.go`
- Test: `internal/adapter/httpserver/machineauth_test.go` (neu)

**Interfaces:**
- Consumes: `ports.Identity.Machine` (Task 2)
- Produces:
  - `actor.TrustedMachine(label string) Actor`
  - `httpserver.MachineAccount{OwnerSub, Label string}`
  - `httpserver.Server.Machines map[string]MachineAccount`
  - `(*Server).authMachineOK(next http.Handler) http.Handler`

- [ ] **Step 1: Den Test schreiben (schlägt fehl)**

`internal/adapter/httpserver/machineauth_test.go`:

```go
package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

// machineTestServer wires a Server whose verifier returns the given identity,
// with one owner already present in the store.
func machineTestServer(t *testing.T, id ports.Identity) (*Server, domain.User) {
	t.Helper()
	users := testutil.NewFakeUserStore()
	owner, err := domain.NewUser("owner-id", "owner-sub", "soenne", "s@x.de", "Soenne")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.UpsertBySub(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	return &Server{
		Verifier: testutil.FakeVerifier{ID: id},
		Users:    users,
		Ensure: usecase.EnsureUser{
			Users: users,
			IDs:   &testutil.FakeIDGen{},
			Allow: func(ports.Identity) bool { return true },
		},
		Machines: map[string]MachineAccount{
			"machine-sub": {OwnerSub: "owner-sub", Label: "wartung-agent"},
		},
	}, owner
}

func machineProbe(gotUser *domain.User, gotActor *actor.Actor) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotUser, _ = userFrom(r.Context())
		*gotActor = actor.FromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})
}

func doBearer(h http.Handler) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/documents", nil)
	req.Header.Set("Authorization", "Bearer t")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestMachineTokenOnOptedInRouteDelegatesToOwner(t *testing.T) {
	srv, owner := machineTestServer(t, ports.Identity{Subject: "machine-sub", Machine: true})
	var gotUser domain.User
	var gotActor actor.Actor

	rec := doBearer(srv.authMachineOK(machineProbe(&gotUser, &gotActor)))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body %q)", rec.Code, rec.Body.String())
	}
	if gotUser.ID != owner.ID {
		t.Fatalf("delegated user = %q, want the owner %q", gotUser.ID, owner.ID)
	}
	if gotActor.Kind != actor.Agent || gotActor.Ref != "wartung-agent" {
		t.Fatalf("actor = %+v, want {agent wartung-agent}", gotActor)
	}
}

func TestMachineTokenNeverCreatesItsOwnUser(t *testing.T) {
	srv, _ := machineTestServer(t, ports.Identity{Subject: "machine-sub", Machine: true})
	var gotUser domain.User
	var gotActor actor.Actor

	doBearer(srv.authMachineOK(machineProbe(&gotUser, &gotActor)))

	// A user record for the MACHINE subject would be a second tenant — exactly
	// what delegation exists to avoid.
	if _, err := srv.Users.GetBySub(context.Background(), "machine-sub"); err == nil {
		t.Fatal("a user record was created for the machine subject")
	}
}

func TestMachineTokenRejectedOnPlainAuthRoute(t *testing.T) {
	srv, _ := machineTestServer(t, ports.Identity{Subject: "machine-sub", Machine: true})
	var gotUser domain.User
	var gotActor actor.Actor

	rec := doBearer(srv.auth(machineProbe(&gotUser, &gotActor)))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "machine tokens are not accepted on this route") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestMachineTokenOnAuthAnyIsForbiddenNotUnauthorized(t *testing.T) {
	srv, _ := machineTestServer(t, ports.Identity{Subject: "machine-sub", Machine: true})
	var gotUser domain.User
	var gotActor actor.Actor

	rec := doBearer(srv.authAny(machineProbe(&gotUser, &gotActor)))

	// Falling through to the cookie would answer 401 to a caller that did in
	// fact authenticate — a misleading answer the operator has to debug.
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestMachineTokenWithUnmappedSubject(t *testing.T) {
	srv, _ := machineTestServer(t, ports.Identity{Subject: "stranger", Machine: true})
	var gotUser domain.User
	var gotActor actor.Actor

	rec := doBearer(srv.authMachineOK(machineProbe(&gotUser, &gotActor)))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "machine token not mapped to an owner") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestMachineAccountMappedToUnknownOwner(t *testing.T) {
	srv, _ := machineTestServer(t, ports.Identity{Subject: "machine-sub", Machine: true})
	srv.Machines = map[string]MachineAccount{
		"machine-sub": {OwnerSub: "nobody", Label: "wartung-agent"},
	}
	var gotUser domain.User
	var gotActor actor.Actor

	rec := doBearer(srv.authMachineOK(machineProbe(&gotUser, &gotActor)))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `machine account "wartung-agent" maps to an unknown owner`) {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestHumanTokenUnaffectedByMachineWrappers(t *testing.T) {
	srv, _ := machineTestServer(t, ports.Identity{Subject: "owner-sub", Username: "soenne"})
	var gotUser domain.User
	var gotActor actor.Actor

	for name, h := range map[string]http.Handler{
		"auth":          srv.auth(machineProbe(&gotUser, &gotActor)),
		"authMachineOK": srv.authMachineOK(machineProbe(&gotUser, &gotActor)),
	} {
		rec := doBearer(h)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s: status = %d, want 204 (body %q)", name, rec.Code, rec.Body.String())
		}
		if gotActor.Kind != actor.Human {
			t.Fatalf("%s: actor = %+v, want a human", name, gotActor)
		}
	}
}
```

- [ ] **Step 2: Test laufen lassen, Fehlschlag bestätigen**

Run: `go test ./internal/adapter/httpserver/ -run TestMachine`
Expected: FAIL — `undefined: MachineAccount`, `srv.Machines undefined`, `srv.authMachineOK undefined`

- [ ] **Step 3: `actor.TrustedMachine` ergänzen**

Ans Ende von `internal/actor/actor.go`:

```go
// TrustedMachine builds audit provenance for a verified machine credential: a
// token minted by the machine issuer/audience pair whose subject the server's
// own configuration maps to an owner. The label comes from that configuration,
// never from the request — which is the condition AuthenticatedHuman's comment
// sets for anything being promoted to Agent.
func TrustedMachine(label string) Actor {
	return Actor{Kind: Agent, Ref: label}
}
```

- [ ] **Step 4: `machineauth.go` anlegen**

```go
package httpserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// MachineAccount delegates a verified machine credential to a human owner's
// tenant. It mirrors config.MachineAccount on purpose: an adapter must not
// import the config package (adapter → usecase → ports ← domain), so
// cmd/flow-server translates between the two.
type MachineAccount struct {
	OwnerSub string
	Label    string
}

// errMachineUnmapped is returned for a machine token whose subject is absent
// from the server's configured accounts.
var errMachineUnmapped = errors.New("machine token not mapped to an owner")

// resolveMachine turns a verified machine identity into the delegated owner and
// its audit label.
//
// Machines deliberately never get a user record of their own: a second user row
// would be a second tenant with its own empty project tree, and the reports
// would then be invisible to the human they are written for.
func (s *Server) resolveMachine(ctx context.Context, id ports.Identity) (domain.User, string, error) {
	acct, ok := s.Machines[id.Subject]
	if !ok {
		return domain.User{}, "", errMachineUnmapped
	}
	u, err := s.Users.GetBySub(ctx, acct.OwnerSub)
	if err != nil {
		// The owner SUBJECT is deliberately not echoed: the label already
		// identifies the entry to fix, and the subject is not the operator's to
		// read off a phone at 06:00.
		return domain.User{}, "", fmt.Errorf("machine account %q maps to an unknown owner", acct.Label)
	}
	return u, acct.Label, nil
}

// ctxWithMachine stores the delegated owner plus machine provenance. The owner
// is what every handler scopes its queries by; the actor is what the audit
// trail records.
func ctxWithMachine(ctx context.Context, u domain.User, label string) context.Context {
	ctx = context.WithValue(ctx, userKey, u)
	return actor.WithContext(ctx, actor.TrustedMachine(label))
}
```

- [ ] **Step 5: `Server.Machines` ergänzen**

In `internal/adapter/httpserver/server.go`, direkt unter `Users ports.UserStore` (Zeile 151):

```go
	// Machines maps a machine credential's OIDC subject to the owner it is
	// delegated to. Empty (the default) disables machine auth entirely.
	Machines map[string]MachineAccount
```

- [ ] **Step 6: `middleware.go` umbauen**

`internal/adapter/httpserver/middleware.go`, Zeilen 38-80 ersetzen:

```go
// resolveBearer verifies a bearer token and ensures the user. ok=false on any
// failure (used by authAny, which then tries the cookie). machine=true reports
// a VERIFIED machine token, which authAny must answer with 403 rather than let
// fall through — see authAny.
func (s *Server) resolveBearer(r *http.Request) (u domain.User, machine bool, ok bool) {
	raw := bearerToken(r)
	if raw == "" {
		return domain.User{}, false, false
	}
	id, err := s.Verifier.Verify(r.Context(), raw)
	if err != nil {
		return domain.User{}, false, false
	}
	if id.Machine {
		return domain.User{}, true, false
	}
	u, err = s.Ensure.Execute(r.Context(), id)
	if err != nil {
		return domain.User{}, false, false
	}
	return u, false, true
}

// bearerToken extracts the raw token, or "" when the header is absent or not a
// Bearer header.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	raw := strings.TrimPrefix(h, "Bearer ")
	if raw == h {
		return ""
	}
	return raw
}

// auth verifies the bearer token, ensures the user, and stores it in context.
// Machine tokens are refused: a route accepts them only by being wrapped in
// authMachineOK, so a newly added route is machine-tight without anyone having
// to remember it.
func (s *Server) auth(next http.Handler) http.Handler {
	return s.authWith(next, false)
}

// authMachineOK is auth plus machine credentials. Wrap only routes a headless
// client legitimately needs.
func (s *Server) authMachineOK(next http.Handler) http.Handler {
	return s.authWith(next, true)
}

func (s *Server) authWith(next http.Handler, allowMachine bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := bearerToken(r)
		if raw == "" {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		id, err := s.Verifier.Verify(r.Context(), raw)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		if id.Machine {
			if !allowMachine {
				http.Error(w, "machine tokens are not accepted on this route", http.StatusForbidden)
				return
			}
			u, label, err := s.resolveMachine(r.Context(), id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r.WithContext(ctxWithMachine(r.Context(), u, label)))
			return
		}
		u, err := s.Ensure.Execute(r.Context(), id)
		if errors.Is(err, usecase.ErrNotAllowed) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctxWithUser(r.Context(), u)))
	})
}
```

- [ ] **Step 7: `authAny` anpassen**

`internal/adapter/httpserver/webauth.go:138` ersetzen:

```go
		if u, machine, ok := s.resolveBearer(r); ok {
			next.ServeHTTP(w, r.WithContext(ctxWithUser(r.Context(), u)))
			return
		} else if machine {
			// The caller DID authenticate; falling through to the cookie would
			// answer 401 and send the operator hunting for a credential problem
			// that does not exist.
			http.Error(w, "machine tokens are not accepted on this route", http.StatusForbidden)
			return
		}
```

- [ ] **Step 8: Test laufen lassen, Erfolg bestätigen**

Run: `go test ./internal/adapter/httpserver/ -run TestMachine && go test ./internal/actor/`
Expected: PASS

- [ ] **Step 9: Ganze Suite**

Run: `go build ./... && go test ./...`
Expected: PASS — insbesondere die bestehenden `csrf_test.go`- und `webauth`-Tests, die `resolveBearer` indirekt treffen.

- [ ] **Step 10: Commit**

```bash
git add internal/actor/ internal/adapter/httpserver/
git commit -m "feat(httpserver): Maschinen-Token delegieren, per Default ablehnen

auth und authAny weisen verifizierte Maschinen-Token immer mit 403 ab;
nur der neue authMachineOK-Wrapper laesst sie zu. Eine kuenftig
hinzugefuegte Route ist damit maschinen-dicht, ohne dass jemand daran
denken muss.

Ein Maschinen-Token loest ueber Server.Machines auf einen bestehenden
User auf und bekommt actor.TrustedMachine als Provenance — es entsteht
kein Maschinen-User, weil das ein zweiter Tenant waere."
```

---

### Task 5: Die sechs Routen freischalten

**Files:**
- Modify: `internal/adapter/httpserver/server.go:158, 226, 229, 233, 234, 235`
- Test: `internal/adapter/httpserver/machineauth_routes_test.go` (neu)

**Interfaces:**
- Consumes: `(*Server).authMachineOK` (Task 4)

- [ ] **Step 1: Den Test schreiben (schlägt fehl)**

`internal/adapter/httpserver/machineauth_routes_test.go`:

```go
package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/ports"
)

// TestMachineRouteMatrix pins the opt-in list from the design spec §5.1. A
// route reaching the handler answers something other than 403; a refused route
// answers exactly 403 with the route message. The point is the ALLOW/DENY
// split, not each handler's own status.
func TestMachineRouteMatrix(t *testing.T) {
	srv, _ := machineTestServer(t, ports.Identity{Subject: "machine-sub", Machine: true})
	mux := srv.Routes()

	cases := []struct {
		method, path string
		allowed      bool
	}{
		{http.MethodPost, "/api/v1/documents", true},
		{http.MethodGet, "/api/v1/documents", true},
		{http.MethodGet, "/api/v1/documents/doc-1", true},
		{http.MethodPut, "/api/v1/documents/doc-1", true},
		{http.MethodPatch, "/api/v1/documents/doc-1", true},
		{http.MethodGet, "/api/v1/me", true},

		{http.MethodDelete, "/api/v1/documents/doc-1", false},
		{http.MethodPost, "/api/v1/documents/import", false},
		{http.MethodPut, "/api/v1/documents/by-path", false},
		{http.MethodPost, "/api/v1/documents/doc-1/pin", false},
		{http.MethodPost, "/api/v1/documents/doc-1/archive", false},
		{http.MethodPost, "/api/v1/documents/doc-1/move", false},
		{http.MethodPost, "/api/v1/documents/doc-1/context-mode", false},
		{http.MethodPost, "/api/v1/nodes", false},
		{http.MethodGet, "/api/v1/nodes", false},
		{http.MethodPost, "/api/v1/sessions", false},
		{http.MethodGet, "/api/v1/settings", false},
		{http.MethodPost, "/api/v1/ics-token/regenerate", false},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			reached, code, body := reachedHandler(mux, tc.method, tc.path)
			if reached != tc.allowed {
				t.Fatalf("reached handler = %v, want %v (status %d, body %q)",
					reached, tc.allowed, code, body)
			}
		})
	}
}

// reachedHandler reports whether a machine-token request got PAST the auth
// middleware.
//
// machineTestServer wires no document usecases, so an allowed route runs into
// a zero-valued usecase and may panic inside its handler. That panic IS the
// signal this test wants — the request demonstrably left the middleware — and
// is recovered here rather than papered over by wiring a full set of fakes,
// which would test the handlers rather than the opt-in list.
func reachedHandler(mux http.Handler, method, path string) (reached bool, code int, body string) {
	defer func() {
		if recover() != nil {
			reached = true
		}
	}()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer t")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	code, body = rec.Code, rec.Body.String()
	reached = !(code == http.StatusForbidden &&
		strings.Contains(body, "machine tokens are not accepted on this route"))
	return
}
```

Import-Block dieser Datei:

```go
import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/ports"
)
```

`(*Server).Routes() http.Handler` ist der Routen-Konstruktor
(`server.go:154`); `context_test.go:17` baut Tests bereits genauso.

- [ ] **Step 2: Test laufen lassen, Fehlschlag bestätigen**

Run: `go test ./internal/adapter/httpserver/ -run TestMachineRouteMatrix`
Expected: FAIL — alle sechs `allowed`-Fälle antworten 403, weil noch keine Route freigeschaltet ist.

- [ ] **Step 3: Die sechs Routen umstellen**

In `internal/adapter/httpserver/server.go` genau diese sechs Zeilen von `s.auth(` auf `s.authMachineOK(` ändern — **keine weiteren**:

```go
	mux.Handle("GET /api/v1/me", s.authMachineOK(http.HandlerFunc(s.handleMe)))
	mux.Handle("POST /api/v1/documents", s.authMachineOK(http.HandlerFunc(s.handleCreateDocument)))
	mux.Handle("GET /api/v1/documents", s.authMachineOK(http.HandlerFunc(s.handleListDocuments)))
	mux.Handle("GET /api/v1/documents/{id}", s.authMachineOK(http.HandlerFunc(s.handleGetDocument)))
	mux.Handle("PUT /api/v1/documents/{id}", s.authMachineOK(http.HandlerFunc(s.handleUpdateDocument)))
	mux.Handle("PATCH /api/v1/documents/{id}", s.authMachineOK(http.HandlerFunc(s.handlePatchDocument)))
```

- [ ] **Step 4: Test laufen lassen, Erfolg bestätigen**

Run: `go test ./internal/adapter/httpserver/ -run TestMachineRouteMatrix`
Expected: PASS

- [ ] **Step 5: Zählprobe**

Run: `rg -c "s\.authMachineOK\(" internal/adapter/httpserver/server.go`
Expected: `6` — mehr bedeutet, dass eine Route versehentlich mit umgestellt wurde.

- [ ] **Step 6: Ganze Suite**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/httpserver/
git commit -m "feat(httpserver): sechs Dokument-Routen fuer Maschinen-Token freischalten

POST/GET /documents, GET/PUT/PATCH /documents/{id}, GET /me. Loeschen,
Import, by-path, pin, archive, move, context-mode sowie saemtliche Node-,
Worktime- und Settings-Routen bleiben Maschinen verschlossen."
```

---

### Task 6: `main.go`-Wiring und Doku

**Files:**
- Modify: `cmd/flow-server/main.go:61-62, 103-111`
- Modify: `README.md` (oder die Betriebsdoku, in der die übrigen `FLOW_*`-Variablen stehen — **Schritt 1 klärt, welche**)

**Interfaces:**
- Consumes: `config.MachineAccounts` (Task 3), `oidcverify.PairConfig` (Task 2), `httpserver.MachineAccount` (Task 4)

- [ ] **Step 1: Den Ort der Env-Doku bestimmen**

Run: `rg -ln "FLOW_ALLOWED_GROUPS|FLOW_OIDC_CLI_CLIENT_ID" --glob '*.md'`

Der Abschnitt aus Schritt 4 gehört in die Datei, die diese Variablen bereits dokumentiert. Gibt es keine, kommt er in `README.md` unter die Konfigurationsübersicht.

- [ ] **Step 2: `VerifierPairs`-Aufruf vervollständigen**

`cmd/flow-server/main.go`, den in Task 2 Schritt 10 geschriebenen Aufruf erweitern:

```go
	verifier, err := oidcverify.New(ctx, oidcverify.VerifierPairs(oidcverify.PairConfig{
		WebIssuer: cfg.OIDCIssuer, WebClient: cfg.OIDCClientID,
		CLIIssuer: cfg.OIDCCliIssuer, CLIClient: cfg.OIDCCliClientID,
		MachineIssuer: cfg.OIDCMachineIssuer, MachineClient: cfg.OIDCMachineClientID,
	}))
```

- [ ] **Step 3: `Server.Machines` befüllen**

Direkt **vor** `srv := &httpserver.Server{` (`main.go:103`):

```go
	// Translate config → adapter. The httpserver package must not import
	// config (adapter → usecase → ports ← domain), so the mapping happens here.
	machines := make(map[string]httpserver.MachineAccount, len(cfg.MachineAccounts))
	for sub, acct := range cfg.MachineAccounts {
		machines[sub] = httpserver.MachineAccount{OwnerSub: acct.OwnerSub, Label: acct.Label}
	}
```

Und im `Server`-Literal, nach `Verifier: verifier,`:

```go
		Machines: machines,
```

- [ ] **Step 4: Doku-Abschnitt schreiben**

In die in Schritt 1 bestimmte Datei:

```markdown
### Headless-Clients authentifizieren

Ein Dienst ohne Browser (CI-Job, CronJob, Runner) meldet sich mit einem
`client_credentials`-Token eines Authentik-Service-Accounts an. flow legt für
ihn **keinen eigenen Benutzer** an — das Token wird auf einen bestehenden
Besitzer delegiert und schreibt in dessen Tenant.

Drei Variablen, alle drei zusammen oder keine:

| Variable | Bedeutung |
|---|---|
| `FLOW_OIDC_MACHINE_ISSUER` | Issuer des `flow-machine`-Providers |
| `FLOW_OIDC_MACHINE_CLIENT_ID` | dessen Audience |
| `FLOW_MACHINE_ACCOUNTS` | `<maschinen-sub>=<besitzer-sub>:<label>`, kommasepariert |

Der Besitzer wird über seinen **OIDC-Sub** adressiert, nicht über den
Benutzernamen — nur `users.oidc_sub` ist eindeutig. Es ist derselbe Wert, der
auch in `FLOW_ALLOWED_SUBS` steht. Das `<label>` erscheint im Aktivitätsfeed als
Urheber und in Fehlermeldungen.

Token holen:

```bash
curl -fsS -X POST https://<authentik>/application/o/token/ \
  -d grant_type=client_credentials \
  -d client_id=flow-machine \
  -d username=<service-account> \
  -d password="$FLOW_SA_PASSWORD" \
  -d scope=openid | jq -r .access_token
```

Das Token lebt rund eine Stunde. Ein lang laufender Job holt es deshalb
**unmittelbar vor** dem Aufruf, nicht beim Start.

**Was ein Maschinen-Token darf:** Dokumente anlegen, lesen und ändern
(`POST`/`GET /api/v1/documents`, `GET`/`PUT`/`PATCH /api/v1/documents/{id}`)
sowie `GET /api/v1/me`. Alles andere — Löschen, Nodes, Zeiterfassung,
Einstellungen — antwortet 403.

**Fehlerbilder:**

| Antwort | Bedeutung |
|---|---|
| `401 invalid token` | Signatur, Issuer, Audience oder Gültigkeit stimmen nicht |
| `403 machine token not mapped to an owner` | Der Sub fehlt in `FLOW_MACHINE_ACCOUNTS` |
| `403 machine account "<label>" maps to an unknown owner` | Der Besitzer-Sub hat keine Benutzerzeile — der Besitzer muss sich einmal angemeldet haben |
| `403 machine tokens are not accepted on this route` | Die Route ist für Maschinen nicht freigegeben |
```

- [ ] **Step 5: Build + volle Suite**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 6: `make ci`**

Run: `make ci`
Expected: grün

- [ ] **Step 7: Commit**

```bash
git add cmd/flow-server/main.go README.md
git commit -m "feat(server): Maschinen-Auth verdrahten und dokumentieren

Uebersetzt config.MachineAccounts nach httpserver.MachineAccount — die
Adapter-Schicht importiert config bewusst nicht — und reicht Issuer und
Audience des flow-machine-Providers an VerifierPairs durch.

Dazu ein Doku-Abschnitt 'Headless-Clients authentifizieren' mit den drei
Variablen, dem Token-curl, dem erlaubten Routen-Set und den vier
Fehlerbildern."
```

---

### Task 7: Runner-Seite — Token just in time in der `report`-Rolle

**Repo:** `serverkraken/wartung` (**nicht** dieses). Eigener Branch dort.

**Files:**
- Modify: `ansible/roles/report/tasks/main.yml` (neuer Schritt vor Step 5a; Step 5a; die `report_flow_error_note`-Aufgabe)
- Modify: `ansible/roles/report/defaults/main.yml`
- Modify: der CronJob-/Secret-Manifest-Ort von `ANTHROPIC_API_KEY` (Schritt 1 ermittelt ihn)

**Interfaces:**
- Consumes: die flow-Seite aus Task 6 sowie den Authentik-Befund aus Task 1.
- Produces: `report_flow_token` (Fact), gesetzt nur bei erfolgreichem Austausch.

- [ ] **Step 1: Ort der Secrets und der Play-Variablen ermitteln**

```bash
cd ../wartung
rg -n "ANTHROPIC_API_KEY|GEMINI_API_KEY" --glob '!ansible/roles/report/*'
rg -n "flow_url" ansible/
```

Die neuen Variablen gehören exakt dorthin, wo `ANTHROPIC_API_KEY` und `flow_url` heute stehen.

- [ ] **Step 2: Variablenvertrag der Rolle erweitern**

In `ansible/roles/report/tasks/main.yml`, im ersten `assert`-Task die Liste ergänzen:

```yaml
    - flow_token_url is defined
```

und die `fail_msg` entsprechend um `flow_token_url` erweitern.

- [ ] **Step 3: Timeout-Default ergänzen**

Ans Ende von `ansible/roles/report/defaults/main.yml`:

```yaml
# Step 5a-pre's token exchange. Same reasoning as report_flow_timeout: one
# small REST call against an internal service, bounded so a hung IdP cannot
# stall the report phase.
report_flow_token_timeout: 20
```

- [ ] **Step 4: Den Austauschschritt einfügen**

In `ansible/roles/report/tasks/main.yml` **unmittelbar vor** dem Task
`"Step 5a: write the long report as a flow document"`:

```yaml
# Step 5a-pre — machine token for flow, fetched JUST IN TIME.
#
# Deliberately here and not in the CronJob's own wrapper: a full run takes up
# to 27.5 h and an access token lives ~1 h, so a token minted at job start is
# roughly 26 h dead by the time this phase runs — which is exactly the 401 this
# whole change exists to remove. Phase 7 runs inside site.yml's always: block
# at the very end, so "just in time" is structural here, not a deadline someone
# has to keep honouring.
#
# Same idiom as every other uri in this role (status_code + failed_when: false,
# then read .status afterwards): the exchange must never be able to fail the
# run (ruling 3). no_log because the body carries the service account password.
- name: "Step 5a-pre: exchange the service-account password for a flow token"
  ansible.builtin.uri:
    url: "{{ flow_token_url }}"
    method: POST
    body_format: form-urlencoded
    body:
      grant_type: client_credentials
      client_id: "{{ lookup('ansible.builtin.env', 'FLOW_MACHINE_CLIENT_ID') }}"
      username: "{{ lookup('ansible.builtin.env', 'FLOW_MACHINE_USERNAME') }}"
      password: "{{ lookup('ansible.builtin.env', 'FLOW_MACHINE_PASSWORD') }}"
      scope: openid
    timeout: "{{ report_flow_token_timeout }}"
    status_code: [200]
  register: report_flow_token_response
  failed_when: false
  no_log: true
  delegate_to: localhost
  become: false

# .status is read straight off the freshly registered result, never through an
# intermediate set_fact — this ansible-core re-templates a set_fact's value
# through a plain string, so a numeric status comes back out as "200" and every
# `== 200` silently evaluates false. Same trap documented at report_claude_ok.
- name: "Step 5a-pre: adopt the token only when the exchange actually worked"
  ansible.builtin.set_fact:
    report_flow_token: "{{ report_flow_token_response.json.access_token }}"
  when: >-
    (report_flow_token_response.status | default(-1)) == 200 and
    (report_flow_token_response.json.access_token | default('') | length) > 0
  no_log: true
  delegate_to: localhost
  become: false
```

- [ ] **Step 5: Step 5a auf das Fact umstellen**

Im Task `"Step 5a: write the long report as a flow document"`:

```yaml
    headers:
      Authorization: "Bearer {{ report_flow_token }}"
      content-type: application/json
```

und ans Ende desselben Tasks (neben `delegate_to`/`become`):

```yaml
  when: report_flow_token is defined
```

- [ ] **Step 6: Die Notiz um die neuen Fälle erweitern**

Der Task `"Step 5a: note the flow write's outcome — never fails the run"`
bekommt drei Zweige vorangestellt. Der bestehende Ausdruck bleibt als
`else`-Zweig erhalten; nur der Kopf ändert sich:

```yaml
- name: "Step 5a: note the flow write's outcome — never fails the run"
  ansible.builtin.set_fact:
    report_flow_error_note: >-
      {{ ('Flow token exchange failed (http_status='
          ~ (report_flow_token_response.status | default(-1) | string)
          ~ '). No token, so the long report was NOT persisted to flow this '
          ~ 'run. Check FLOW_MACHINE_* in the job environment and the '
          ~ 'flow-machine service account in Authentik.')
         if report_flow_token is not defined else
         ('Flow doc write failed: flow rejected the machine token (401). The '
          ~ 'token was obtained but flow did not accept it — check '
          ~ 'FLOW_OIDC_MACHINE_ISSUER/CLIENT_ID on the server.'
          if (report_flow_response.status | default(-1)) == 401 else
         ('Flow doc write failed: the machine account is not permitted (403): '
          ~ (report_flow_response.content | default('') | trim)
          ~ ' The long report was NOT persisted to flow this run.'
          if (report_flow_response.status | default(-1)) == 403 else
         (…bestehender Ausdruck, unverändert…))) }}
  delegate_to: localhost
  become: false
```

Damit die 403-Meldung den Grund überhaupt enthalten kann, braucht Step 5a
zusätzlich `return_content: true` — dieselbe Lektion wie bei Step 5c am
2026-08-01, wo hinter einem 502 die eigentliche Ursache nur im Rumpf stand.

- [ ] **Step 7: Secrets ins Job-Environment**

`FLOW_MACHINE_CLIENT_ID`, `FLOW_MACHINE_USERNAME`, `FLOW_MACHINE_PASSWORD` aus
sops → k8s-Secret → CronJob-Env, an derselben Stelle wie `ANTHROPIC_API_KEY`
(Schritt 1). `FLOW_TOKEN` ersatzlos entfernen:

```bash
rg -n "FLOW_TOKEN" .
```

Expected: keine Treffer mehr außer im Changelog/in Kommentaren.

- [ ] **Step 8: Trockenlauf**

```bash
ansible-playbook ansible/site.yml --check -e mode=check
```

Expected: die Rolle läuft durch; unter `--check` werden alle `uri`-Tasks
übersprungen, der Austausch also auch — `report_flow_token` bleibt undefiniert
und die Notiz meldet den fehlgeschlagenen Austausch. Das ist korrektes
Verhalten, kein Fehler: es beweist, dass der `when:`-Guard an Step 5a greift.

- [ ] **Step 9: Commit (im wartung-Repo)**

```bash
git add ansible/
git commit -m "feat(report): flow-Token just in time statt FLOW_TOKEN aus dem Env

Der Austausch sitzt unmittelbar vor Step 5a statt im CronJob-Wrapper: ein
Full-Lauf dauert bis 27,5 h, ein Access-Token lebt ~1 h. Phase 7 laeuft
in site.ymls always:-Block ganz am Ende, damit ist just-in-time
strukturell statt terminlich."
```

---

### Task 8: Integrations-Gate

**Files:** keine — dies ist die Abnahme.

- [ ] **Step 1: `make ci` im flow-Worktree**

Run: `make ci`
Expected: grün

- [ ] **Step 2: Server mit Maschinen-Auth starten**

Die drei Variablen aus dem Task-1-Befund setzen, Server starten. Erwartet: startet sauber. Dann eine Variable entfernen und neu starten — erwartet: **lauter Startfehler**, der die fehlende Variable nennt.

- [ ] **Step 3: Live-Smoke gegen die laufende Instanz**

Mit einem frisch geholten Token (curl aus Task 1):

```bash
TOKEN=$(curl -fsS -X POST "$AUTHENTIK/application/o/token/" \
  -d grant_type=client_credentials -d client_id=flow-machine \
  -d username=wartung-agent -d password="$FLOW_SA_PASSWORD" \
  -d scope=openid | jq -r .access_token)

# 1) erlaubt → 201
curl -isS -X POST "$FLOW/api/v1/documents" \
  -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' \
  -d '{"type":"free","path":"notes/runs/smoke-1","title":"Smoke","body":"x","tags":["smoke"]}' | head -1

# 2) verboten → 403 + Routentext
curl -isS -X DELETE "$FLOW/api/v1/documents/<id-aus-1>" \
  -H "Authorization: Bearer $TOKEN" | head -3

# 3) verbotener Bereich → 403
curl -isS "$FLOW/api/v1/nodes" -H "Authorization: Bearer $TOKEN" | head -3

# 4) kaputtes Token → 401
curl -isS "$FLOW/api/v1/me" -H "Authorization: Bearer ${TOKEN}x" | head -1
```

- [ ] **Step 4: Provenance prüfen**

Das in Schritt 3 angelegte Dokument in der Web-UI öffnen. Erwartet: als Urheber steht **`wartung-agent`** (Maschine), nicht Soenne. Danach das Smoke-Dokument löschen.

- [ ] **Step 5: Menschliche Pfade gegenprüfen**

Browser-Login und `flow login` (device_code) einmal durchspielen. Erwartet: unverändert.

- [ ] **Step 6: Echter Runner-Lauf**

Den CronJob manuell auslösen. Erwartet: der Kurzbericht in Chat meldet
`Flow doc write: OK (notes/runs/<run_id>)`, und das Dokument existiert.

- [ ] **Step 7: Spec-Akzeptanzkriterien abhaken**

`docs/superpowers/specs/2026-08-01-machine-auth-design.md` §13 durchgehen und die Haken setzen. Offene Punkte bleiben offen und werden benannt, nicht stillschweigend abgehakt.

- [ ] **Step 8: Commit + PR**

```bash
git add docs/superpowers/specs/2026-08-01-machine-auth-design.md
git commit -m "docs(spec): Akzeptanzkriterien nach dem Live-Gate abgehakt"
git push -u origin machine-auth
gh pr create --title "feat: Maschinen-Auth für headless Clients (wartung-Runner)" --body "…"
```

---

## Selbstprüfung des Plans

**Spec-Abdeckung:**

| Spec | Task |
|---|---|
| §4.1 Audience-Struct + Machine-Flagge | 2 |
| §4.2 `PairConfig`, Prod/Dev/aus | 2 |
| §4.3 `Identity.Machine` | 2 |
| §5 Default-Deny, `authMachineOK` | 4 |
| §5.1 Opt-in-Liste (6 Routen) | 5 |
| §5.2 Ablauf, kein `EnsureUser` für Maschinen | 4 |
| §5.3 Allowlist = Mapping | 3 (Parsing) + 4 (Nutzung) |
| §6 Config, alle Regeln | 3 |
| §6.1 Besitzer per Sub | 3 |
| §7 `TrustedMachine`, Audit | 4 |
| §8 vier Fehlerbilder | 4 (drei) + 4/`authWith` (401) |
| §9 Authentik-Vertrag | 1 |
| §9.1 Vorab-Verifikation | 1 |
| §10 Runner | 7 |
| §11 Tests | 2, 3, 4, 5 |
| §13 Akzeptanzkriterien | 8 |

**Typkonsistenz geprüft:** `Audience`/`IssuerAudiences`/`PairConfig` (Task 2) werden in Task 6 mit denselben Feldnamen benutzt. `config.MachineAccount{Sub, OwnerSub, Label}` (Task 3) und `httpserver.MachineAccount{OwnerSub, Label}` sind bewusst **verschieden** — die Adapter-Variante trägt den Sub nicht, weil er dort schon der Map-Schlüssel ist; Task 6 Schritt 3 übersetzt feldweise. `Server.Machines` ist in Task 4 definiert und in den Tasks 4, 5 und 6 identisch benannt.

**Bekannte Unschärfen, bewusst stehen gelassen:**

1. **Task 7 Schritt 6** kürzt den bestehenden Ausdruck mit `…bestehender Ausdruck, unverändert…` ab. Das ist Absicht: der Ausdruck steht wörtlich in `report/tasks/main.yml` und wird umschlossen, nicht neu geschrieben — ihn hier zu duplizieren wäre eine zweite Kopie, die driften kann.
2. **Task 1 kann diesen Plan umwerfen.** Liefert Authentik ein opakes Token, sind die Tasks 2–6 neu zu schneiden. Deshalb steht Task 1 vorn und hat ein ausdrückliches STOP.
3. **Task 6 Schritt 1** lässt offen, in welche Markdown-Datei der Doku-Abschnitt gehört, und ermittelt es per `rg`. Der Abschnitt selbst steht wörtlich da — nur sein Zielort hängt vom Bestand ab.
