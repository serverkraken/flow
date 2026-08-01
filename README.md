# flow

> Wissens- und Worktime-Produkt für Menschen **und** AI-Agents — server-
> autoritativ, multi-tenant, mit WebUI, TUI und MCP-Server auf denselben Daten.

`flow` hält fest, woran gearbeitet wird und was dabei gelernt wurde: Zeiten,
Projekte, Dokumente, Artefakte. Alle drei Oberflächen sprechen dieselbe HTTP-API
gegen denselben Postgres — was im Browser passiert, sieht die TUI ohne Reload,
und ein Agent über MCP sieht es auch.

```
┌──────────────┐   ┌──────────────┐   ┌──────────────┐
│   Browser    │   │  Terminal    │   │ Claude Code  │
│   (WebUI)    │   │  flow (TUI)  │   │  flow-mcp    │
└──────┬───────┘   └──────┬───────┘   └──────┬───────┘
       │ HTML+SSE         │ HTTP+Bearer      │ HTTP+Bearer
       └──────────────────┼──────────────────┘
                          │
                   ┌──────┴───────┐
                   │ flow-server  │  OIDC · SSE · owner-scoped
                   └──────┬───────┘
                          │
                   ┌──────┴───────┐
                   │  PostgreSQL  │  + pgvector, pg_trgm
                   └──────────────┘
```

---

## Drei Binaries

| Binary        | Rolle                                                                 |
| ------------- | --------------------------------------------------------------------- |
| `flow-server` | HTTP-Server: WebUI (server-rendered), REST-API, SSE, OIDC, Migrationen |
| `flow`        | CLI + TUI gegen den Server — Worktime, Docs, Nodes, Kontext, Export    |
| `flow-mcp`    | MCP-Server, der dieselbe API für AI-Agents zugänglich macht            |

`flow-mcp` ist ein reiner Client: er spricht ausschließlich die REST-API und hat
keinerlei Server-Code im Abhängigkeitsbaum. Ein Update der MCP-Tools braucht
deshalb kein Server-Deployment.

---

## Was steckt drin

**Worktime** — Sessions starten, pausieren, korrigieren; Tagesziel, Woche,
Historie. Freie Tage (Urlaub, Krank, Feiertage) als eigene Fläche. Export nach
CSV/JSON pro Projekt und Zeitraum. Läuft ein Timer, propagiert jede Änderung
über SSE live in alle offenen Oberflächen.

**Wissen** — Markdown-Dokumente mit Typen (`spec`, `plan`, `memory`,
`instruction`, `skill`, `daily`, `project`, `free`), Wikilinks und Backlinks.
Die Suche ist hybrid: Postgres-FTS für Phrasen, `pg_trgm` für Fragmente und
Tippfehler, `pgvector` für Bedeutung. Binäres hängt als Artefakt am Dokument
oder am Node.

**Nodes** — die Projekthierarchie aus `engagement` → `vorhaben` → `repo`.
Verzeichnisse und Git-Remotes werden an Nodes gebunden, sodass jede Oberfläche
weiß, in welchem Projekt sie gerade steht. Worktime rollt über den Teilbaum auf.

**Kontext** — ein kuratierter, budgetierter Auszug (Instruktionen, Memories,
aktiver Stand), den AI-Agents beim Session-Start ziehen, statt sich durch das
Repo zu raten.

**Cockpit** — die Einstiegsfläche, die Zeit, laufende Arbeit und Aktivität
zusammenzieht.

Menschen und AI-Agents sind dabei gleichberechtigte Akteure: `actor kind`
(human/agent) zieht sich durch Aktivität, Avatare und MCP.

---

## Architektur

Hexagonal, mit einer Verantwortung pro Datei:

```
cmd/
  flow-server/       Composition-Root des Servers (main.go + server.go)
  flow/              cobra-CLI + TUI
  flow-mcp/          MCP-Server
internal/
  domain/            reine Logik, keine I/O
  ports/             Interfaces
  usecase/           Anwendungsfälle — je ein Execute(...)
  adapter/
    httpserver/      HTTP-Handler, REST + WebUI-Routen
    webui/           templ-Komponenten, Seiten, statische Assets
    pgstore/         Postgres + goose-Migrationen
    apiclient/       HTTP-Client, den CLI/TUI/MCP teilen
    oidcauth/  oidcdevice/  oidcverify/  websession/  tokenstore/
    sse/  embed/  editor/  opener/  systemclock/  uuidgen/
  tui/               bubbletea-Screens und UI-Primitives
  i18n/              catalog_de.go + catalog_en.go (Parität test-erzwungen)
```

Jeder Datenzugriff ist **owner-scoped** — `ownerID` gehört in jede
Store-Query. Cross-Tenant-Leaks gelten als Critical-Finding, und
Performance- wie Sicherheitsargumente müssen pro Tenant halten.

---

## Voraussetzungen

| Komponente          | Pflicht  | Wofür                                              |
| ------------------- | -------- | -------------------------------------------------- |
| **Go 1.25+**        | ja       | Build aller drei Binaries                          |
| **PostgreSQL 16+**  | ja       | mit `pgvector`; `pg_trgm` kommt aus contrib        |
| **OIDC-Provider**   | ja       | Authentik, Dex, … — Login für WebUI und CLI        |
| **podman**          | dev      | lokaler Stack (Postgres + Dex) via `make dev-up`   |
| **tailwindcss-CLI** | dev      | nur für `make web`; nicht Teil von `make ci`       |
| **Ollama**          | optional | Embeddings für die semantische Suche               |

---

## Lokal entwickeln

Der Dev-Stack ist self-contained: Postgres und Dex laufen in Containern,
`flow-server` bewusst auf dem Host — so lösen Browser und Server dieselbe
Issuer-URL auf.

```sh
make dev-up      # Postgres + Dex, wartet bis beide bereit sind
make dev-run     # flow-server auf https://localhost:8080 (migriert automatisch)
make dev-token   # Dex-id_token für Bearer-Aufrufe
make dev-down    # Teardown (ARGS=-v verwirft auch das DB-Volume)
```

Login im Browser: **msoent@dev.local / password**. Zwei Tabs nebeneinander
zeigen die SSE-Live-Sync ohne Reload. Details und Fallstricke — Selfsigned-Cert,
Allowlist gegen den Dex-`sub`, LAN-Zugriff — stehen in
[`deploy/dev/README.md`](deploy/dev/README.md).

---

## Installieren

```sh
git clone https://github.com/serverkraken/flow.git
cd flow
make install     # flow, flow-server, flow-mcp -> ~/.local/bin
```

> **Hinweis zu GitHub-Releases:** die veröffentlichten Releases (aktuell
> `v1.4.3`) stammen noch aus der Zeit **vor** dem Rebuild-Cutover und enthalten
> die alte Single-Binary-App. Bis der erste Release auf dem neuen Baum
> durchgelaufen ist, ist `make install` der richtige Weg. Danach liefern die
> Archive `flow_<os>_<arch>.tar.gz` und `flow-mcp_<os>_<arch>.tar.gz`.
> `flow-server` wird nicht als Archiv veröffentlicht, sondern als Container-Image.

---

## Konfiguration

`flow-server` liest ausschließlich Environment-Variablen:

| Variable                     | Bedeutung                                            |
| ---------------------------- | ---------------------------------------------------- |
| `FLOW_LISTEN_ADDR`           | Listen-Adresse                                       |
| `FLOW_PUBLIC_BASE_URL`       | öffentliche Basis-URL (OIDC-Redirects)               |
| `FLOW_SESSION_SECRET`        | Signaturschlüssel für Session-Cookies                |
| `FLOW_OIDC_ISSUER`           | Issuer für den Browser-Auth-Code-Flow                |
| `FLOW_OIDC_CLIENT_ID`        | Client-ID des Browser-Clients                        |
| `FLOW_OIDC_CLIENT_SECRET`    | Client-Secret des Browser-Clients                    |
| `FLOW_OIDC_CLI_ISSUER`       | Issuer für den CLI-Device-Flow                       |
| `FLOW_OIDC_CLI_CLIENT_ID`    | Client-ID des CLI-Clients                            |
| `FLOW_ALLOWED_SUBS`          | Allowlist auf OIDC-`sub`                             |
| `FLOW_ALLOWED_GROUPS`        | Allowlist auf Gruppen-Claim                          |
| `FLOW_CONTEXT_BUDGET`        | Token-Budget des Kontext-Auszugs (Default 12k)       |
| `FLOW_OLLAMA_HOST`           | Ollama-Endpunkt für Embeddings                       |
| `FLOW_EMBED_MODEL`           | Embedding-Modell                                     |
| `FLOW_EMBED_BATCH`           | Batch-Größe des Embedding-Workers                    |
| `FLOW_EMBED_INTERVAL`        | Intervall des Embedding-Workers                      |
| `FLOW_EMBED_TIMEOUT`         | Timeout pro Embedding-Aufruf                         |
| `FLOW_MIGRATE_ONLY`          | nur migrieren, dann beenden                          |
| `FLOW_DEV`                   | Dev-Modus — u.a. Session-Cookie ohne `Secure`        |
| `FLOW_OIDC_MACHINE_ISSUER`   | Issuer des `flow-machine`-Providers                  |
| `FLOW_OIDC_MACHINE_CLIENT_ID` | dessen Audience                                     |
| `FLOW_MACHINE_ACCOUNTS`      | Maschinen-Delegation, siehe unten                    |

`FLOW_DEV=1` ist auf localhost Pflicht: sonst bekommt das Session-Cookie
`Secure`, wird über http nie gesendet, und der Login läuft in eine Schleife.

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

Der Besitzer wird hier immer über seinen **OIDC-Sub** adressiert, nicht über
den Benutzernamen — nur `users.oidc_sub` ist eindeutig. `FLOW_ALLOWED_SUBS`
ist davon unabhängig und nimmt trotz des Namens sowohl den Sub als auch den
Benutzernamen an (`usecase.AllowList` prüft beides); der besitzende Eintrag
dort muss also nicht zwangsläufig derselbe String sein. Das `<label>`
erscheint im Aktivitätsfeed als Urheber und in Fehlermeldungen.

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

**Revocation hat zwei Schalter:** Einen Besitzer aus `FLOW_ALLOWED_SUBS` zu
entfernen deaktiviert keine auf ihn delegierten Maschinen-Accounts — dafür
muss der zugehörige Eintrag zusätzlich aus `FLOW_MACHINE_ACCOUNTS` entfernt
werden. Ist beim Serverstart eine Sub-Allowlist ohne Gruppen konfiguriert und
fehlt der Besitzer-Sub eines Maschinen-Accounts darin, loggt der Server beim
Start eine WARN-Zeile mit Label und Besitzer-Sub — der Start selbst wird
dadurch **nicht** verhindert, denn der fehlende Sub ist zweideutig: Er kann
einen entzogenen Besitzer bedeuten, oder schlicht, dass `FLOW_ALLOWED_SUBS`
für diesen Besitzer per Benutzername statt Sub geführt wird (siehe oben).

---

## MCP-Integration

`flow-mcp` stellt AI-Agents 31 Tools bereit — Dokumente lesen und schreiben,
suchen, Nodes und Bindings verwalten, Artefakte hochladen, Kontext kuratieren
und den aktiven Stand flushen. Für Claude Code:

```json
{
  "mcpServers": {
    "flow": {
      "command": "flow-mcp",
      "env": {
        "FLOW_SERVER_URL": "https://flow.example.org",
        "FLOW_OIDC_ISSUER": "https://id.example.org/application/o/flow-cli/"
      }
    }
  }
}
```

Authentifiziert wird per Device-Flow — einmal `flow login`, danach liegt das
Token im System-Keyring.

---

## Build · Test · Lint

| Target                 | Tut                                                        |
| ---------------------- | ---------------------------------------------------------- |
| `make build`           | alle drei Binaries nach `bin/`                             |
| `make install`         | Binaries nach `$PREFIX/bin` (Default `~/.local/bin`)       |
| `make test`            | `go test -race ./...`                                      |
| `make cover`           | Coverage gegen das 75-%-Gate (`*_templ.go` ausgenommen)    |
| `make lint`            | `golangci-lint run`                                        |
| `make generate`        | `templ generate` — die `*_templ.go` werden committet       |
| `make web`             | Tailwind-Build nach `internal/adapter/webui/static/app.css`|
| `make verify-generate` | schlägt an, wenn generierte Dateien veraltet sind          |
| `make verify-css`      | schlägt an, wenn `app.css` nicht zum Quellstand passt      |
| `make verify-no-popups`| verbietet native Browser-Popups in der WebUI               |
| `make ci`              | alles davon in der Reihenfolge, die auch das Gate ist      |

`make ci` muss grün sein, bevor etwas „fertig" ist. `make fmt` bitte **nicht**
laufen lassen — die Toolchain driftet gegenüber CI.

Eine Falle, die mehrfach zugeschlagen hat: Tailwind v4 scannt jede nicht
ignorierte Datei im Repo und liest Prosa als Klassen-Token. `docs/`, `.claude/`
und Root-Markdown sind deshalb in `web/tailwind.css` per `@source not`
ausgeschlossen. Wer eine neue Datei ins Root legt, prüft `make verify-css`.

---

## Konventionen

- **WebUI:** templ + htmx + Tailwind, server-rendered. Kein SPA, kein Node zur
  Laufzeit. Fragmente aktualisieren sich über `hx-trigger="sse:<event>"`.
- **i18n:** keine hartcodierten Strings — alles über `components.T(ctx, key)`
  gegen `catalog_de.go` / `catalog_en.go`, deren Parität ein Test erzwingt.
- **Keine Browser-Popups.** `confirm()`/`alert()` sind verboten, es gibt eine
  eigene Dialog-Komponente; `make verify-no-popups` hält das durch.
- **Keine Emoji-Piktogramme.** Nur Monospace-Glyphen (`● ◆ ⬡ ▶ ■ ▰ ▱`) und SVG —
  eine Zelle breit auf jedem Font.
- **Eine Verantwortung pro Datei.** Keine Monolithen; Tests liegen daneben.
- **Commits:** Conventional Commits, kleine Schritte, Test zuerst.

Verbindliche Details für Coding-Agents stehen in [`AGENTS.md`](AGENTS.md).

---

## Deployment

`flow-server` wird als Multi-Arch-Container-Image nach GHCR gebaut
(`ghcr.io/serverkraken/flow-server`), getaggt mit Commit-SHA und `latest`.
Dockerfile: [`deploy/podman/Dockerfile.server`](deploy/podman/Dockerfile.server).
Der Digest steht in der Job-Summary des Build-Workflows und wird downstream
gepinnt.

---

## Lizenz

[MIT](LICENSE) — siehe `LICENSE` für den vollen Text.
