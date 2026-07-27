# flow Rebuild · M1b — Device-Flow-Login · Design

**Datum:** 2026-06-14
**Status:** Draft — Brainstorm abgeschlossen, wartet auf User-Review
**Branch:** `rebuild` (Worktree `flow-rebuild`)
**Scope:** Den manuellen `FLOW_TOKEN`-Env-Paste der CLI/TUI durch echtes `flow login` (OIDC Device-Flow, RFC 8628) ersetzen — Token im OS-Keyring (File-Fallback), Silent-Refresh, alle Client-Commands nutzen das gespeicherte Token.
**Voraussetzung:** M0 (Spine) + M1a (Worktime live-sync) — beide DONE auf `rebuild`.

## Problem

Heute authentifiziert sich die CLI/TUI über die Env-Variable `FLOW_TOKEN` (`cmd/flow/whoami.go:17-19` sagt explizit „device-flow login comes in M1"). Das Token wird manuell via `scripts/dev-token.sh` (OAuth2-Password-Grant gegen Dex) gemintet und exportiert. Kein Refresh, kein Logout, kein persistentes Login. M1b liefert den vollständigen Login-Lifecycle.

## Kern-Entscheidungen (Brainstorm 2026-06-14)

| Frage | Entscheidung |
|---|---|
| Scope | **Voller Lifecycle:** `flow login` / `flow logout` / `flow whoami` + alle Folge-Commands nutzen das gespeicherte Token; Silent-Refresh; `FLOW_TOKEN` bleibt als CI-Override. MCP bleibt M3, teilt aber den Adapter. |
| Bearer-Token | **access_token** (konventionell für APIs) + Refresh via refresh_token. |
| Storage | **Keyring primär** (go-keyring, Felder gesplittet wg. macOS-2KB) **+ 0600-File-Fallback**; `FLOW_TOKEN`-Env übersteuert alles. |
| CLI-Client | **Dedizierter public Client `flow-cli`** (kein Secret) + **Multi-Audience-Verifier** serverseitig. |
| Lib | **`golang.org/x/oauth2` Bordmittel** (`DeviceAuth`/`DeviceAccessToken`/`TokenSource`) statt eigenem RFC-8628-Code. |

### Empirisch verifiziert beim Brainstorm

- Dex (`deploy/dev/dex.yaml`) bewirbt `urn:ietf:params:oauth:grant-type:device_code` + `device_authorization_endpoint` und liefert `/device/code` für `flow-dev` (mit und ohne Secret) → vollständige `user_code`/`verification_uri_complete`-Response.
- Dex' **access_token ist ein JWKS-verifizierbares JWT** mit `aud: flow-dev` (== clientID), `iss`, `sub`, `email`, `name` — exakt was `oidcverify` prüft. (Kein `preferred_username`/`groups` von Dex → Allowlist via `sub`, wie bestehender Dev-Setup.)
- `golang.org/x/oauth2 v0.36.0` hat `Config.DeviceAuth` + `Config.DeviceAccessToken` (inkl. `slow_down`/Polling) und `Config.TokenSource` (Refresh). `go-oidc/v3 v3.18.0` ist vorhanden; `go-keyring` muss als Dependency dazu.

## Architektur — neue/geänderte Bausteine

Prinzip „keine Monolithen": ein File pro Verantwortung.

### Neuer Adapter `internal/adapter/oidcdevice/`

Device-Flow-Client über go-oidc-Discovery + x/oauth2.

- `New(ctx, issuer, clientID) (*Flow, error)` — discovert Endpoints. Da `provider.Endpoint()` das `device_authorization_endpoint` **nicht** setzt, wird es via `provider.Claims(&extra)` aus dem Discovery-Doc gelesen und in `oauth2.Endpoint.DeviceAuthURL` gelegt. Baut `oauth2.Config` (public Client, **kein Secret**, Scopes `openid profile email offline_access`).
- `Start(ctx) (*oauth2.DeviceAuthResponse, error)` → `cfg.DeviceAuth`.
- `Poll(ctx, da) (*oauth2.Token, error)` → `cfg.DeviceAccessToken` (blockt bis Approve/Timeout, behandelt Intervall/`slow_down`).
- `TokenSource(ctx, tok) oauth2.TokenSource` → `cfg.TokenSource` (Refresh über refresh_token).

### Neuer Port + Adapter `internal/adapter/tokenstore/`

`ports.TokenStore { Save(Token) error; Load() (Token, bool, error); Clear() error }` mit `Token{ AccessToken, RefreshToken string; Expiry time.Time }`.

- `keyring.go` — go-keyring-Impl, **Felder gesplittet** (Service `flow`, separate Items `access_token` / `refresh_token` / `expiry`) wg. macOS-~2-KiB-pro-Item-Limit.
- `file.go` — `0600`-JSON unter `~/.config/flow/token.json`.
- `store.go` — `Open() ports.TokenStore`: probt Keyring (Test-Write/Delete), fällt bei Fehler auf File zurück.

### Client-Config `internal/clientconfig/`

`FLOW_SERVER_URL` (default `http://localhost:8080`), `FLOW_OIDC_ISSUER`, `FLOW_OIDC_CLI_CLIENT_ID` (default `flow-cli`). Spiegelt das serverseitige `internal/config`-Pattern (getenv-injizierbar, testbar). `FLOW_OIDC_CLI_ISSUER` als optionaler Override für M5 (eigener Authentik-Provider) vorgesehen, in Dev = `FLOW_OIDC_ISSUER`.

### `cmd/flow/auth.go` — gemeinsamer Session-Helper

`clientFromStore(ctx) (*apiclient.Client, error)`:
1. `FLOW_TOKEN` gesetzt → statischer Token-`http.Client` (CI, kein Refresh).
2. Sonst `tokenstore.Open()` → Token laden; keins → Fehler „run `flow login`".
3. Persistierende `oauth2.TokenSource`: wrappt `oidcdevice.TokenSource`; bei geändertem AccessToken sofort `store.Save`.
4. `apiclient.New(base, oauth2.NewClient(ctx, persistingSource))`.

### `apiclient.New(base string, hc *http.Client)`

Signatur von Token-String auf `*http.Client` umgestellt. Der oauth2-Transport setzt Bearer + refresht automatisch; `do` / `Whoami` / `events.go` setzen den `Authorization`-Header nicht mehr selbst (DRY, fixt SSE-Auth einheitlich). Bestehende Tests (`client_test.go`, `worktime_test.go`) ziehen auf den neuen Constructor nach.

## Server-Change — Multi-Audience-Verifier

`internal/adapter/oidcverify`: von Single- auf Multi-Audience. `oidc.Config{ SkipClientIDCheck: true }`, danach manueller Check `aud ∈ {FLOW_OIDC_CLIENT_ID, FLOW_OIDC_CLI_CLIENT_ID}`. `New(ctx, issuer, audiences []string)`. `internal/config` bekommt `OIDCCliClientID` (`FLOW_OIDC_CLI_CLIENT_ID`, required), `cmd/flow-server/main.go` reicht beide Audiences durch. `middleware.go`/`auth` bleiben unverändert (rufen weiter `Verify`).

Issuer bleibt in Dev derselbe (Dex) → **ein** Verifier reicht. Multi-Issuer (eigener Authentik-CLI-Provider mit `per_provider` issuer_mode) bleibt **M5-Carry**.

## Flows

### `flow login`
1. `clientconfig` laden → `oidcdevice.New`.
2. `Start` → druckt `user_code` groß, `verification_uri_complete` (klickbar) und Fallback (`verification_uri` + Code).
3. `Poll` (Spinner) bis Approve.
4. `/api/v1/me` callen → Identität; Token via `store.Save` persistieren.
5. „Logged in as \<DisplayName\> \<email\>".
6. Fehlerfälle: `expired_token` (zu langsam) → Hinweis erneut zu versuchen; `access_denied` → klare Meldung.

### `flow logout`
`store.Clear()` → Bestätigung. Idempotent (kein Token → trotzdem ok).

### `flow whoami` + Folge-Commands
Über `clientFromStore`. Kein Token → „not logged in — run `flow login`". Der bisherige `FLOW_TOKEN`-Pflichtfehler in `whoami.go` entfällt.

## Dev-Env

- `deploy/dev/dex.yaml`: public Client `flow-cli` (`public: true`, kein Secret, device_code+refresh).
- `scripts/dev-run.sh`: setzt `FLOW_OIDC_CLI_CLIENT_ID=flow-cli` für den Server.
- `scripts/dev-token.sh` (Password-Grant) bleibt für CI/automatisierte Smokes.

## Tests & Done-Gate

**Unit:**
- `oidcdevice`: httptest-Stub für discovery + `/device/code` + `/token`; Happy-Path (Start→Poll→Token) + Endpoint-Discovery (`device_authorization_endpoint` korrekt übernommen).
- `tokenstore`: go-keyring `MockInit` (Roundtrip + `Clear`), File (`0600`-Perms, Roundtrip, `Clear`, Open-Fallback wenn Keyring fehlt).
- Persistierende Source: Refresh liefert neues AccessToken → `store.Save` wurde gerufen.
- `oidcverify`: Token mit `aud=flow-cli` passt, `aud=unbekannt` scheitert, Multi-Aud beibehält Signatur/iss/exp-Checks.
- `apiclient`: neuer `*http.Client`-Constructor; bestehende Tests angepasst.

**Wiring-Verifikation** (Lehre aus früheren Plänen — Composition-Root muss die neuen Konstruktoren wirklich aufrufen):
- Smoke/Test: ein über den `flow-cli`-Client gemintetes access_token gibt an `/api/v1/me` 200; `cmd/flow-server/main.go` verdrahtet beide Audiences.

**Manuelles Done-Gate (Dogfood):**
1. `flow login` gegen Dev-Dex → Browser-Approve → „Logged in as …".
2. `flow whoami` **ohne** `FLOW_TOKEN` → korrekter User.
3. Token persistiert (Keyring bzw. File); Prozess-Neustart → weiterhin authentifiziert (ggf. nach Refresh).
4. `flow logout` → `flow whoami` scheitert mit „run `flow login`".

`make ci` grün (Coverage-Gate halten).

## Mitgeführte Stolperfallen

- **macOS-Keyring ~2 KiB/Item** → Felder gesplittet (oben ✓).
- **`go-keyring-base64:`-Prefix** → nur relevant bei Inspektion via `security`-CLI; eigener Code liest via go-keyring, kein Stripping nötig.
- **oauth2-Polling** behandelt `slow_down` selbst; wir fangen `expired_token`/`access_denied` explizit ab.
- **Authentik-Prod (M5):** public Client + `device_code`-Grant + brand-level Device-Flow mit `authentication: none`; access_token-`aud` muss clientID enthalten; `per_provider` issuer_mode → ggf. Multi-Verifier.

## Non-Goals (M1b)

- **MCP** auf den Device-Token umstellen → M3 (Adapter wird aber wiederverwendet).
- **Multi-Issuer** (separater Authentik-CLI-Provider) → M5.
- **Browser-Auto-Open** beim Login → optional/später; M1b druckt URL+Code.
- **Token-Verschlüsselung im File-Fallback** → 0600 reicht (Klartext bewusst akzeptiert).

## Offene Punkte (für den Plan)

- Verifizieren, dass Dex für den **public** `flow-cli`-Client im Device-Flow mit `offline_access` ein **refresh_token** ausstellt (erster Plan-Task, empirisch).
- Browser-Auto-Open: weglassen oder optionales `--open`-Flag.
