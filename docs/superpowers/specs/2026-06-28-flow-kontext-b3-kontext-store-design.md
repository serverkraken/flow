---
type: agent
project: github.com/serverkraken/flow
---
# flow Kontext-Redesign · Baustein 3 — Kontext-Store (Kern) — Detail-Spec

**Datum:** 2026-06-28 · **Branch:** Slice-Branch von `rebuild` (z.B. `b3-kontext-store`), am Ende integriert (unmerged) — wie B1/B2 · **Status:** Entwurf bestätigt 2026-06-28 (Schnitt + Ranking + Output-Kontrakt + Save-Trigger + Hook-Mechanik verifiziert); bereit für Implementation-Plan
**Übersichts-Spec:** `docs/superpowers/specs/2026-06-27-flow-kontext-redesign-design.md` (Achsen, Mechanik, D1–D11)
**Vorgänger B1 (Hierarchie):** `docs/superpowers/specs/2026-06-27-flow-kontext-b1-hierarchie-bindings-design.md` (`nodes`, `Ancestors`, Resolution — gelandet)
**Vorgänger B2 (Tags):** `docs/superpowers/specs/2026-06-28-flow-kontext-b2-tag-system-design.md` (`taggings`, `FilterIDs(AND|OR)`, `TagScope` — gelandet)

## Ziel

Die **Load/Save-Schleife** des Kontext-Stores bauen: ein **Compose-Endpoint**, der für ein Repo die
**Ahnenkette** (B1) ∪ **Tag-Matches** (B2) in *einem* Round-Trip, nach Typ gruppiert und **token-budgetiert**
zusammenstellt; ein **path-Upsert** für `activeContext`; und die **zwei Hooks**, die das in Claude Code
verdrahten — `SessionStart` **lädt**, `Stop` **erinnert** ans Flushen. Dazu `flow context` CLI + Offline-Cache.
Das macht aus den vorhandenen Lese-Armen + B1/B2-Primitiven zum ersten Mal einen **automatischen, geräte­
übergreifenden Start-Kontext** — ohne Disziplin-Abhängigkeit beim Laden, mit konditionalem Backstop beim Speichern.

B3-Kern konsumiert **genau** die zwei Primitive, die B1/B2 liefern (`NodeStore.Ancestors`, `TagStore.FilterIDs`)
und fügt **eine** Schema-Änderung hinzu (`documents.pinned`).

## Scope

**In:**
- **Compose-Read:** `GET /api/v1/context?repo=X` + MCP `flow_get_context` — Ahnenkette ∪ global-Tag-Match, nach Typ gruppiert, gerankt, budgetiert; kapselt `global≠none` intern.
- **activeContext-Write:** path-Upsert (`ON CONFLICT (owner_id, coalesce(node_id,''), path)`) + `PUT /context/active` + MCP `flow_set_active_context`. Konvention: `type=memory`, path `active-context`, **repo-scoped**.
- **`documents.pinned BOOLEAN`** (Migration 0021) — schlanker expliziter Flag, Top-Sortkey im Relevanz-Tier.
- **Hooks:** `SessionStart`-Load + `Stop`-Flush-Reminder (konditional, debounced); `flow context install-hooks` (idempotent, settings.json).
- **CLI:** `flow context` (rendert), `install-hooks`, `flush-check` (Hook-intern); **Offline-Cache** (`~/.flow/context-cache`) mit stale-Marker.
- **Einschritt-Write:** native Auto-Memory aus + Konvention „Memory nur nach flow"; generelle Memories laufen weiter über `flow_create_doc/update_doc`.

**Out (bewusst — je eigener Baustein):**
- **Branch-Mechanik** (2-dim Resolution origin→Repo + `git branch`→Branch-Node, `gitremote.CurrentBranch`, auto-create, branch-scoped activeContext, Nicht-Code-`vorhaben`-Scope) → **B3c**. B3-Kern ist repo-scoped = Default-Branch-Fall, **forward-compatible** (B1-8: `node_id` zeigt später auf den Branch-Node statt Repo).
- **DocType-Redesign** (`agent`→`spec`/`plan`, `activeContext` als eigener Type) + **Werte-Umschreiben aller doc-`path`** → **B3d**. B3-Kern operationalisiert D5 über **bestehende** Types (`instruction`/`memory`).
- **Ist-Migration** (~40 lokale Memories + globale CLAUDE.md + `CLAUDE-*.md` → flow, klassifiziert) → **B3d**.
- **Lifecycle** (Provenance, Verfall, `veraltet`-Status, Verdichtung) → **Querschnitt A**. `pinned` ist der einzige vorgezogene, vorwärtskompatible Anker.
- **Kontext-Inspektor**, Hierarchie-Baum-Sichten, Typ/Scope/Tag-Badges, Aufräum-Ansicht, globale Cross-Scope-Suche → **Querschnitt B**. Das Compose-**JSON** ist die Datenquelle, die der Inspektor später rendert.
- **Optimistic Concurrency** (If-Match auf activeContext) → deferred (Einzelnutzer, last-write-wins).

## Entscheidungen (B3-spezifisch, erweitern D1–D11 der Übersicht)

- **B3-1** Schnitt = **voller Kern** (Compose+Write+Hooks) in *einer* Spec; Branch (B3c) / DocType+Migration (B3d) / Lifecycle (Quer A) / UI (Quer B) je eigener Baustein. *(entschieden 2026-06-28)*
- **B3-2** Compose-Output = **strukturiertes JSON**; die `flow context` CLI **rendert** Markdown; MCP gibt JSON an Claude; der Hook ist ein CLI-Zweizeiler. (Gegen Markdown-vom-Endpoint: *eine* Render-Stelle; die Struktur speist Inspektor/Offline-Cache/MCP wieder.)
- **B3-3** Query-freies Ranking im Relevanz-Tier = **`pinned` → Tag-Match (global, D7) → `updated_at` desc**, harter Budget-Cap, **„dropped"-Footer** je Gruppe. (Gegen recency-only: Kern-Memories überleben; gegen explizite Prio-Stufen: keine laufende Pflege-Last.)
- **B3-4** **`pinned BOOLEAN DEFAULT false`** als schlanker **expliziter** Flag auf `documents` — **kein Tag** (B2-7: Tags sind neutral, keine Logik im Tag). Einzige Schema-Änderung; vorwärtskompatibel zu Prio/Lifecycle (Quer A).
- **B3-5** Bootstrap-Scope (D5) operationalisiert über **bestehende** Types `instruction` + `memory` (+ activeContext = `memory`@`active-context`); `agent`/`daily`/`free`/`skill`/`project` bleiben **on-demand** (nicht im Bootstrap). Kein DocType-Redesign in B3-Kern.
- **B3-6** activeContext **repo-scoped** (Default-Branch, B1-8-forward-compat); **path-Upsert** auf `(owner_id, coalesce(node_id,''), path)` (nutzt B1s Unique-Index direkt); **adressiert per path**, nicht per doc-id (der Flush kennt keine id).
- **B3-7** Save-Trigger = **`Stop`-Hook konditional** (`hookSpecificOutput.additionalContext`, verifiziert: „conversation continues so Claude can act"), **nicht `SessionEnd`** (cleanup-only, erreicht Claude nicht). Claude erzeugt den Text, der Hook sichert die Verlässlichkeit. Debounce über Frische-Prüfung + `stop_hook_active`.
- **B3-8** Concurrency = **last-write-wins** (Einzelnutzer, sequenziell über Geräte); `updated_at` wird mitgegeben; If-Match deferred.
- **B3-9** **Offline-Cache** (`~/.flow/context-cache/<key>.json`): bei flow-unerreichbar stale-mit-Marker servieren — **`SessionStart` bricht nie hart ab**. Der **HARD-RULES-Seed** (D6, lokales `~/.claude/CLAUDE.md`) bleibt **handgepflegt**; der Installer rührt CLAUDE.md **nie** an, nur `settings.json`.
- **B3-10** Die `global≠none`-Falle lebt **vollständig im Compose-Usecase** (global = `node_id IS NULL`); Caller/CLI/Hook/MCP sehen sie nie.

---

## 1 · Compose — reiner Kern + Usecase-Orchestrierung

Das **Ranking/Budgeting ist eine reine Funktion** (deterministisch, ohne DB, voll table-testbar). Der Usecase
macht nur die I/O (resolve, ancestors, Store-Queries, `FilterIDs`) und ruft dann den reinen Composer.

```
Tier IMMER (ungekappt)          Tier RELEVANZ (gekappt, gerankt)
─────────────────────           ────────────────────────────────
instruction:  Ahnenkette ∪ NULL  memory@engagement
activeContext: memory@repo        memory@global  ── nur wenn tag-relevant (D7)
memory:       node ∈ {repo,                         Rang: pinned → updated_at desc
                       vorhaben}   ── Budget-Cap → Rest fällt raus, Footer zählt
```

**Domain/Usecase-Typen** (`internal/usecase/compose_context.go`):

```go
type ContextItem struct {
    ID        string
    NodeID    *string         // nil = global
    ScopeLabel string         // "repo:flow" / "engagement:Privat" / "global"
    Type      domain.DocumentType
    Tags      []string
    UpdatedAt time.Time
    Pinned    bool
    EstTokens int             // Heuristik (siehe §3)
    Body      string          // nur für die Render-/MCP-Ausgabe
}

type DroppedCount struct{ Engagement, Global int }

type ComposedContext struct {
    Resolution struct {
        Repo       *domain.Node   // nil wenn unauflösbar
        Chain      []domain.Node  // Blatt→Wurzel (leer wenn unauflösbar)
        Unresolved bool
    }
    Instructions  []ContextItem            // immer
    ActiveContext *ContextItem             // immer (nil wenn noch keiner existiert)
    Memories      map[string][]ContextItem // "repo"|"vorhaben"|"engagement"|"global"
    Budget        struct{ Used, Cap int; Dropped DroppedCount }
}

// Compose ist REIN: ordnet, budgetiert, zählt Weggelassenes. Kein DB-Zugriff.
// alwaysItems sind gesetzt; relevance-Kandidaten kommen bereits tag-gegatet (global) rein.
func Compose(always alwaysTier, relevance relevanceTier, cap int) ComposedContext
```

**Usecase `ComposeContext.Execute(ctx, owner, in ResolveInput, cap int)`** (`ResolveInput{RemoteSlug, MachineID, Cwd, NodeOverride string}` — dieselben Eingaben wie B1 `ResolveNode`, vom CLI/MCP **client-seitig** gefüllt, da nur der Client git-origin + machine-id seines Repos kennt):
1. **Resolve** via B1 `ResolveNode(owner, RemoteSlug, MachineID, Cwd)` (oder `NodeOverride`-Slug) → Repo-Node. Unauflösbar → `Unresolved=true`, nur global-Tier (kein Fehler).
2. **Ancestors** `NodeStore.Ancestors(owner, repoID)` → Kette `[repo,(vorhaben),engagement]`.
3. **Gather** (Store-Queries, §4): instructions(Kette ∪ NULL) · activeContext(memory@repo@`active-context`) · memory(node ∈ {repo,vorhaben}) · memory@engagement · global-memory tag-gegatet.
4. **Tag-Gate global (D7):** `AktiveTags` = ∪ Tags der Kettenknoten (B2 node-`TagsForMany`); `FilterIDs('document', AktiveTags, TagOr)` ∩ {global memories}. Leere AktiveTags ⇒ kein global-memory-Cross.
5. **Compose** (rein) → `ComposedContext`.

> **Hinweis Seed-Dedup:** global-`instruction` aus flow + lokaler HARD-RULES-Seed (D6) können sich inhaltlich überlappen. In B3-Kern existieren noch *keine* global-instructions in flow (Migration = B3d) → Tier faktisch leer, harmlos. Die saubere Trennung „Seed = nur HARD RULES lokal / flow = Rest global" entscheidet B3d beim Aufteilen der heutigen globalen CLAUDE.md.

## 2 · Datenmodell — eine Spalte, kein neuer Index

**Migration `0021`** (goose-annotiert, [[feedback_pgstore_goose_migrations]]):
```sql
-- +goose Up
ALTER TABLE documents ADD COLUMN pinned BOOLEAN NOT NULL DEFAULT false;
-- +goose Down
ALTER TABLE documents DROP COLUMN pinned;
```

- **path-Upsert** braucht **keinen** neuen Index — er trifft den in B1 angelegten Unique-Index
  `documents_owner_node_path` auf `(owner_id, coalesce(node_id,''), path)`:
  ```sql
  INSERT INTO documents (id, owner_id, node_id, type, path, title, body, pinned, created_at, updated_at)
  VALUES ($1,$2,$3,'memory',$4,$5,$6,$7,now(),now())
  ON CONFLICT (owner_id, coalesce(node_id,''), path)
  DO UPDATE SET body = EXCLUDED.body, title = EXCLUDED.title, updated_at = now()
  RETURNING id, updated_at;
  ```
- `pinned` ist nur im Relevanz-Tier (engagement/global memories) sortrelevant; auf anderen Docs harmlos.

## 3 · Token-Schätzung (Heuristik, kalibrierbar)

Keine Tokenizer-Abhängigkeit: `EstTokens = ceil(len(body)/4)` (grobe DE/EN-Ratio). Genau genug, um Budget zu
fahren und den Footer zu füllen; die **exakte Cap-Zahl** wird beim ersten Dogfood am Footer kalibriert (D8, §13).
`FLOW_CONTEXT_BUDGET` (env, Default z.B. 6000 „Tokens") überschreibbar; `--cap` am CLI für Experimente.

## 4 · Ports + Stores

**Wiederverwendet (B1/B2, keine Änderung):** `NodeStore.Ancestors`, `TagStore.FilterIDs`, `TagStore.TagsForMany`.

**Neu/erweitert (`internal/ports/ports.go`, `pgstore`):**
- `DocumentStore.ListForNodes(ctx, owner string, nodeIDs []string, types []domain.DocumentType) ([]domain.Document, error)` — Kette-Docs (NULL via Sentinel im Slice). Ein Query, hydriert Tags via B2.
- `DocumentStore.GetByPath(ctx, owner string, nodeID *string, path string) (domain.Document, bool, error)` — activeContext gezielt holen (bzw. aus dem `ListForNodes`-Ergebnis filtern, falls günstiger).
- `DocumentStore.UpsertByPath(ctx, owner string, nodeID *string, typ domain.DocumentType, path, title, body string, pinned bool) (id string, updatedAt time.Time, error)` — §2.
- `DocumentStore.SetPinned(ctx, owner, id string, pinned bool) error` — Pin setzen (REST/CLI).

## 5 · API

`internal/adapter/httpserver/context.go` (+ Wiring `server.go`):
- **`GET /api/v1/context?remote=<origin-slug>&machine=<id>&path=<cwd>&node=<slug-override>&cap=<n>`** (Resolution-Triple wie B1, alle optional, **client-seitig** gefüllt) → `200` mit Compose-JSON (§1). **Unauflösbar = `200`** (nicht 404) mit `resolution.unresolved=true` + global-Tier — der Hook soll *immer* etwas Sinnvolles bekommen.
- **`PUT /api/v1/context/active`** body `{repo|node, body, title?, tags?}` → path-Upsert → `200 {id, updatedAt}`.
- **`POST /api/v1/documents/{id}/pin`** body `{pinned}` → `SetPinned` → `200`.
- `apiclient` (`internal/adapter/apiclient/context.go`): `ComposeContext`, `SetActiveContext`, `SetPinned`.

## 6 · MCP (`cmd/flow-mcp/`)

- **`flow_get_context`** `{repo?, cap?}` (`repo` = Node-Slug-Override; default = client-seitige Resolution origin/machine/cwd des MCP-Servers) → das Compose-JSON (Claude reasont drüber).
- **`flow_set_active_context`** `{repo?, body, tags?}` → path-Upsert; der Flush-Pfad, den der Stop-Reminder anstößt.
- Bestehende Doc-Tools (`flow_create_doc/update_doc`) bleiben der generelle Memory-Schreibpfad (B3-5).

## 7 · CLI (`cmd/flow/context.go`)

```
flow context [--path <dir>] [--repo <slug>] [--cap <n>] [--json]   # --path (Default cwd) treibt die client-seitige origin/machine-Resolution; --repo = expliziter Node-Slug-Override; rendert Markdown/JSON; nutzt Offline-Cache
flow context install-hooks [--print]                              # idempotent in ~/.claude/settings.json; --print = dry-run
flow context flush-check [--path <dir>]                           # Hook-intern: exit/stdout steuert den Stop-Reminder
```

- **Render** (Markdown): Gruppen als Abschnitte (`## Instructions`, `## Active Context`, `## Memories — Repo/…`),
  pro Item eine knappe Scope/Updated-Zeile, **Footer**: `used/cap Tokens · +N engagement, +M global nicht gezeigt`
  (und im Offline-Fall `⚠ offline — Stand <ts>`). Glyphen monospace, keine Emoji ([[feedback_no_icons]]).
- **Offline-Cache:** jeder erfolgreiche `flow context` schreibt das JSON nach `~/.flow/context-cache/<owner+repo-key>.json`;
  bei Netzfehler wird der Cache mit stale-Marker gerendert. Kein TTL (stale-aber-da schlägt leer).

## 8 · Hooks (verifiziert gegen die offizielle Doku)

**SessionStart** (lädt; `additionalContext`, nicht blockierend, vor erstem Prompt):
```jsonc
// ~/.claude/settings.json (vom Installer geschrieben)
{ "hooks": { "SessionStart": [ { "hooks": [
  { "type": "command", "command": "flow context --path \"$CLAUDE_PROJECT_DIR\"" }
] } ] } }
```
stdout (Markdown) wird als Kontext injiziert. Bricht nie hart ab (Offline-Cache, §7).

**Stop** (erinnert konditional; `additionalContext` → „conversation continues so Claude can act"):
```jsonc
{ "hooks": { "Stop": [ { "hooks": [
  { "type": "command", "command": "flow context flush-check --path \"$CLAUDE_PROJECT_DIR\"" }
] } ] } }
```
`flush-check`-Logik:
1. `stop_hook_active == true` (aus Hook-Input) ⇒ **sofort still** (Loop-Schutz).
2. **echte Arbeit?** Heuristik aus dem `transcript_path`: kam in dieser Session ≥1 mutierender Tool-Use vor
   (Edit/Write/`git commit`/flow-Write)? Nein ⇒ still.
3. **stale?** flow `active-context.updated_at` (für den Repo-Node) **<** Session-Start (erster Transcript-Timestamp)?
   Nein (frisch, Claude hat schon geflusht) ⇒ still.
4. sonst ⇒ stdout/JSON `additionalContext`: „Du hast in dieser Session gearbeitet, aber `active-context` (wo war ich /
   was offen / nächster Schritt) nicht aktualisiert — flush jetzt via `flow_set_active_context`, bevor du stoppst."
   → Claude flusht → nächster Stop sieht frisch (Schritt 3) ⇒ still. Genau ein Extra-Turn.

**Installer** (`install-hooks`): merged die beiden Blöcke **idempotent** in `~/.claude/settings.json`
(vorhandene fremde Hooks erhalten; eigene per Marker-Kommentar erkennbar/ersetzbar). **Rührt CLAUDE.md nie an** (D6/B3-9).

## 9 · Einschritt-Write + Seed

- **native Auto-Memory aus** (Konfig) + Konvention im geladenen Kontext: „Memory nur nach flow schreiben"
  (über `flow_create_doc/update_doc` typed; activeContext über `flow_set_active_context`). Kein vergessener Zweit-Spiegel.
- **Seed (D6):** lokales `~/.claude/CLAUDE.md` = **nur HARD RULES**, handgepflegt. B3-Kern ändert es **nicht** und committet es **nie**
  (globale CLAUDE.md-Regel). Die *Aufteilung* heutiger CLAUDE.md-Inhalte in Seed vs. flow = **B3d**.

## 10 · Datei-Änderungs-Karte (für den Plan)

- **usecase:** neu `compose_context.go` (reiner `Compose` + Orchestrierung), `set_active_context.go`, `set_pinned.go`.
- **ports:** `ports.go` — `DocumentStore.{ListForNodes,GetByPath,UpsertByPath,SetPinned}`.
- **pgstore:** `documents.go` (Upsert, ListForNodes, SetPinned, `pinned` in `docCols`/scan), Migration `0021`.
- **adapter/httpserver:** neu `context.go` (`GET /context`, `PUT /context/active`, `POST /documents/{id}/pin`); `server.go`-Wiring.
- **adapter/apiclient:** neu `context.go` (`ComposeContext`/`SetActiveContext`/`SetPinned`).
- **cmd/flow:** neu `context.go` (`flow context [--json|--cap|--repo]`, `install-hooks`, `flush-check`); Offline-Cache-Helfer.
- **cmd/flow-mcp:** `flow_get_context` + `flow_set_active_context` (Tools + Schemas + Wiring).
- **domain:** ggf. `document.go` — `Pinned bool`-Feld.
- **(kein Hook-Code im Repo außer den vom Installer geschriebenen Snippets — die leben in `~/.claude/settings.json`.)**

## 11 · Testing-Strategie

TDD durchgängig ([[feedback_plan_main_wiring_task]] — finaler Wiring-Task mit curl-Smoke jeder neuen Route;
[[feedback_subagent_git_commits_isolated]] — HEAD nach jedem Subagent prüfen):
- **Domain/Usecase (rein, der Schwerpunkt):** `Compose` als Table-Test — Tier-Zuordnung, pinned-vor-recency, Tag-Gate global, Budget-Cap + exakte `Dropped`-Zählung, leere-AktiveTags, unauflösbares Repo (nur global), activeContext-nil.
- **pgstore (Docker):** `UpsertByPath` (insert→update idempotent, trifft den B1-Index, `coalesce(node_id,'')` für global), `ListForNodes` (Kette ∪ NULL, Typ-Filter, Tag-Hydration), `SetPinned`, Migration `0021` (idempotent, Default false).
- **usecase:** `ComposeContext.Execute` via Fakes (resolve→ancestors→gather→Compose; D7-Gate über Fake-`FilterIDs`).
- **REST:** httptest `GET /context` (auflösbar/unauflösbar=200, cap greift), `PUT /context/active` (Upsert), `POST /pin`.
- **CLI:** Render-Snapshot (Footer, Offline-Marker), `install-hooks` idempotent (settings.json merge erhält Fremd-Hooks), `flush-check` Entscheidungstabelle (stop_hook_active / keine-Arbeit / frisch / stale).
- **apiclient/mcp:** je Layer (Fake-Server/Tool-Schema).
- **Done-Gate:** `make ci` grün (Coverage-Gate halten) + **Live-Smoke vs Postgres+Dex** (Compose über echte Hierarchie+Tags, path-Upsert, Pin wirkt aufs Ranking) **+ echter Hook-Dogfood in Claude Code** (SessionStart lädt sichtbaren Block; Stop erinnert *nur* bei stale+Arbeit; Offline-Fall bricht den Start nicht).

## 12 · Abhängigkeiten / Reihenfolge / Slicing

B3-Kern setzt **B1** (Ahnenkette) + **B2** (Tag-Match) voraus — beide gelandet. Empfohlener Schnitt (der Plan macht Tasks daraus):

```
(a) Compose-Kern + Store + Migration   reiner Compose + ListForNodes/UpsertByPath/pinned 0021   → pgstore-Docker-Gate
(b) API + MCP + CLI                     GET /context, PUT /context/active, pin; flow_get_context/ → curl-Smoke
                                        flow_set_active_context; flow context (+Offline-Cache)
(c) Hooks + Installer                   SessionStart/Stop-Snippets, install-hooks, flush-check    → Hook-Dogfood
(d) Einschritt-Write + Wiring + Gate    Auto-Memory aus + Konvention; Composition-Root; curl jede  → make ci + Live
                                        Route; Live-Dogfood
```

**Branch:** Slice-Branch von `rebuild` (z.B. `b3-kontext-store`), am Ende integriert (unmerged) — wie B1/B2. Worktree-Wahl beim Plan-Start (`git worktree list`; aktuell gehört die Arbeit in `flow-rebuild`).

## 13 · Offene Kalibrierung (bewusst beim Dogfood, nicht jetzt)

- **Budget-Cap-Zahl** (D8): Default grob, exakt am Footer des ersten Live-Dogfoods justieren — nicht raten.
- **Flush-„echte-Arbeit"-Signal** (§8.2): Start-Heuristik = ≥1 mutierender Tool-Use im Transcript; falls zu laut/zu still,
  am Dogfood schärfen (z.B. Mindest-Turn-Zahl, oder nur bei Datei-/Commit-/flow-Mutationen).
- **estTokens-Faktor** (§3): `/4` ist ein Startwert; am realen Korpus gegen die tatsächliche Block-Größe abgleichen.
