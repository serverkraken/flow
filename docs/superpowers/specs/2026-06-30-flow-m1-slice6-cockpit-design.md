# flow M1 — Slice 6: Cockpit (Node-Detail mit Tabs) — Design Spec

> Datum: 2026-06-30 · Status: **DRAFT** (brainstormed, user-approved design) · Branch-Ziel: `rebuild`.
> Übergeordnete Spec: [[specs/2026-06-29-flow-m1-projekt-zentrik-webui-design]] §13.6 (Slice 6).
> Vorgänger: Slices 1–5 DONE + in `rebuild` gemerged (zuletzt Activity-Feed `6bcce3a`).

---

## 1. Kontext & Ziel

Slice 6 macht aus der bestehenden read-only Node-Detailseite `/nodes/{id}` das **Cockpit**: ein
projekt-/knoten-zentrisches Arbeitszentrum mit **persistentem Kontext-Kopf** (inkl. Per-Node-Timer
und Subtree-Rollup) und vier **Tabs** (Worktime · Wissen · Struktur · Bindings). Es ist überwiegend
**WebUI-Arbeit** auf bereits vorhandenem Backend; der einzige nennenswerte Backend-Zuwachs sind
dünne WebUI-Handler für den Per-Node-Timer.

**Erfolgskriterien**
- Von jedem Knoten aus: Timer starten/stoppen/wechseln, Rollup sehen, eigene Sessions + Docs +
  Kinder + Bindings erreichen — ohne die Seite zu verlassen.
- Rollup-Zahlen sind **korrekt** (Subtree, geerbte Rate) — nicht mehr die alte Eigensumme.
- Sieht im Kristall-Stil **schick** aus und ist **responsive** (Desktop ↔ Phone).

---

## 2. Nicht-Ziele (Slice 6)

- **Kein** per-Node Saldo/Burndown (existiert nur owner-weit; bleibt so). „Stats" ist kein eigener
  Tab — die Zahlen leben als Rollup im Kopf.
- **Kein** Voll-Editor für Worktime im Cockpit (Edit/Delete bleibt auf `/zeit`; hier nur Liste +
  Nachbuchen).
- **Kein** neues Datenmodell, keine Migration. Keine Sharing/Tenancy-Features (das ist M2/M3).
- **Kein** Kontext-/AI-Sicht-Tab (das ist Slice 8).

---

## 3. Ist-Zustand (Forensik, Anker für den Plan)

- **Seite:** `GET /nodes/{id}` → `handleWebNodeView` (`internal/adapter/httpserver/webui_nodes.go:435`),
  Daten via `nodeCockpitData` (`webui_nodes.go:409`), Template `webui.NodeView` /
  `nodeCockpitBody` (`internal/adapter/webui/nodes.templ:284`/`:311`). Single-Scroll, 7 Sektionen
  (Header, Beschreibung, Git, Worktime-Aggregat, Docs, Bindings read-only, Move-Form).
- **Nav-Tree** (`GET /ui/nav/tree` → `handleNavTreeFragment`) und Node-Liste verlinken bereits auf
  `/nodes/{id}`. Es gibt **kein** `/projects/{id}` mehr.
- **Backend bereits vorhanden** (kein Neubau nötig):
  - `StatsComputer.NodeStats(ctx, ownerID, nodeID) (domain.NodeRollup{Total,Week,Month}, error)` —
    Subtree-Rollup; REST `GET /api/v1/nodes/{id}/stats`; apiclient `NodeStats()`.
  - `domain.ResolveRate(chain []Node) *Money` (`internal/domain/node.go:129`) — geerbte Rate
    leaf→root; `NodeAncestors.Execute` liefert die Kette.
  - `StartSession.Execute(ctx, ownerID, nodeID *string, tags, note)` — nimmt nodeID **beim Start**
    (Guard `IsBookable`); `StopSession.Execute(ctx, ownerID, sessionID, nodeID *string)`.
  - `ListDocuments.Execute(ctx, ownerID, nodeID *string, tags)` — node-scoped Docs.
  - `ListNodes` / `NodeAncestors` / Subtree-Kinder (über `ListNodes` + `MoveTargetsFor`).
  - Bindings: `BindNode`/`UnbindNode` Usecases; REST `PUT /api/v1/nodes/{id}/bindings`,
    `DELETE /api/v1/nodes/bindings`, `GET /api/v1/nodes/{id}/bindings`; apiclient
    `BindRemote/UnbindRemote/BindPath/UnbindPath/ListBindings`.
  - `AddSession` (Nachbuchen), `UpdateNode` (Status), Activity (`ListActivity`) vorhanden.
- **Komponente:** `components.TabStrip(tabs []Tab, active string)` (`components/subnav.templ:10`,
  `Tab{Key,Href,LabelKey}`) existiert (Full-Page-`<a>`-Links) — wird für die htmx-Variante leicht
  erweitert oder pro Cockpit neu instanziiert.
- **Veraltet (wird ersetzt):** `nodeWorktime` (`webui_nodes.go:375`) summiert nur **eigene**
  Sessions (kein Subtree) und nutzt `n.Rate` **direkt** (nicht `ResolveRate`). Entfällt.
- **i18n:** `node.section.*`, `node.worktime.*`, `node.move*`, `node.status.*` vorhanden.
  **Keine** `cockpit.*` Tab-Keys — neu anzulegen (`catalog_de.go`/`catalog_en.go`).

---

## 4. Informationsarchitektur

**Persistenter Kopf + 4 Tabs.** Kein eigener „Übersicht"-Tab — die Übersicht IST der immer
sichtbare Kopf. Reihenfolge: **Worktime · Wissen · Struktur · Bindings**. Worktime ist Default.

```
┌─ engagement › vorhaben › [Knoten] ───────────────────────────┐
│ ⬡  Knotenname   [Kind]  ● status   95 €/h (geerbt)   [Timer] │   ← persistenter
│ ┌ Σ Gesamt ┐ ┌ Woche ┐ ┌ Monat ┐ ┌ Erlös ┐                   │      Glas-Kopf
├─ Worktime · Wissen · Struktur · Bindings ────────────────────┤   ← Tabstrip
│ (aktiver Panel — htmx-Swap)                                   │
└──────────────────────────────────────────────────────────────┘
```

---

## 5. Persistenter Kopf

Glas-Karte (Kristall-Tokens, `web/tailwind.css`), farbige Akzent-Kante in der Knotenfarbe,
Hexagon-Glyph, Breadcrumb (Ancestors root→leaf), Kind-Badge, Status, **geerbte Rate**.

**Rollup-Kacheln:** Σ Gesamt (inkl. Unterknoten) · Woche · Monat · Erlös — gespeist aus
`NodeStats` + `ResolveRate` (§7).

**Timer — drei Zustände** (Quelle: aktuell laufende Session des Owners):
1. **nichts läuft** → `[▶ Start]` → `POST /nodes/{id}/start` (StartSession mit nodeID).
2. **läuft auf diesem Knoten** → Live-Uhr (`[data-timer]`-Rebind wie Home) + `[■ Stopp]` →
   `POST /nodes/{id}/stop`.
3. **läuft auf anderem Knoten Y** → „läuft auf Y →" (Link zu `/nodes/Y`) + `[Wechseln]` mit
   **Inline-Confirm** (kein Popup) → `POST /nodes/{id}/switch` (stoppt Y, startet hier).

Nur **buchbare** Knoten (`IsBookable`: engagement/vorhaben/repo) zeigen den Timer; ein **branch**
zeigt stattdessen einen dezenten Hinweis „nicht buchbar".

---

## 6. Tabs

### 6.1 Worktime
Read-only Liste der **eigenen** Sessions dieses Knotens (Datum, Zeitspanne, Tag/Note, Dauer),
neueste zuerst, begrenzt (z. B. letzte 25, „mehr" optional später) + `[+ Nachbuchen]`. Nachbuchen
öffnet ein Formular **vorgebucht auf den Knoten** und ruft den vorhandenen `AddSession`-Pfad.
Edit/Delete bewusst **nicht** hier (bleibt `/zeit`). Kopf zeigt Subtree-Rollup, Tab zeigt
Eigenbuchungen — diese Trennung wird im UI beschriftet.

### 6.2 Wissen
Node-scoped Docs via `ListDocuments(ownerID, &nodeID, nil)`, Liste mit Titel/Kind → Link auf
`/wissen/{id}` + `[+ Neu]`, das den Editor mit **vorbelegtem Knoten** öffnet. `handleWebEditorNew`
(`webui_editor.go:35`) liest heute **keinen** Node-Param → ein optionaler `?node=`-Query-Prefill ist
hier zu ergänzen (kleine Zusatzarbeit). EmptyState wenn leer.

### 6.3 Struktur
Direkte **Kinder** des Knotens (aus `ListNodes` gefiltert auf `ParentID == id`), je mit Kind-Badge
+ Mini-Worktime, Link aufs Kind-Cockpit. Plus:
- `[+ Unterknoten]` — Formular mit **vorbelegtem Parent + erlaubtem Kind**. `handleWebNodeNew`
  (`webui_nodes.go:133`) defaultet heute auf `Kind=engagement` ohne Parent-Prefill → ein
  `?parent=`/`?kind=`-Query-Prefill ist zu ergänzen (kleine Zusatzarbeit).
- **Reparent/Move** — die bestehende Move-Form (`nodeMoveForm`, `nodes.templ:376`) wandert hierher.
- **Status**-Umschalter (aktiv/pausiert/archiviert) via bestehenden `POST /nodes/{id}/status`.

### 6.4 Bindings
Liste aktueller Bindings (remote + path) mit **Löschen**. **Hinzufügen nur für `remote`**
(Git-Slug-Eingabe → `PUT /api/v1/nodes/{id}/bindings`-Pfad). **`path`-Bindings** sind
maschinen-lokal (entstehen über CLI `flow start` Auto-Detect) → read-only anzeigen + löschen, mit
Hinweis „pro Maschine via CLI hinzufügen". (Löst Spec §15 „Umfang Bindings-Management ohne
Sharing-Backend".)

---

## 7. Datenfluss & Korrektur

Der Kopf nutzt künftig **`NodeStats`** (Subtree-Rollup) statt der in-process Eigensumme, und
**`ResolveRate`** (geerbte Ahnenketten-Rate über `NodeAncestors`) statt `n.Rate` direkt —
konsistent mit dem Export (`BuildExport`). `nodeWorktime` (`webui_nodes.go:375`) wird entfernt.

- **Kopf-Rollup** = `NodeStats(ownerID, id)` → `{Total, Week, Month}`; Erlös = `ResolveRate(chain)`
  × Total (Money), leerer Erlös wenn keine Rate in der Kette.
- **Worktime-Tab-Liste** = eigene Sessions (`ListSessionsRange` gefiltert auf `NodeID == id`, oder
  ein schmaler Read; kein neuer Usecase nötig — Filter im Handler wie heute, aber nur für die
  Liste, nicht für die Aggregat-Zahlen).

---

## 8. Tab- & SSE-Mechanik (htmx)

Tabs sind **htmx-Fragment-Swaps**, kein Full-Page-Reload: der Kopf (mit laufendem Live-Timer)
bleibt montiert, nur der Panel-Bereich tauscht.

- Tab-Klick → `GET /nodes/{id}/tab/{worktime|wissen|struktur|bindings}` → Panel-Fragment in
  `#cockpit-panel` (`hx-target`, `hx-swap=innerHTML`) + `hx-push-url` (Deep-Link via `?tab=`).
- **Getrennte SSE-Container**, damit ein Live-Reload nicht den aktiven Tab zurücksetzt:
  - **Kopf** (`#cockpit-head`) reloaded auf `sse:session.started, sse:session.stopped,
    sse:session.updated, sse:node.updated, sse:node.moved` → Timer-Zustand + Rollup aktuell.
  - **Panel** reloaded themenbezogen: Worktime→`session.*`, Wissen→`document.*`, Struktur→`node.*`,
    Bindings→(kein Live-Bedarf; reload nach eigener Mutation).
- Default-Aufruf `GET /nodes/{id}` rendert Kopf + Worktime-Panel; `?tab=` wählt den initialen Panel
  (für Deep-Links/Reload-Stabilität).

---

## 9. Backend-Fläche (neu, dünn)

Neue **WebUI-Handler/Routen** (`internal/adapter/httpserver/server.go` Web-Block + `webui_*.go`):

| Route | Handler | Tut |
|---|---|---|
| `POST /nodes/{id}/start` | `handleWebNodeStart` | StartSession mit nodeID (Guard IsBookable) |
| `POST /nodes/{id}/stop` | `handleWebNodeStop` | StopSession der laufenden Session |
| `POST /nodes/{id}/switch` | `handleWebNodeSwitch` | Stop laufende, dann Start hier (zwei Usecase-Aufrufe nacheinander; keine DB-Transaktion) |
| `GET /nodes/{id}/tab/{name}` | `handleWebNodeTab` | Panel-Fragment (worktime/wissen/struktur/bindings) |
| `POST /nodes/{id}/sessions` | `handleWebNodeAddSession` | Nachbuchen (AddSession, vorgebucht) |
| `POST /nodes/{id}/bindings` | `handleWebNodeBindRemote` | remote-Binding add (Form → BindNode) |
| `POST /nodes/{id}/bindings/delete` | `handleWebNodeUnbind` | Binding löschen (UnbindNode) |

**Wiederverwendet, kein Neubau:** `AddSession`, `BindNode`/`UnbindNode`, `UpdateNode` (Status,
bestehender `POST /nodes/{id}/status`), `NodeStats`, `ResolveRate`, `NodeAncestors`,
`ListDocuments`, `ListNodes`. Alle mutierenden Handler publizieren ihr `domain.Event` über
`s.Bus.Publish(...)` (Live-Sync), wie der Rest der WebUI.

---

## 10. Responsive (Kristall)

Ein Layout mit Tailwind-Breakpoints auf der bestehenden AppShell (nicht zwei getrennte Layouts):
- **Sidebar → Bottom-Nav** (Home · Zeit · Wissen · Mehr) + kompakte App-Bar (Zurück + Theme-Toggle)
  auf Mobile — nutzt die mobile „Mehr"-Drawer-Chrome aus Slice 3.
- **Kopf stapelt:** Identität → Timer (volle Breite) → Rollup-Kacheln **2×2**.
- **Tabstrip horizontal scrollbar** (aktiver Tab sichtbar).
- **Worktime-Liste → Karten** (zweizeilig) statt fester Grid-Spalten.
- Dark = primär (Kristall-Approved); Light ist derived — am `/ui`-Styleguide gegenchecken.

---

## 11. Fehlerbehandlung

- Unbekannte/fremde Node-ID → **404** (wie heute, `ErrNodeNotFound`).
- Start/Wechseln auf **nicht-buchbarem** Knoten (branch) → **400**; im UI gar nicht erst anbietbar.
- Start während hier bereits läuft → idempotent (kein zweiter Timer).
- Bindings-Doppelanlage → **409** sauber als **Inline-Fehler** im Tab (kein Popup).
- `to <= from` / ungültige Nachbuch-Range → 400 mit Inline-Feldfehler.

---

## 12. i18n

Neue `cockpit.*`-Keys in `catalog_de.go` + `catalog_en.go` (Parität): Tab-Labels
(`cockpit.tab.worktime|wissen|struktur|bindings`), Timer (`cockpit.timer.start|stop|switch|
runningOn|notBookable`), Rollup (`cockpit.rollup.total|week|month|earnings|inclChildren`),
Worktime (`cockpit.worktime.add|empty|ownOnly`), Struktur (`cockpit.struktur.addChild|children|
status`), Bindings (`cockpit.bindings.addRemote|pathHint|delete|empty`). Bestehende `node.*`-Keys
wo möglich wiederverwenden.

---

## 13. Testing & Done-Gate

- **TDD**: pro neuem Handler ein Test (Start/Stop/Switch/Tab/AddSession/Bind/Unbind) +
  **reine VM-Unit-Tests** für: Rollup-Zahlen (Subtree + geerbte Rate), Timer-Zustandswahl
  (nichts/hier/anderswo/nicht-buchbar), Tab-Auswahl & Deep-Link `?tab=`.
- `make ci` grün **inkl. `make web`** (app.css; CI baut es nicht) und Coverage ≥ Gate.
- **Live-Done-Gate** vs. Dev-Stack (Postgres + Dex): Timer Start/Stop/Wechseln end-to-end,
  Subtree-Rollup-Korrektheit (Eltern = Σ Kinder + eigen), Tab-Swap **ohne** Timer-Reset,
  Nachbuchen erscheint in Liste + Rollup, Bindings add/remove, Status-Umschalten, **Mobile-Reflow**
  im Browser.
- **Abschluss:** Opus-Holistic-Review + **Wiring-Verifikation** (main.go/Composition-Root +
  curl-Smoke jeder neuen Route) als letzte Task (Memory `feedback_plan_main_wiring_task`).

---

## 14. Slicing / Task-Outline (für den Plan)

Subagent-driven, fresh implementer pro Task + per-Task-Review. Grobe Reihenfolge:
1. **i18n** `cockpit.*` (de+en Parität).
2. **Kopf-Umbau** + Rollup-Korrektur (`NodeStats` + `ResolveRate`, `nodeWorktime` raus) — read-only,
   noch ohne Timer-Buttons.
3. **Per-Node-Timer** (Start/Stop/Switch-Handler + 3-Zustands-Kopf + Live-Timer-Rebind).
4. **Tab-Mechanik** (`/tab/{name}`-Fragmente, getrennte SSE-Container, `?tab=` Deep-Link).
5. **Worktime-Tab** (Liste + Nachbuchen).
6. **Wissen-Tab** (node-scoped Liste + Neu-vorbelegt).
7. **Struktur-Tab** (Kinder + Unterknoten + Move + Status).
8. **Bindings-Tab** (Liste + remote-add + delete + path-Hinweis).
9. **Responsive-Politur** (Reflow, Bottom-Nav, Karten) + Styleguide-Check.
10. **Wiring-Verifikation + Done-Gate** (curl-Smoke, make ci+web, live).

---

## 15. Aufgelöste offene Entscheidungen

- **Spec §15 „Umfang Bindings-Management ohne Sharing-Backend":** remote add/delete im UI; path nur
  anzeigen+löschen (CLI-erzeugt). (§6.4)
- **„Stats"-Tab:** entfällt — Rollup im Kopf, kein per-Node-Burndown. (§2/§4)
- **Worktime-Editierbarkeit:** Liste + Nachbuchen; Edit/Delete bleibt `/zeit`. (§6.1)
- **Timer-Cross-Node:** „Wechseln" (1 Klick + Inline-Confirm). (§5)
- **Tab-Mechanik:** htmx-Fragment-Swap mit persistentem Kopf (nicht Full-Page). (§8)
