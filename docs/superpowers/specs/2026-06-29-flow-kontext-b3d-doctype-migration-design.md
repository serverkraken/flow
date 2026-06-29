---
type: agent
project: github.com/serverkraken/flow
---
# flow Kontext-Redesign · Baustein 3d — DocType-Redesign + Ist-Migration + Seed-Split — Detail-Spec

**Datum:** 2026-06-29 · **Branch:** Slice-Branch von `rebuild` (z.B. `b3d-doctype-migration`), am Ende integriert (unmerged) — wie B1/B2/B3-Kern · **Status:** **DESIGN — bestätigt** (alle Forks via Brainstorm-Fragen entschieden 2026-06-29), bereit für Plan.
**Übersichts-Spec:** `docs/superpowers/specs/2026-06-27-flow-kontext-redesign-design.md` (Achsen, Mechanik, D1–D11)
**Vorgänger B1 (Hierarchie):** `docs/superpowers/specs/2026-06-27-flow-kontext-b1-hierarchie-bindings-design.md` (`nodes`, `Ancestors`, `ResolveNode` — gelandet)
**Vorgänger B2 (Tags):** `docs/superpowers/specs/2026-06-28-flow-kontext-b2-tag-system-design.md` (`taggings`, `FilterIDs`, `TagScope` — gelandet)
**Vorgänger B3-Kern:** `docs/superpowers/specs/2026-06-28-flow-kontext-b3-kontext-store-design.md` (Compose, `pinned`, `UpsertByPath`, Hooks, `flow context` — gelandet; activeContext provisorisch als `memory @ active-context`)

## Ziel

Die in B3-Kern noch **bewusst vertagten** Daten- und Struktur-Aufgaben abschließen, damit der Kontext-Store
nicht nur *funktioniert*, sondern mit dem **realen Bestand befüllt** und **sauber typisiert** ist:

1. **DocType-Redesign** — `agent` in `spec`/`plan` auftrennen, `activeContext` zum **eigenen Typ** machen,
   die heutigen `specs/…`/`plans/…`-`path`-Präfixe auf den schlanken Slug eindampfen (B1: `path` = Name im Knoten).
2. **Ist-Migration** — die **116 lokalen Memory-Files** des flow-Projekts + die **globale `~/.claude/CLAUDE.md`**
   nach flow überführen, **klassifiziert** nach Scope (global/engagement/vorhaben/repo) und Typ.
3. **Seed-Split** — die globale CLAUDE.md in den **minimalen lokalen HARD-RULES-Seed** (D6) und den Rest als
   flow-`global`-`instruction` zerlegen; native Auto-Memory **aus** (Einschritt-Write, B3-Kern §9).

Damit wird flow zum ersten Mal **tatsächlich** die Single Source of Truth für Claude-Kontext — der SessionStart-Hook
(B3-Kern) lädt ab jetzt **echten** komponierten Kontext statt eines leeren Gerüsts, und der Stop-Reminder schreibt
in einen real existierenden activeContext.

## Ausgangslage (live verifiziert 2026-06-29)

- **Knoten in flow:** Engagements `Privat` (`privat`) + `RTL Extern` (`rtl-extern`); Repos `github.com/serverkraken/flow`,
  `github.com/serverkraken/homelab-study`, mehrere `gitlab.com/dataalliance/*` (RTL-Arbeit); plus Sammel-Knoten `Import`.
- **Docs in flow:** **87** Docs vom Typ `agent`, **alle** mit `path` `specs/…` oder `plans/…` (Spec-vs-Plan sauber aus
  Präfix + Titel ableitbar). `plan` existiert bereits als Typ; **`spec` existiert nicht**; **kein eigener activeContext-Typ**.
- **Memory-Quelle:** flow-Auto-Memory-Verzeichnis `~/.claude/projects/-Users-msoent-Sourcecode-serverkraken-flow/memory/`
  mit **116 Files** (`project_*`, `feedback_*`, `reference_*`, je mit `name`/`description`/`metadata.type`-Frontmatter) + die
  `MEMORY.md`-Index-Datei. **0 Memory-Docs** liegen heute in flow.
- **Global:** `~/.claude/CLAUDE.md` (110 Zeilen, HARD RULES + Arbeits-Konventionen + Subagent-Routing + Memory-Bank + flow-Konvention).
  Die 6 globalen `~/.claude/CLAUDE-*.md` sind **leere Template-Gerüste** (14–21 Zeilen, nur Überschriften) → **nicht zu migrieren**.
- **Wikilinks:** `document_links.target_path` matcht über den **bloßen Slug**; alle Wikilinks im Korpus nutzen bereits die
  schlanke Form (`[[2026-06-23-flow-webui-overhaul-design]]`, `[[feedback_no_icons]]`) — **keiner** mit `specs/`/`plans/`-Präfix.
  Die heutigen Präfix-`path`s matchen ihre eigenen Inbound-Links **nicht** → diese Links hängen aktuell ins Leere.
- **`documents.type`** wird in Go (`domain.valid()`) validiert, **kein** DB-Enum/CHECK → neue Typwerte = reine Code-Änderung, keine Schema-Migration nötig.

## Scope

**In:**
- **DocType-Redesign (Code):** `DocSpec="spec"` + `DocActiveContext="activecontext"` zum Enum; `DocAgent` **deprecaten** (valid, ungenutzt);
  Compose-Gather / `UpsertByPath` / `set_active_context` / Hook von „`memory @ active-context`" auf **`type=activecontext`** umstellen.
- **Daten-Transform (CLI, deterministisch):** die 87 `agent`-Docs → `spec`/`plan` (per `path`-Präfix) + `path`-Präfix strippen.
- **Migrations-Tool (CLI):** `flow context migrate` (hexagonal: cobra → Usecase → apiclient → REST), idempotent, `--dry-run`,
  zwei Modi `doctypes` | `memories`; wiederverwendbar für die übrigen 20 Projekt-Memory-Verzeichnisse.
- **Klassifikation (Agent + Mensch):** Subagent liest die 116 Files → reviewbares `manifest.tsv` (`file · scope · tags · pin · keep`);
  Soenne reviewt in einem Durchgang; dann `--dry-run` → apply.
- **Seed-Split (B3d-4):** lokale CLAUDE.md auf **HARD RULES** eindampfen; Rest als flow-`global`-`instruction`; native Auto-Memory aus.
- **Done-Gate:** `make ci` grün + Live-Anwendung des Redesigns + Imports gegen die echte flow-Instanz + **echter SessionStart-Hook-Dogfood**
  (schließt zugleich das in B3-Kern vertagte Hook-Done-Gate).

**Out (bewusst — je eigener Baustein):**
- **Lifecycle** (`veraltet`-Status, Provenance, Verfall, Verdichtung) → **Querschnitt A**. B3d nutzt **nur** `pinned` (vorhandener Flag aus B3-Kern)
  + die `keep/skip`-Spalte des Manifests (Skip = nicht importieren) zur Verdichtung beim Migrieren.
- **Cross-Scope-Wikilink-Resolution** (ein `repo`-Doc, das ein `global`-Memory verlinkt, müsste die Ahnenkette ∪ global walken
  wie Compose) → **Querschnitt B** (B3d-7 ✓: vertagt — begrenzt nur, wie gut migrierte *global*-Memories aus Repo-Docs verlinken).
- **Vollständige `agent`-Entfernung** aus Enum/Tests/MCP-Beschreibungen → trivialer Cleanup, **nachdem** prod als 0-`agent` bestätigt ist.
- **Die übrigen 20 Projekt-Memory-Verzeichnisse** (~180 weitere Files: homelab-study 42, reusable-workflows 36, …) → später per Re-Run des Tools.
- **Branch-Mechanik / Auto-create** (B3c), **Assets** (B4 / Phase 2), **Kontext-Inspektor / UI-Transparenz** (Querschnitt B).

## Entscheidungen (B3d-spezifisch, erweitern D1–D11 + B3-1…B3-11)

- **B3d-1** Schnitt = **voller Bundle** (DocType-Redesign + Ist-Migration + Seed-Split) in *einer* Spec, intern 5 Slices (§1). *(entschieden 2026-06-29)*
- **B3d-2** Migrations-Quelle = **flow-Projekt + global**; das Tool ist **wiederverwendbar** für die übrigen Projekte (separate Re-Runs, nicht in B3d). *(2026-06-29)*
- **B3d-3** Klassifikation = **Agent schlägt vor, Mensch reviewt** (`manifest.tsv`); danach idempotenter Importer. (Gegen Heuristik-only: mis-scopt die Quer-Schneider; gegen voll-manuell: 116 Files Handarbeit.) *(2026-06-29)*
- **B3d-4** Importer-Form = **erstklassiges `flow`-CLI-Subkommando** (cobra + Usecase + Tests, idempotent, `--dry-run`). (Gegen `flow docs import` erweitern: biegt ein Vault-Dir-Tool zum Manifest-Tool; gegen „Claude via MCP": ~116 Tool-Calls, kein echtes Tool.) *(2026-06-29)*
- **B3d-5** Redesign-Transform = **dasselbe CLI-Tool** (Modus `doctypes`), **nicht** goose-SQL. (Bewusst: einheitliches Tool, dry-runbar, reviewbar; Implikation: der Daten-Transform ist ein **manueller Run** im Done-Gate, nicht auto-applied beim Deploy. Die Enum-Erweiterung bleibt Code.) *(2026-06-29)*
- **B3d-6** `agent` = **valid-but-deprecated** während B3d (unkonvertierte Zeilen laden weiter); volle Entfernung = Cleanup nach prod-0-`agent`. *(2026-06-29)*
- **B3d-7** Cross-Scope-Wikilink-Resolution = **vertagt** (Querschnitt B). B3d profitiert bereits vom korrektiven path-Strip (gleiche-Scope-Links resolven). *(2026-06-29)*
- **B3d-8** `instruction` vs. `memory` folgt der **Quelle**: alles aus einem `memory/`-Verzeichnis → `memory`; CLAUDE.md/AGENTS.md → `instruction`. Keine Per-File-Typ-Entscheidung nötig. *(2026-06-29)*
- **B3d-9** `MEMORY.md`-Index wird **nicht** migriert (Compose/UI ersetzen den Index); die leeren globalen `CLAUDE-*.md`-Templates ebenso wenig. *(2026-06-29)*

---

## 1 · Form — 5 interne Slices (analog B3-Kern a–d)

```
B3d-1  DocType-Redesign (Code)      Enum +spec +activecontext; Compose/UpsertByPath/set_active_context/
                                    Hook nutzen activecontext-Typ; agent deprecaten
B3d-2  Migrations-Tool (CLI)        flow context migrate — ein cobra-Cmd, Usecase(s), Tests, --dry-run,
                                    idempotent; Modi: doctypes | memories
B3d-3  Klassifizieren + anwenden    Subagent → manifest.tsv → Review → --dry-run → apply (Redesign + Import)
B3d-4  Seed-Split + 1-Schritt-Write CLAUDE.md → lokaler Seed (HARD RULES) + flow-global-instruction; Auto-Memory AUS
B3d-5  Wiring + Done-Gate           make ci; Live-Redesign+Import gegen echte flow-Instanz; echter Hook-Dogfood
```

## 2 · DocType-Redesign (B3d-1)

**Enum** (`internal/domain/document.go`):
- `+ DocSpec DocumentType = "spec"`
- `+ DocActiveContext DocumentType = "activecontext"`
- `DocAgent` bleibt in `DocumentTypes()` / `valid()` (deprecated; Kommentar „retire after prod 0-agent").
- `HumanOwned()`-Logik unverändert (spec/plan/activecontext = agent-owned).
- MCP-Tool-Beschreibungen (`flow_list_docs` type-Aufzählung) ergänzen.

**Daten-Transform** (CLI `doctypes`-Modus, deterministisch über `path`-Präfix):
- `type := if path matches '^plans/' → plan else → spec` (Titel bestätigt; alle 87 Docs sind `Design`/`Spec` vs. `Implementation Plan`/`Runbook`).
- `path := regexp_replace(path, '^(specs|plans)/', '')`.
- idempotent: bereits konvertierte (`type ∈ {spec,plan}`, kein Präfix) werden übersprungen.

**activeContext als eigener Typ** (umstellen, was B3-Kern provisorisch als `memory @ active-context` baute):
- **Compose-Gather** (`compose_context.go`): activeContext nicht mehr als `memory`-mit-`path`-Filter, sondern `type=activecontext` (repo-scoped); aus dem Memory-Tier herausnehmen.
- **`UpsertByPath` / `set_active_context`**: schreibt `type=activecontext` (path bleibt Konvention `active-context`).
- **Hook/`flush-check`**: Frische-Prüfung gegen den activecontext-Typ.
- **0 Datenzeilen** heute → reine Code-Änderung, **keine** Daten-Konversion.

**Wikilink-Gewinn (korrektiv, kein Risiko):** der path-Strip bringt die Doc-`path`s in Deckung mit der schlanken Link-Form,
die die Bodies schon nutzen → heute hängende Cross-Spec-Links (`[[2026-06-23-flow-webui-overhaul-design]]`) **resolven danach**.
`document_links` selbst bleibt unberührt (speichert den Link-Text, nicht den eigenen path).

## 3 · Migrations-Tool (B3d-2 / B3d-4-Form)

`cmd/flow/context.go` (oder neu `context_migrate.go`) — Subkommandos unter `flow context migrate`:

```
flow context migrate doctypes [--dry-run]
    # liest alle agent-Docs (apiclient list type=agent, project=global),
    # leitet spec/plan + neuen path ab, update via REST; idempotent.

flow context migrate memories --dir <memdir> --manifest <m.tsv> [--dry-run]
    # liest jede Datei laut Manifest, parst+strippt Frontmatter, resolved scope→node, upsert.
```

**Hexagonal:** dünnes cobra → Usecases `RedesignDocTypes` / `MigrateMemories` → `apiclient` → REST (`UpsertByPath`,
list/update). Keine Geschäftslogik im cobra-Layer ([[feedback_no_monoliths]]).

**Pro Memory-File (`MigrateMemories`):**
- Frontmatter parsen → `path = <Dateiname ohne .md>` (Slug), `title = description`, `tags = [metadata.type] ∪ Manifest-Tags`, `pinned = Manifest`.
- **Body = Inhalt ohne Frontmatter** (B2: Body = reiner Inhalt; Tags strukturiert, nicht im Body).
- `type = memory`. `scope`-Slug aus Manifest → `node_id` (global = NULL; sonst Node-Slug aus `flow_list_projects`).
- **`MEMORY.md` überspringen.** `keep=skip`-Zeilen überspringen.
- **Idempotent:** Upsert auf `(owner, node_id, path)` → Re-Run = Update, keine Dubletten.

**Manifest-Format** (`manifest.tsv`, vom Subagent erzeugt, vom Menschen editiert):
```
file                              scope                          tags                 pin  keep
project_flow_rebuild_m1a.md       github-com-serverkraken-flow   project              -    y
feedback_no_icons.md              global                         feedback,ux          y    y
feedback_pgstore_goose_*.md       github-com-serverkraken-flow   feedback,db          -    y
reference_soenne_worktime_*.md    privat                         reference,worktime   -    y
feedback_authentik_device_*.md    github-com-serverkraken-homelab-study  feedback,authentik  -  y
project_flow_rebuild_m0.md        github-com-serverkraken-flow   project              -    skip
```

## 4 · Klassifikation + Scope-Regeln (B3d-3)

- **Subagent** liest alle 116 Files und füllt `manifest.tsv`. **Mensch reviewt** in einem Durchgang (override jederzeit).
- **Scope-Daumenregeln** (Agent-Vorschlag, Mensch entscheidet):
  - `project_flow_rebuild_*`, flow-Tech-`feedback_*` (tailwind/charm/pgstore/search) → `repo:flow`.
  - allgemeine Arbeitsweise-`feedback_*` (no_icons, no_monoliths, plan_main_wiring, generic_features, dont_descope…) → `global`.
  - Deploy/Authentik/Homelab (`feedback_authentik_*`, `reference_homelab_study_*`, next-image…) → `repo:homelab-study`.
  - Worktime-Workflow (`reference_soenne_worktime_workflow`) → `engagement:privat`.
- **`keep/skip`:** offensichtlich tote/duplizierte „done"-Milestones → `skip` (nicht importieren). Das ist der **Verdichtungs-Moment**
  für die überlimitige MEMORY.md, **ohne** den `veraltet`-Lifecycle (Querschnitt A) vorzuziehen. Im Zweifel `keep` (Compose budgetiert ohnehin).

## 5 · Seed-Split + Einschritt-Write (B3d-4)

- **Lokale `~/.claude/CLAUDE.md` behält NUR** den `HARD RULES — NEVER VIOLATE`-Block (Banned CLI Tools, Banned/Required Behaviors) —
  der bootstrap-unabhängige Seed (D6). **Wird nie committet** (globale CLAUDE.md-Regel) → die Bearbeitung ist ein lokaler Handgriff im Done-Gate, kein Repo-Change.
- **→ flow `global` `instruction`** (ein Doc, z.B. `path = working-agreement`): „How Soenne & Claude Work Together", „Subagent Routing",
  „CLI Quick Reference" **plus neu geschriebene** Abschnitte „Memory Bank System" + „Flow as cross-device store" — angepasst an die
  neue Realität (flow *ist* das Memory; Schreiben typed/scoped via `flow_create_doc`; activeContext via `flow_set_active_context`;
  Recall via `flow_get_context` / SessionStart-Hook). Erzeugt via `flow_create_doc` (type `instruction`, project `none`).
- **Native Auto-Memory AUS** (settings.json) — flow ist das einzige Schreibziel (B3-Kern §9, Einschritt-Write). Die 6 globalen
  `CLAUDE-*.md`-Templates sind leer → **nicht migriert**.

## 6 · Datei-Änderungs-Karte (für den Plan)

- **domain:** `document.go` — `DocSpec`, `DocActiveContext`; `DocumentTypes()`/`valid()`/`HumanOwned()`; `DocAgent`-Deprecation-Kommentar. Tests anpassen (`document_types_test.go`, `documenttype_test.go`, `document_owned_test.go`).
- **usecase:** `compose_context.go` (activeContext-Gather auf `activecontext`-Typ), `set_active_context.go` (Typ schreiben); neu `migrate_memories.go` + `redesign_doctypes.go` (reine Transform/Klassifikations-Anwendung, Store-I/O via Ports).
- **ports:** ggf. List-by-type/global + Update bereits vorhanden (B2/B3); prüfen, ob `UpsertByPath` Typ-Parameter trägt (B3-Kern hatte `typ` im Signatur — wiederverwenden).
- **adapter/apiclient:** `context.go` — evtl. List(type=agent, global) + Update(path,type) Helfer für `doctypes`; `SetActiveContext` Typ.
- **adapter/httpserver:** falls für `doctypes`-Update ein Endpoint fehlt (PATCH type/path) — sonst bestehende Doc-Update-Route nutzen.
- **cmd/flow:** `context.go`/`context_migrate.go` — `flow context migrate doctypes|memories`, Frontmatter-Parser/Stripper, Manifest-Reader, Scope→Node-Resolver, `--dry-run`.
- **cmd/flow-mcp:** Tool-Beschreibungen (type-Aufzählung) um `spec`/`activecontext` ergänzen; `agent` als deprecated markieren.
- **(kein Schema-Migration für Typwerte — `documents.type` ist TEXT, Go-validiert.)**

## 7 · Testing-Strategie

TDD durchgängig ([[feedback_plan_main_wiring_task]] — finaler Wiring-Task mit Live-Verifikation jeder neuen Route/jedes Modus;
[[feedback_subagent_git_commits_isolated]] — HEAD nach jedem Subagent prüfen):
- **domain (rein):** neue Typen valid; `HumanOwned` korrekt; `agent` weiter valid (deprecated).
- **usecase (rein, Schwerpunkt):**
  - `RedesignDocTypes`: `plans/x`→`plan`+`x`, `specs/y`→`spec`+`y`, schon-konvertiert → no-op (idempotent).
  - `MigrateMemories`: Frontmatter parse+strip (Body ohne Frontmatter), `path`=Slug, `tags`=metadata.type ∪ Manifest, `pinned`, scope→node (global=NULL), `skip`/`MEMORY.md` übersprungen, Upsert idempotent.
- **cmd/flow:** Manifest-Reader (Tab-Parsing, Kommentare), `--dry-run`-Snapshot (zeigt create/update/skip ohne Schreibzugriff), Frontmatter-Stripper Unit-Test.
- **apiclient:** List/Update-Bodies asserten.
- **Done-Gate:** `make ci` grün (Coverage-Gate halten) **+ Live**:
  1. `flow context migrate doctypes --dry-run` → review → apply → verifizieren: 87 Docs jetzt `spec`/`plan`, `path` ohne Präfix, Wikilinks resolven (z.B. `[[2026-06-23-flow-webui-overhaul-design]]` zeigt Backlink).
  2. Subagent → `manifest.tsv` → Review → `migrate memories --dry-run` → apply → `flow_get_context` für flow-Repo zeigt migrierte instructions+memories mit plausiblem Budget; Stichprobe `[[feedback_no_icons]]` (gleiche-Scope-Fall) resolvt.
  3. Seed-Split: lokale CLAUDE.md auf HARD RULES; flow-global-instruction angelegt; Auto-Memory aus; **SessionStart-Hook lädt den komponierten Block** (schließt B3-Kerns vertagtes Hook-Done-Gate).

## 8 · Rollout / Reihenfolge / Risiken

- **Reihenfolge:** B3d-1 (Code) → B3d-2 (Tool) → B3d-3 (`doctypes` zuerst, dann `memories`) → B3d-4 (Seed) → B3d-5 (Gate).
  `doctypes` vor `memories`, damit das finale Typ-System steht, bevor neue Docs landen (Memories sind `memory` — vom Redesign unberührt, aber Reihenfolge hält es sauber).
- **Ziel-Instanz:** die echte flow-Instanz, auf die der MCP zeigt (prod). **`--dry-run` überall zuerst**; empfohlen ein Probelauf gegen den lokalen Dev-Stack (`make dev-up`, [[reference_flow_dev_env]]) vor prod.
- **Risiken:** (a) Scope-Fehlklassifikation → durch Mensch-Review + Idempotenz (Re-Run mit korrigiertem Manifest) abgefedert. (b) `agent`-Deprecation statt -Entfernung lässt eine harmlose Altlast im Enum (B3d-6). (c) Cross-Scope-Links zu `global`-Memories resolven erst mit Querschnitt B (B3d-7).

## 9 · Abhängigkeiten

Setzt **B1** (`nodes`, `Ancestors`, `ResolveNode`, Node-Slugs), **B2** (`taggings`, Tags als Parameter), **B3-Kern**
(`UpsertByPath`, `pinned`, Compose, Hooks, `flow context`) voraus — **alle gelandet**. B3d fügt **keine** Schema-Migration hinzu
(Typwerte sind Go-validiert), nur Code + zwei CLI-Modi + die einmalige (re-runbare) Daten-Anwendung.
