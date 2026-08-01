# Maschinen-Auth für headless Clients (Design Spec)

- **Datum:** 2026-08-01
- **Branch:** `machine-auth` (Worktree `../flow-machine-auth`, von `main` @ `532f842`)
- **Status:** Draft — zur Review
- **Umfang:** End-to-End über zwei Repos — `serverkraken/flow` (Verifier,
  Middleware, Config, Actor) und `serverkraken/wartung` (Token-Austausch in der
  `report`-Rolle). Die Authentik-Seite steht als normativer Vertrag in §9;
  umgesetzt wird sie im Infra-Repo, nicht hier.
- **Auslöser:** [[notes/fr-machine-auth-wartung-runner]] (flow-Kompendium)
- **Verwandt:** [[plans/2026-07-29-wartung-agent]] Task 17 (Memories des
  Runners), [[plans/2026-07-30-wartung-ansible]] (der Runner ist heute
  Ansible, kein Claude-Code-Agent mehr).

---

## 1. Kontext & Ziel

Der Wartungs-Runner (K8s-CronJob, Repo `serverkraken/wartung`) schreibt am Ende
jedes Laufs einen Lauf-Report als flow-Dokument. Der Aufruf steht seit Plan 3 im
Code und funktioniert in allem außer der Authentifizierung:

```yaml
# wartung/ansible/roles/report/tasks/main.yml, Step 5a
- name: "Step 5a: write the long report as a flow document"
  ansible.builtin.uri:
    url: "{{ flow_url }}/api/v1/documents"
    headers:
      Authorization: "Bearer {{ lookup('ansible.builtin.env', 'FLOW_TOKEN') }}"
```

`FLOW_TOKEN` ist leer, und es gibt keinen Weg, es zu füllen: der
`flow-cli`-Provider deklariert `grant_types: [device_code, refresh_token]`, und
flow kennt kein eigenes Service-Token-Konzept. Jeder Lauf endet mit
`Flow doc write failed (http_status=401)` — zuletzt real beobachtet am
2026-08-01, Run `wartung-check-m6w4l`.

Der Runner ist dabei bereits so gebaut, dass ihn dieser Fehlschlag nicht rot
macht (Ruling 3 der Rolle): die Notiz landet im Kurzbericht, der Lauf bleibt
grün. Das Ergebnis ist trotzdem, dass **der Langbericht seit Inbetriebnahme
nirgends existiert** — der Operator liest jede Nacht einen Verweis auf ein
Dokument, das nie geschrieben wurde.

### Erfolgskriterien

1. Ein `client_credentials`-Token des Service-Accounts `wartung-agent` gegen
   `POST /api/v1/documents` liefert **201**, und das Dokument liegt unter
   `notes/runs/<run_id>` in **Soennes** Tenant.
2. Am geschriebenen Dokument steht Maschinen-Provenance (`updated_by_kind =
   agent`, `updated_by_ref = wartung-agent`), nicht Soennes Name.
3. Dasselbe Token gegen jede nicht freigegebene Route liefert **403** mit einem
   Körper, der sagt warum.
4. Ein abgelaufenes oder manipuliertes Token liefert **401**.
5. Menschliche Tokens (Browser, CLI) verhalten sich unverändert.
6. Der Runner tauscht sein Token **just in time vor dem Report** und schreibt
   den Langbericht erfolgreich.

---

## 2. Entscheidungen (Soenne, 2026-08-01)

| Frage | Entscheidung | Verworfen |
|---|---|---|
| Tenant | **Delegation** — der Runner schreibt in Soennes Tenant | Eigener Tenant für den Service-Account |
| Mapping | **Server-Env** `FLOW_MACHINE_ACCOUNTS` | Claim aus Authentik; DB-Tabelle `service_accounts` |
| Rechte | **Feste Maschinen-Rechte**, Dokumente lesen/schreiben | Scope-Vokabular; gar kein Rechtemodell |
| Token-Quelle | **Eigener Provider `flow-machine`** | `client_credentials` an `flow-cli` anhängen |
| Zuschnitt | **End-to-End** inkl. Runner | Nur flow-Code |

Zur Tenant-Frage, weil sie den Rest bestimmt: flow ist owner-scoped, jeder
Handler zieht `userFrom(ctx)` und hängt alles an `u.ID`. Ein per Auto-Provisioning
angelegter Service-Account-User bekäme eine eigene `owner_id` und einen eigenen,
leeren Projektbaum. Die Reports lägen dann in einem Konto, das Soenne nirgends
sieht — flow kennt kein Cross-Tenant-Lesen. Der Service-Account muss also auf
einen **bestehenden** User auflösen.

---

## 3. Kernidee

> „Maschine" ist keine Konfigurationsbehauptung, sondern eine **vom Verifier
> bewiesene Eigenschaft des Tokens**.

Nur ein Token aus dem `flow-machine`-Paar (Issuer + Audience) kann die
Eigenschaft tragen. Aus ihr folgen genau drei Dinge:

1. **Delegation** — der Token löst auf einen bestehenden Besitzer auf.
2. **Ein enges Routen-Set** — alles andere 403.
3. **Actor-Provenance `agent`** — die Maschine ist im Audit sichtbar.

Kein Rechte-Vokabular, keine Migration, kein neuer Store, kein neuer
User-Datensatz.

---

## 4. Verifier (`internal/adapter/oidcverify`)

### 4.1 Die Flagge hängt an der Audience, nicht am Paar

```go
type Audience struct {
	ClientID string
	Machine  bool
}

type IssuerAudiences struct {
	Issuer    string
	Audiences []Audience
}
```

`Verify` stempelt nach der bestehenden Audience-Prüfung `Identity.Machine` aus
der **getroffenen** Audience.

Der naheliegende Entwurf — ein `Machine bool` am `IssuerAudiences` selbst — ist
falsch, und zwar an einer Stelle, die man nur im Code sieht. `Verify` hat die
Invariante *„der erste Issuer, dessen Signatur, `iss` und `exp` aufgehen, besitzt
den Token"*, und schlägt bei einem Audience-Miss **hart fehl statt
weiterzusuchen** (`verifier.go:95-97`):

```go
if !ok {
    return ports.Identity{}, fmt.Errorf("oidcverify: audience %v not allowed for issuer %s", …)
}
```

Zwei Einträge mit demselben Issuer wären damit unbrauchbar: der erste gewinnt,
und wenn seine Audience-Menge nicht passt, ist der Token abgelehnt, obwohl der
zweite Eintrag ihn akzeptiert hätte. Genau das ist der **Dev-Fall** — ein
einzelner Dex-Issuer, der alle drei Clients fronted. Die Flagge muss deshalb pro
Audience getragen werden, dann bleibt es bei einem Eintrag je Issuer und die
Invariante ist unangetastet.

### 4.2 `VerifierPairs` bekommt ein Struct

Heute:

```go
func VerifierPairs(webIssuer, webClient, cliIssuer, cliClient string) []IssuerAudiences
```

Vier gleichtypige Positionsparameter sind schon grenzwertig; sechs sind eine
Vertauschung, die kompiliert. Deshalb:

```go
type PairConfig struct {
	WebIssuer, WebClient         string
	CLIIssuer, CLIClient         string
	MachineIssuer, MachineClient string
}

func VerifierPairs(c PairConfig) []IssuerAudiences
```

Verhalten:

Die Fallunterscheidung hängt an **`MachineClient`**, nicht am Issuer:

- **Maschinen-Auth nicht konfiguriert** (`MachineClient == ""`): kein
  Maschinen-Eintrag, keine Maschinen-Audience, `Identity.Machine` ist nie true.
  Das gesamte Feature ist inert — der Zustand jeder Dev-Umgebung, die es nicht
  braucht, und er muss ohne weitere Konfiguration funktionieren.
- **Prod** (`MachineIssuer != WebIssuer`, Authentik `per_provider`): drei
  Issuer, jeder an genau seinen eigenen Client gebunden. Nur die Audience des
  Maschinen-Paars trägt `Machine: true`.
- **Dev** (`MachineIssuer == WebIssuer`, ein Dex-Issuer): **ein** Eintrag mit
  allen konfigurierten Audiences, davon die Maschinen-Audience mit
  `Machine: true`. Dasselbe gilt unabhängig davon für `CLIIssuer` — die
  bestehende Zusammenfassungsregel (`pairs.go:13`) bleibt, sie gilt jetzt nur
  für bis zu drei Clients statt zwei.

`MachineIssuer` wird also auch in Dev gesetzt (auf den Dex-Issuer); dass §6 alle
drei Variablen gemeinsam verlangt, gilt in beiden Umgebungen gleich.

### 4.3 `ports.Identity`

```go
type Identity struct {
	Subject, Username, Email, Name string
	Groups                         []string
	Machine                        bool // NEU: aus der getroffenen Audience
}
```

---

## 5. Middleware — Default-Deny per Konstruktion

Routen werden einzeln registriert (`server.go:158-241`), das Muster ist zur
Registrierzeit bekannt. Deshalb **kein Pfad-Präfix-Abgleich zur Laufzeit**,
sondern zwei Wrapper:

- `s.auth` und `s.authAny` **weisen Maschinen-Token immer mit 403 ab.**
- `s.authMachineOK` verhält sich wie `s.auth`, akzeptiert aber zusätzlich
  Maschinen-Token.

Eine künftig hinzugefügte Route ist damit maschinen-dicht, **ohne dass jemand
daran denken muss**. Eine Präfix-Tabelle hätte die umgekehrte Eigenschaft: sie
muss bei jeder neuen Route mitgepflegt werden, und `/api/v1/documents/{id}/archive`
gegen ein Präfix `/api/v1/documents` zu prüfen ist genau die Art Vergleich, die
still zu viel erlaubt.

### 5.1 Opt-in-Liste

| Route | Warum |
|---|---|
| `POST /api/v1/documents` | der Lauf-Report; später die Task-17-Memories |
| `GET /api/v1/documents` | Zurücklesen / Idempotenzprüfung |
| `GET /api/v1/documents/{id}` | Zurücklesen |
| `PUT /api/v1/documents/{id}` | Nachtragen |
| `PATCH /api/v1/documents/{id}` | Nachtragen |
| `GET /api/v1/me` | Credential-Smoke ohne Schreibwirkung |

Ausdrücklich **nicht** freigegeben: `DELETE /api/v1/documents/{id}`,
`POST /api/v1/documents/import`, `PUT /api/v1/documents/by-path`, `…/pin`,
`…/archive`, `…/move`, `…/context-mode`, sämtliche Node-, Worktime-, Settings-
und ICS-Routen.

`GET /api/v1/nodes/resolve` bleibt bewusst draußen. Task 17 („Erkenntnisse als
Memories") braucht sie möglicherweise, um die `wartung`-Node-ID aufzulösen — aber
das ist heute nicht entschieden, und Nachrüsten ist per Konstruktion eine Zeile.

### 5.2 Ablauf

```
Verify(token)                  → Fehler: 401 "invalid token"
  ├─ id.Machine == false       → EnsureUser (unverändert), ctxWithUser
  └─ id.Machine == true
       ├─ Route nicht opt-in   → 403 "machine tokens are not accepted on this route"
       ├─ Sub nicht gemappt    → 403 "machine token not mapped to an owner"
       ├─ Owner unbekannt      → 403 `machine account "<label>" maps to an unknown owner`
       └─ sonst                → ctxWithMachine(owner, label)
```

**`EnsureUser` läuft für Maschinen nicht.** Es gibt keinen Maschinen-User; der
Token löst auf den bestehenden Besitzer auf. Das ist kein Sonderfall, den man
umgeht, sondern der Punkt: ein Maschinen-Datensatz wäre ein zweiter Tenant, und
den wollen wir gerade nicht.

**`authAny` ist ebenfalls maschinen-bewusst.** Ein Maschinen-Token dort einfach
als „kein gültiger Bearer" zu behandeln, ließe die Anfrage zur Cookie-Prüfung
durchfallen und endete in einem 401 `unauthorized` — für einen Aufrufer, der
sich sehr wohl authentifiziert hat, eine irreführende Antwort. Stattdessen
derselbe spezifische 403 wie oben.

### 5.3 Allowlist

Ein Eintrag in `FLOW_MACHINE_ACCOUNTS` **ist** die Allowlist. Kein zusätzlicher
Gruppenzwang: ob Authentik `groups` in ein `client_credentials`-Token legt, hängt
am Property-Mapping des Providers und am angeforderten Scope. Das wäre eine
zweite Bedingung, die still brechen kann, ohne dass am Mapping etwas falsch ist —
und sie brächte nichts, was die serverseitige Aufzählung nicht schon leistet.

---

## 6. Konfiguration (`internal/config`)

```sh
FLOW_OIDC_MACHINE_ISSUER=https://id.thebackend.org/application/o/flow-machine/
FLOW_OIDC_MACHINE_CLIENT_ID=flow-machine
FLOW_MACHINE_ACCOUNTS=<sa-sub>=<owner-sub>:wartung-agent
```

`FLOW_MACHINE_ACCOUNTS` ist eine kommaseparierte Liste von
`<maschinen-sub>=<besitzer-sub>:<label>`, geparst wie `FLOW_ALLOWED_SUBS`
(`config.go:40-49`): Leerraum wird getrimmt, leere Einträge übersprungen.

### 6.1 Der Besitzer wird per OIDC-Sub adressiert, nicht per Username

Der erste Entwurf schrieb hier `<username>`. Das ist falsch, und der Grund steht
im Schema: in `0001_users.sql` ist **nur `oidc_sub` UNIQUE**; `username` ist ein
gewöhnliches `TEXT NOT NULL DEFAULT ''`. Eine Auflösung per Username wäre damit
mehrdeutig — und in einer Multi-Tenant-App ist eine mehrdeutige Besitzerauflösung
kein Schönheitsfehler, sondern ein Cross-Tenant-Risiko: zwei User mit demselben
`preferred_username` aus verschiedenen Quellen, und das Maschinen-Token schreibt
in den falschen Tenant.

Der Sub ist dagegen durch ein Datenbank-Constraint eindeutig, `GetBySub` existiert
bereits am Port (`ports.go:20`), im `pgstore` und im Fake — es braucht **keine
neue Store-Methode**. Und es ist ein Wert, den Soenne ohnehin schon pflegt: sein
eigener Sub steht bereits in `FLOW_ALLOWED_SUBS`.

Preis: zwei undurchsichtige Kennungen in einer Zeile. Dagegen steht das `<label>`
am Ende, das den Eintrag für Menschen lesbar hält, und die Fehlermeldungen aus §8.

### 6.2 Regeln

- Alle drei Variablen zusammen oder keine. Teilkonfiguration ist ein
  Startfehler mit Nennung der fehlenden Variablen — nicht ein Server, der
  Maschinen-Token still ablehnt und niemandem sagt warum.
- Ein Eintrag ohne `=` oder ohne `:` ist ein Startfehler, kein stiller
  Übersprung. Ein vertipptes Mapping darf nicht als „nicht gemappt" enden.
- Leerer Maschinen-Sub, leerer Besitzer-Sub oder leeres Label → Startfehler.
- Doppelter Maschinen-Sub → Startfehler.
- **Maschinen-Sub == Besitzer-Sub → Startfehler.** Ein Eintrag, der einen
  Menschen auf sich selbst delegiert, ist immer ein Konfigurationsfehler und
  würde einen menschlichen Token stillschweigend auf Maschinen-Rechte
  herabstufen, sobald er je über den Maschinen-Provider käme.
- Der **Besitzer wird zur Request-Zeit aufgelöst**, nicht beim Start: `config`
  hat keinen DB-Zugriff, und ein Startfehler wegen eines Subs, dessen User-Zeile
  erst beim ersten Browser-Login entsteht, wäre eine Startreihenfolge-Falle. Ein
  unbekannter Besitzer-Sub ergibt den 403 aus §5.2.

---

## 7. Actor & Audit (`internal/actor`)

```go
// TrustedMachine builds audit provenance for a verified machine credential:
// a token from the machine issuer/audience pair whose subject is mapped to an
// owner server-side. Unlike MCP ClientInfo, the label is not caller-controlled.
func TrustedMachine(label string) Actor {
	return Actor{Kind: Agent, Ref: label}
}
```

Der Kommentar an `AuthenticatedHuman` (`actor.go:36-40`) sieht diesen Fall
wörtlich vor: *„Agent is retained for historical entries and future trusted agent
credentials; caller-controlled labels must never be promoted to it."* Genau die
Bedingung ist hier erfüllt — das Label kommt aus der Server-Env, nicht aus dem
Request.

Über `updated_by_kind` / `updated_by_ref` (Migration 0028) steht damit an jedem
vom Runner geschriebenen Dokument, dass eine Maschine es geschrieben hat, und
welche. Im Aktivitätsfeed ist der Runner von Soenne unterscheidbar.

---

## 8. Fehlerbilder

Der Runner zitiert den Antwortkörper wörtlich in seine Chat-Notiz
(`report/tasks/main.yml`, Step 5c seit dem 2026-08-01 mit `return_content: true`,
eingeführt weil hinter einem 502 der eigentliche Grund nur im Rumpf stand). Diese
Texte sind also **Produkt, nicht Debug-Ausgabe** — sie landen morgens um 06:00
auf einem Telefon.

| Situation | Status | Körper |
|---|---|---|
| Signatur/`iss`/`exp`/Audience falsch | 401 | `invalid token` |
| Maschinen-Token, Sub nicht in `FLOW_MACHINE_ACCOUNTS` | 403 | `machine token not mapped to an owner` |
| Mapping zeigt auf unbekannten Besitzer-Sub | 403 | `machine account "<label>" maps to an unknown owner` |
| Maschinen-Token auf nicht freigegebener Route | 403 | `machine tokens are not accepted on this route` |

Klartext über `http.Error`, wie im Rest des Servers. Kein JSON-Fehlerobjekt nur
für diese vier Fälle.

---

## 9. Authentik-Vertrag (normativ, Umsetzung im Infra-Repo)

Neuer OAuth2/OIDC-Provider **`flow-machine`**:

- `grant_types: [client_credentials]` — **kein** `device_code`, kein
  `authorization_code`.
- Issuer-Modus `per_provider` wie die bestehenden beiden.
- Eigene Anwendung mit Policy-Binding, das den Service-Account autorisiert.
- Access-Token-Gültigkeit: der Default (~1 h) genügt, siehe §10.

Service-Account **`wartung-agent`**:

- Deklarativ per Blueprint, App-Passwort aus sops via `!Env` — Präzedenzfall
  `32-users-mail.yaml.j2`.
- **Keine** `flow-users`-Gruppenmitgliedschaft nötig (§5.3).

Der `flow-cli`-Provider bleibt unangetastet reines `device_code` +
`refresh_token`.

### 9.1 Vorab-Verifikation — vor der ersten Zeile Go

Der gesamte Entwurf hängt an einer unbelegten Annahme: dass Authentiks
`client_credentials`-Access-Token ein **JWT** ist, signiert vom Provider, mit
`iss` = Provider-Issuer und `aud` = `flow-machine`. Ist er stattdessen opak oder
trägt eine andere Audience, bricht §4 in seiner Substanz.

```sh
curl -fsS -X POST https://id.thebackend.org/application/o/token/ \
  -d grant_type=client_credentials \
  -d client_id=flow-machine \
  -d username=wartung-agent \
  -d password="$FLOW_SA_PASSWORD" \
  -d scope=openid | jq -r .access_token | cut -d. -f2 | base64 -d | jq
```

Erwartet: ein dekodierbarer Payload mit `iss`, `aud`, `sub`, `exp`. Das ist der
**erste Task des Implementierungsplans**, vor jeder Code-Änderung. Zwanzig
Sekunden gegen sechs Tasks auf falschem Fundament.

Der dort ausgelesene `sub` ist zugleich der Wert, der in
`FLOW_MACHINE_ACCOUNTS` gehört.

---

## 10. Runner-Seite (`serverkraken/wartung`)

### 10.1 Abweichung vom FR, bewusst

Der FR sieht den Token-Austausch im **sh-Wrapper des CronJobs** vor. Das ist
falsch herum: ein Full-Lauf dauert bis 27,5 h, ein Access-Token lebt ~1 h. Beim
Report wäre es rund 26 h tot, und der 401 stünde exakt wieder da, wo er heute
steht.

Stattdessen ein `uri`-Schritt **unmittelbar vor Step 5a, in der `report`-Rolle
selbst**. Phase 7 läuft in `site.yml`s `always:`-Block ganz am Ende — damit ist
„just in time" **strukturell** statt terminlich, und es gibt keine Frist, die
jemand künftig verletzen kann.

### 10.2 Neuer Schritt

Vor Step 5a, mit den Idiomen, die die Rolle für jede andere `uri` bereits
festgelegt hat:

- `status_code: [200]`, `failed_when: false`, danach `.status` lesen — damit
  auch dieser Schritt den Lauf nie rot machen kann (Ruling 3).
- `no_log: true` — der Request trägt das Service-Account-Passwort.
- `.status` **direkt am frisch registrierten Result** lesen, nie über ein
  Zwischen-`set_fact`: die Rolle dokumentiert bei `report_claude_ok`, dass ein
  numerischer Wert dort als String `"200"` zurückkommt und jeder `== 200`-
  Vergleich still falsch wird.
- `body_format: form-urlencoded` (der Token-Endpoint nimmt kein JSON).

Danach:

- `report_flow_token` wird nur gesetzt, wenn der Austausch 200 lieferte.
- Step 5a nimmt `Authorization: "Bearer {{ report_flow_token }}"` statt
  `lookup('env','FLOW_TOKEN')` und läuft nur, wenn das Fact existiert.
- `report_flow_error_note` bekommt zusätzliche Zweige: fehlgeschlagener
  Token-Austausch (mit Status), sowie 401 und 403 von flow getrennt vom
  generischen Fall — die drei Fälle haben verschiedene Ursachen und verschiedene
  Abhilfen, und der Operator liest genau diesen Satz.
- Die Bedingung des Kurzbericht-Zeigers („Full report: flow doc …", heute
  `status in [201, 409]`) bleibt unverändert korrekt: ohne Token gibt es keinen
  201, also auch keinen Zeiger auf ein nicht existierendes Dokument.

### 10.3 Secrets & Defaults

- `FLOW_MACHINE_CLIENT_ID`, `FLOW_MACHINE_USERNAME`, `FLOW_MACHINE_PASSWORD` aus
  sops → k8s-Secret → CronJob-Env, gelesen per `lookup('ansible.builtin.env', …)`
  wie `ANTHROPIC_API_KEY` und `GEMINI_API_KEY`.
- `flow_token_url` als Play-Variable neben `flow_url`, mit demselben
  `assert`-Vertrag am Rollenanfang.
- `report_flow_token_timeout: 20` in `defaults/main.yml`, mit der Begründung von
  `report_flow_timeout`: ein kleiner interner REST-Call.
- `FLOW_TOKEN` entfällt ersatzlos.

---

## 11. Tests

**`oidcverify`** — `verifier_multi_test.go` hat bereits Mehr-Issuer-Infrastruktur:

- Prod-Form: drei Issuer, Maschinen-Token setzt `Machine`, CLI- und Web-Token
  nicht.
- Dev-Form: ein Issuer, drei Audiences, nur die Maschinen-Audience setzt
  `Machine`.
- Nicht konfiguriert: kein Maschinen-Paar, `Machine` nie true.
- Ein Token mit Maschinen-Audience, aber vom CLI-Issuer signiert → abgelehnt.

**`httpserver`**

- Maschinen-Token auf `POST /api/v1/documents` → 201; das Dokument trägt die
  `owner_id` des **Besitzers** und `updated_by_kind = agent`,
  `updated_by_ref = wartung-agent`.
- Maschinen-Token auf `DELETE /api/v1/documents/{id}` → 403 mit dem Routentext.
- Maschinen-Token auf einer `authAny`-Route → 403, **nicht** 401.
- Maschinen-Token mit unbekanntem Sub → 403 mit dem Mapping-Text.
- Mapping auf unbekannten Besitzer-Sub → 403 mit dem Owner-Text.
- Menschen-Token auf denselben Routen → unverändert 200/201.

**`config`** — vollständiges Mapping; Teilkonfiguration; Eintrag ohne `=`;
Eintrag ohne `:`; leeres Feld; doppelter Maschinen-Sub; Maschinen-Sub gleich
Besitzer-Sub; Leerraum.

`make ci` (lint, verify-generate, verify-css, verify-no-popups, cover ≥ 75 %,
build) muss grün sein.

**Hinweis zur Abdeckung:** `COVER_PKG := ./internal/...` misst `cmd/...` nicht.
Für diesen Slice ist das folgenlos — alle Änderungen liegen unter `internal/` —
aber es bleibt der blinde Fleck, den [[specs/2026-07-28-patch-doc-shrink-guard-design]]
schon notiert hat.

---

## 12. Bewusst nicht Teil dieses Entwurfs

- **Scope-Vokabular** (`doc.read`, `doc.write`, …) — ein Vokabular für genau
  einen Konsumenten.
- **Tabelle `service_accounts`** mit CLI-Verben, Rotation, Widerruf. Der Weg
  dorthin ist offen; wenn eine zweite Maschine mit anderem Bedarf kommt, ist das
  der Moment.
- **Token-Introspection / Revocation-Check** — Authentiks `exp` genügt.
- **Rate-Limits pro Maschine.**
- **Cross-Tenant-Sharing** — durch die Delegation gerade nicht nötig.
- **Maschinen-Auth für flow-mcp oder die CLI** — beide sind interaktiv, der
  Runner spricht REST.

---

## 13. Akzeptanzkriterien

- [ ] Der `curl` aus §9.1 liefert einen dekodierbaren JWT mit erwartetem `iss`
      und `aud`; der Wert von `sub` steht als **Maschinen-Sub** in
      `FLOW_MACHINE_ACCOUNTS`, Soennes eigener Sub (aus `FLOW_ALLOWED_SUBS`)
      als **Besitzer-Sub**.
- [ ] `client_credentials`-Token → `POST /api/v1/documents` → **201**, Dokument
      unter `notes/runs/<id>` in Soennes Tenant sichtbar.
- [ ] Das Dokument trägt `updated_by_kind = agent` / `updated_by_ref =
      wartung-agent`; der Aktivitätsfeed zeigt die Maschine, nicht Soenne.
- [ ] Dasselbe Token gegen `DELETE /api/v1/documents/{id}` → **403** mit
      erklärendem Körper.
- [ ] Dasselbe Token gegen `POST /api/v1/nodes` → **403**.
- [ ] Token mit unbekanntem Sub → **403** mit erklärendem Körper.
- [ ] Abgelaufenes/manipuliertes Token → **401**.
- [ ] Browser-Login und `flow login` (device_code) funktionieren unverändert.
- [ ] Ohne gesetzte Maschinen-Variablen startet der Server und verhält sich wie
      vorher.
- [ ] Der Runner tauscht sein Token in Phase 7 und schreibt den Langbericht;
      der Kurzbericht in Chat meldet `Flow doc write: OK`.
- [ ] Doku-Abschnitt „Headless-Clients authentifizieren": Grant, Provider,
      benötigte Variablen, Beispiel-curl, die vier Fehlerbilder.
- [ ] `make ci` grün.

---

## 14. Risiken

| Risiko | Umgang |
|---|---|
| Authentik liefert ein opakes statt eines JWT-Access-Tokens | §9.1 vor jeder Code-Änderung; fällt die Annahme, ändert sich §4 grundlegend und der Plan wird neu geschnitten |
| `aud` des Maschinen-Tokens ist nicht `flow-machine` | ebenfalls §9.1; ggf. wird die Audience-Erwartung an den tatsächlichen Wert angepasst |
| Delegation macht das SA-Passwort zu einem Schlüssel für Soennes Dokumente | Rechte auf Dokument-Lesen/Schreiben begrenzt (§5.1); kein Löschen, keine Nodes, keine Worktime, keine Settings |
| Ein Dokument-Overwrite per `PUT` durch den Runner ist nicht rückholbar | Bekannt und akzeptiert; das Sicherheitsnetz ist [[feature-requests/document-revisions-und-papierkorb]] — offen, Spec fertig. Der Runner schreibt heute ausschließlich `POST` auf einen pro Lauf eindeutigen Pfad |
| Startfehler durch Teilkonfiguration in PROD | Bewusst gewählt: ein Server, der Maschinen-Token still ablehnt, kostet mehr Diagnosezeit als ein lauter Start-Abbruch |
