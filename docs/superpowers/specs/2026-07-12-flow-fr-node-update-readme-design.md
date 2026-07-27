# flow — FR-Slice: Node-Update-Tooling + README im Projekt-View — Design Spec

**Status:** DRAFT (brainstormed, user-approved design 2026-07-12). Branch-Ziel: eigener
Feature-Branch off `rebuild`. Quelle: zwei Feature-Requests (Jukebox-Session 2026-07-12):
`feature-requests/agent-tooling-gaps` und `feature-requests/readme-in-project-view`.

## Ziel

Ein kombinierter Slice, der zwei zusammenhängende Reibungspunkte aus der Jukebox-Session löst:

- **FR-B (Agent-Tooling):** Node-Metadaten non-interaktiv pflegbar machen — über CLI (`flow node update`) und MCP (`flow_update_node`) — und dabei die `UpdateNode`-PATCH-Semantik von „Full-Replace“ auf **echte Partial-Semantik** umstellen. Das schließt einen **Live-Bug** (jeder `pause`/`resume`/`archive` nullt heute das Node-Icon) und den Upstream-Unbind-Footgun.
- **FR-A (README-Anzeige):** Besitzt ein Knoten ein Kompendium-Dokument mit Pfad `readme`, rendert die WebUI-Projektansicht (Cockpit) dieses Dokument automatisch — inkl. Inline-Artefakte `![[slug]]` — wie GitHub die README.md.

Beide FRs sind in einem Spec und einem Plan gebündelt (Entscheidung Soenne). Die Umsetzung
ist in Slices geschnitten, sodass die riskante Backend-Änderung (Partial-PATCH) zuerst landet
und alle Oberflächen darauf aufsetzen.

## Kern-Entscheidungen (bestätigt)

1. **Ein kombiniertes Spec/Plan** für beide FRs.
2. **Echte Partial-PATCH-Semantik** über Pointer-Felder (nicht: Full-Replace + Fetch-Merge-Write pro Client).
3. **`flow node update` = Alleskönner:** Metadaten **+** `--status` **+** `--rate/--currency/--clear-rate` **+** `--slug`.
4. **MCP `flow_update_node`** bekommt Parität zur CLI (generische Features in jeden Host).
5. **SVG-Hinweis: out of scope** — komplett weggelassen.
6. **README-Sektion ganz oben** im Cockpit (direkt nach der Anleitung), mit **dezentem Empty-State + Anlege-Link**, wenn kein `readme`-Doc existiert.

---

## Slice 1 — Partial-PATCH-Kern (Backend, das Rückgrat)

Alle weiteren Slices hängen hieran. `UpdateNode` wird von „voll überschreiben“ auf „nur gesetzte
Felder anwenden“ umgestellt.

### Datenmodell / Usecase

`internal/usecase/update_node.go` — `UpdateNodeInput` auf Pointer:

```go
type UpdateNodeInput struct {
	Name        *string
	Slug        *string
	Color       *string
	Glyph       *string
	Icon        *string
	Description *string
	UpstreamGit *string
	Status      *domain.NodeStatus
	CountsTowardTarget *bool // bleibt wie heute
}
```

`Execute` wendet jedes Feld **nur an, wenn non-nil** an (nil = unangetastet; non-nil `""` =
bewusst leeren). Das existierende nil-Guard-Muster von `CountsTowardTarget` wird zum Muster für alle:

```go
if in.Name != nil        { p.Name = *in.Name }
if in.Slug != nil        { p.Slug = *in.Slug }
if in.Color != nil       { p.Color = *in.Color }
if in.Glyph != nil       { p.Glyph = *in.Glyph }
if in.Icon != nil        { p.Icon = *in.Icon }
if in.Description != nil  { p.Description = *in.Description }
if in.Status != nil      { p.Status = *in.Status }
if in.CountsTowardTarget != nil { p.CountsTowardTarget = in.CountsTowardTarget }
```

**`syncRemoteBinding` nur feuern, wenn `in.UpstreamGit != nil`.** Der Upstream wird nur dann
angefasst (und ein Binding nur dann evtl. gelöscht), wenn der Caller Upstream ausdrücklich
mitschickt. Ein Partial-Update ohne `UpstreamGit` kann kein Binding mehr löschen — der Footgun
ist an der Quelle geschlossen, für **jeden** Caller.

### Transport (HTTP)

`internal/adapter/httpserver/worktime.go` — `updateProjReq` auf Pointer-Felder (`*string` /
`*NodeStatus`), damit **abwesende JSON-Keys nil bleiben** (Go: fehlender Key → nil Pointer). Der
Handler `handleUpdateNode` reicht die Pointer 1:1 in `UpdateNodeInput` durch.

### apiclient

`internal/adapter/apiclient/client.go` — `UpdateNodeFields` auf Pointer **und fehlendes `Icon`
ergänzen** (Live-Bug-Fix). JSON-Tags mit `omitempty`, damit nur gesetzte Felder im Body landen:

```go
type UpdateNodeFields struct {
	Name        *string `json:"name,omitempty"`
	Slug        *string `json:"slug,omitempty"`
	Color       *string `json:"color,omitempty"`
	Glyph       *string `json:"glyph,omitempty"`
	Icon        *string `json:"icon,omitempty"`
	Description *string `json:"description,omitempty"`
	UpstreamGit *string `json:"upstreamGit,omitempty"`
	Status      *string `json:"status,omitempty"`
}
```

### Bestehende Caller anpassen

- **`cmd/flow/node_status.go`** (`pause`/`resume`/`archive`): kein Fetch-Full/Rebuild mehr —
  nur noch `UpdateNodeFields{Status: &s}` senden. Das behebt den Icon-Null-Bug automatisch
  (Icon nil → unangetastet).
- **WebUI Node-Formular** (`handleUpdateNode`-Caller in der WebUI, Node-Edit): bewusst **weiter
  Full-Replace** — das Formular sendet alle Felder (alle non-nil), Verhalten unverändert. Das
  Formular *soll* ein voller Replace sein; Agenten senden eine Teilmenge. Ein Endpoint bedient
  beide Intents korrekt, kein Split nötig. **Verifizieren**, dass das Formular alle Felder (inkl.
  Icon) sendet — sonst würde das Edit-Formular jetzt Felder auf nil lassen statt zu leeren.

### Tests

- `update_node`: nur-Name-Update lässt Slug/Description/Upstream/Icon/Status unverändert;
  Upstream nil → `syncRemoteBinding` feuert nicht, Binding bleibt; Upstream non-nil geändert →
  Binding-Sync wie bisher; Icon nil → Icon bleibt; Icon non-nil `""` → Icon geleert.
- `worktime`-Handler: JSON ohne `icon`-Key → `Icon == nil`; JSON `{"description":"x"}` → nur
  Description-Pointer gesetzt, Rest nil.
- Regression: `pause`/`resume`/`archive` (über apiclient-Fake) nullen das Icon **nicht** mehr.

---

## Slice 2 — CLI: `flow node update` + `flow node show`

### `flow node update <slug>` (`cmd/flow/node_subcommands.go` + `cmd/flow/node.go`)

```
flow node update <slug> [--name …] [--desc …] [--color …] [--glyph …] [--icon …]
                        [--upstream …] [--status active|paused|archived]
                        [--slug …]
                        [--rate <amount-minor>] [--currency EUR] [--clear-rate]
```

**Nur tatsächlich gesetzte Flags werden gesendet** — via `cmd.Flags().Changed("<flag>")`; jedes
gesetzte Flag → non-nil Pointer in `UpdateNodeFields`. Kein Pre-Fetch, kein Read-Merge (der Kern
macht das obsolet).

**Metadaten + Status + Slug** gehen in den `UpdateNode`-PATCH (ein Aufruf). `--status` mappt
String → `domain.NodeStatus`; ungültiger Wert → klarer Usage-Fehler.

**Rate ist ein separater Endpoint** (`POST /api/v1/nodes/{id}/rate` via
`apiclient.SetNodeRate(ctx, id, *int64, currency)`). Das Kommando **orchestriert zwei Aufrufe**:
- `--rate <amount-minor>` (+ optional `--currency`, Default `EUR`) → `SetNodeRate(id, &amount, cur)`.
- `--clear-rate` → `SetNodeRate(id, nil, "")`.
- `--rate` und `--clear-rate` zusammen → Usage-Fehler.
Rate bleibt backend-seitig eine eigene Operation; nur die CLI bündelt sie in `update`.

**Node-Adressierung:** `<slug>` → via `GetNode`/Resolve zu Node-ID (Muster wie andere
Node-Verben). `--slug` ändert den Slug des adressierten Knotens (Identitäts-Änderung; bewusst
mit aufgenommen).

### `flow node show` (`runNodeShow`)

Eine `Beschreibung:`-Zeile ergänzen, wenn der Knoten eine Description hat. Schließt die
„ich kann nicht prüfen, was gesetzt ist“-Lücke.

### Tests

- `Changed`-Logik: nur gesetzte Flags landen als non-nil im Fake-Client-Call.
- `--rate` → `SetNodeRate` mit amount; `--clear-rate` → `SetNodeRate(nil)`; beide → Fehler.
- `--status` Mapping inkl. Fehlerfall.
- `node show` druckt Description, wenn vorhanden; nichts, wenn leer.

---

## Slice 3 — MCP: `flow_update_node`

`cmd/flow-mcp/` — neues Tool, registriert wie `flow_bind_project`
(`mcp.AddTool(s, &mcp.Tool{...}, h.updateNode)`), Params-Struct mit `omitempty` (Muster
`bindNodeIn`). **Parität zur CLI:**

```
flow_update_node {
  repo?, slug?, id?,              // Adressierung
  name?, description?, color?, glyph?, icon?, upstream?,
  status?,                        // active|paused|archived
  rate?, currency?, clearRate?    // Rate wie in der CLI (zweiter Call)
}
```

**Adressierung** mirror der übrigen Node-Tools: explizites `id`/`slug` gewinnt; sonst den an das
aktuelle Repo gebundenen Knoten auflösen (gleiche Resolution wie
`flow_bind_project`/`flow_project_context`).

`omitempty`-Felder → non-nil Pointer nur wenn gesetzt → dieselbe Partial-Semantik end-to-end.
Kein Fetch-Merge im Handler nötig (der Kern trägt das). Rate: bei `rate`/`clearRate` gesetzt den
`SetNodeRate`-Call wie die CLI absetzen.

> **Slug via MCP** ist mit aufgenommen (Parität), aber ein Identitäts-Footgun für Agenten — im
> Tool-Description klar als „selten nötig, ändert die Adressierung“ kennzeichnen.

### Tests

- Handler-Test (Fake-apiclient): nur gesetzte Felder werden durchgereicht; Adressierung
  id > slug > repo-binding; `clearRate` → SetNodeRate(nil).

---

## Slice 4 — FR-A: README-Sektion im Cockpit

### Discovery (nur eigener Knoten, keine Vererbung)

In `nodeCockpitData` (`internal/adapter/httpserver/webui_cockpit.go`): dieselbe
`s.ListDocuments.Execute(ctx, u.ID, &n.ID, nil)` wie die Wissen-Sektion; client-seitig
case-insensitiv nach einem Doc filtern, dessen Pfad `readme` bzw. `readme.md` ist
(`strings.EqualFold(strings.TrimSuffix(path, ".md"), "readme")`). Ein README erbt **nicht** von
Ancestors/Subtree — nur das eigene Doc des Knotens.

### Render

Artefakt-Resolver mit dem bestehenden `buildArtifactResolver(chain, arts)` bauen (chain =
Ancestors+Self via `s.NodeAncestors.Execute`, Artefakte via `s.ListArtifacts.Execute` — exakt wie
`buildDocumentVM`), Wikilink-Resolver analog. Dann `webui.RenderDocument(ctx, doc.Body,
wikilinkResolver, artifactResolver)`. Ergebnis: Inline-`![[slug]]`-Artefakte (Node → Ancestors →
Frei-Bibliothek aufgelöst) und `[[wikilinks]]`, bluemonday-sanitisiert — identisch zur
`/wissen/{id}`-Leseseite. **Voll rendern, keine Kürzung** (Projekt-Startseite).

**Silent degrade:** jeder Fehler (ListDocuments/Ancestors/ListArtifacts/Render) → keine
README-Sektion bzw. Empty-State, **nie 500** (Konvention des Builders).

### View

- Neues Feld auf `webui.NodeCockpit` (`internal/adapter/webui/cockpit_vm.go`): z.B.
  `Readme template.HTML` + `HasReadme bool` (oder kleine Struct mit Empty-State-Flag +
  Anlege-Href).
- Neue Sektion `cockpitReadmeSection(d)` in `internal/adapter/webui/cockpit_main.templ`, in
  `CockpitMain` **ganz oben, direkt nach `cockpitInstr`** (vor `cockpitEnthaeltSection`)
  eingehängt.
- **Empty-State** (kein `readme`-Doc): dezente Zeile „Kein README — Dokument mit Pfad `readme`
  anlegen“ mit Direktlink zum Editor, `?node={id}` vorbefüllt (bestehender `handleWebEditorNew`
  unterstützt `?node=`). Falls Pfad-Prefill (`?path=readme`) noch nicht existiert: entweder klein
  ergänzen oder Hinweistext „Pfad `readme` setzen“ — im Plan entscheiden.

### SSE

Die README-Sektion an denselben Cockpit-Reload-Container hängen, der schon auf `document.*`
lauscht (Wissen-Sektion), damit ein README-Edit live durchschlägt. Falls die Sektion einen
eigenen Container braucht: `sse:document.updated`/`document.created` triggern.

### Tests

- `nodeCockpitData`: Knoten mit `readme`-Doc → `HasReadme`, gerendertes HTML enthält den
  Doc-Inhalt; Knoten ohne → Empty-State-Flag; case-insensitive Match (`README`, `readme.md`).
- Inline-Artefakt: `![[slug]]` im README → aufgelöster `<img>`/Link im HTML (Fake-Resolver).
- Fehlerpfad: ListDocuments-Fehler → kein 500, Empty-State/keine Sektion.
- templ-Golden/Render-Test der Sektion (Empty-State + gefüllt).

---

## Slice 5 — Main-Wiring-Verifikation + Gate

- **Wiring prüfen** ([[feedback_plan_main_wiring_task]]): neue Routen/Tools/Kommandos verdrahtet
  (`cmd/flow-server/main.go` unverändert nötig? — neue Usecases? nein, alle bestehen; MCP-Tool in
  `newServerH`; CLI-Kommando in `nodeCmd()`).
- **curl-Smoke:** `PATCH /api/v1/nodes/{id}` mit `{"description":"x"}` → nur Description geändert,
  Icon/Upstream/Binding intakt; voller Body → Full-Replace unverändert.
- **CLI-Smoke:** `flow node update <slug> --desc …`; `--rate`; `--clear-rate`; `flow node show`
  zeigt Description; `pause`/`resume` nullen Icon nicht mehr.
- **MCP-Smoke:** `flow_update_node` gegen einen Testknoten.
- **Browser-Dogfood:** Jukebox-Projekt-View zeigt README samt der drei PNG-Artefakte gerendert;
  Projekt ohne README zeigt den Empty-State.
- `make ci` GRÜN (Lint, verify-generate, verify-css, cover 75% `*_templ.go` excl., Build).
  `make generate` nach templ-Änderung committen; **kein `make fmt`** in Dispatches.

---

## Akzeptanzkriterien (aus den FRs)

- Ein Agent kann eine Node-Beschreibung (und Farbe/Glyph/…) **non-interaktiv** setzen — CLI **und**
  MCP.
- `PATCH` mit Teil-Body lässt nicht genannte Felder unberührt; ein Update ohne Upstream löscht kein
  Binding; `pause`/`resume`/`archive` nullen das Icon nicht mehr.
- `flow node show` zeigt die Beschreibung.
- Cockpit des Jukebox-Projekts zeigt das README samt der drei PNG-Artefakte gerendert; Projekte
  ohne `readme`-Doc zeigen den dezenten Empty-State.

## Bewusst out of scope

- **SVG→PNG-Hinweis** in der Artefakt-Fehlermeldung (aus FR-B; Soenne: weglassen).
- **TUI-README-Anzeige** (FR-A: nice-to-have, nicht Teil dieses FRs).
- Kein neues Feld/keine Migration für README — reine Anzeige-Konvention (Pfad `readme`).

## Prozess / Done-Gate

TDD ([[feedback_no_monoliths]]: ein File pro Verantwortung, keine Monolithen). Slices
subagent-getrieben, je eigener Plan-Abschnitt; Opus-Review je Slice; Live-Dogfood im Browser für
Slice 4. Verbot von `make fmt` in Dispatches. Multi-Tenant-Disziplin: alle Store-Queries
owner-scoped ([[feedback_flow_is_multi_tenant]]).
