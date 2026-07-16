# Globale CLAUDE.md-Neufassung — flow always in the loop (Design Spec)

- **Datum:** 2026-07-16
- **Status:** Approved (Brainstorm mit Soenne, alle Richtungsfragen entschieden)
- **Scope:** `~/.claude/CLAUDE.md` (global, pro Maschine), Hook-Installation, self-hosted
  Kontrakt-Doc in flow, kuratierte Alt-Memory-Migration. **Keine** Code-Änderungen an
  flow-server / flow / flow-mcp.

---

## 1. Kontext & Problem

flow ist funktional fertig für die Rolle „erste Quelle der Wahrheit für Kontext":

- **flow-mcp** exponiert 26 Tools (Docs-CRUD, Hybrid-Suche, `flow_get_context` mit
  Profilen handoff/standard/full, `flow_set_active_context`, Kuratierung via
  `flow_context_inventory`/`flow_curate_context`/`flow_reorder_context`, Artefakte,
  `flow_bind_project`, `flow_update_node`) plus MCP-Resources pro Projekt-Doc.
- **Typsystem:** `daily, project, free, memory, instruction, skill, plan, spec,
  activecontext` — `agent` ist **deprecated**.
- **Kontext-Compose:** Tiers (leaf/vorhaben/engagement/global), Pins, Kontext-Modus
  auto/immer/nie, Priority-Ranking, Budget (Server-Default 12k Tokens), Dedup.
- **Loop-Mechanik existiert als Code:** `flow context` rendert das
  SessionStart-Markdown (Instructions → Active Context → Always → Memories →
  Budgetzeile) mit Offline-Cache-Fallback (Hook kann nie hart failen);
  `flow context install-hooks` installiert idempotent SessionStart
  (`flow context --path "$CLAUDE_PROJECT_DIR"`) + Stop (`flow context flush-check`:
  Flush-Reminder, wenn mutierende Arbeit passiert ist und der Active Context stale ist).

**Problem:** Nichts davon ist im Alltag verdrahtet.

1. Die Hooks sind in `~/.claude/settings.json` **nicht installiert** — flow ist nur im
   Loop, wenn Claude von sich aus dran denkt.
2. Die globale CLAUDE.md ist **stale**: sie schreibt verbindlich `type: "agent"` vor
   (deprecated), kennt weder `install-hooks` noch activecontext/Kontext-Modi/Kuratierung.
3. Es laufen **drei konkurrierende Gedächtnissysteme**: flow, Claude Codes natives
   Auto-Memory (`MEMORY.md`-Verzeichnis, ~50 Einträge), Memory-Bank (`CLAUDE-*.md`).
   Doppelpflege, Drift, Widersprüche.

## 2. Entschiedene Richtungsfragen (Soenne, 2026-07-16)

| # | Frage | Entscheidung |
|---|-------|--------------|
| 1 | Nicht-flow-Gedächtnissysteme | **flow-exklusiv** — natives Auto-Memory stilllegen, Memory-Bank-Sektion streichen; Memories/Instructions/ActiveContext nur noch in flow |
| 2 | Hook-Rollout | **Einmalig `flow context install-hooks` pro Maschine**; CLAUDE.md dokumentiert das (kein dotfiles-Management) |
| 3 | Alt-Bestände | **Ja, kuratiert migrieren** (Triage statt Blindkopie), danach MEMORY.md-Stub |
| 4 | CLAUDE.md-Architektur | **A: Bootstrap-Kontrakt + self-hosted Regeln** — schlanker Kern in der Datei, Detailregeln als globales instruction-Doc in flow (Modus „immer") |

## 3. Zielbild: der Loop

Drei deterministische Punkte plus eine Disziplin-Regel:

1. **Session-Start (deterministisch):** SessionStart-Hook injiziert den komponierten
   flow-Kontext. Der `# flow context`-Block ist die maßgebliche Wahrheit über
   Projektstand, Memories und Regeln.
2. **Session-Ende (deterministisch):** Stop-Hook erinnert an
   `flow_set_active_context`, wenn Arbeit passiert ist und der Active Context stale ist.
3. **Während der Session (Disziplin, via CLAUDE.md + Kontrakt-Doc):**
   - Schreibdisziplin: dauerhafte Erkenntnisse/Stände/Specs/Pläne → flow-MCP-Tools mit
     korrektem Typ; niemals natives Auto-Memory; vor create erst suchen
     (Update schlägt Duplikat).
   - Recall: `flow_search_docs` vor Brainstorming/Planung/Resume.

## 4. Neue globale CLAUDE.md (Bootstrap-Kontrakt)

Ziel: deutlich kürzer, drift-arm, funktioniert als Minimal-Fallback auch ohne
erreichbaren Server.

**Bleibt (inhaltlich unverändert, ggf. gestrafft):**
- Hard Rules: Banned CLI-Tools (fd/rg-Tabelle) + Verweis auf enforce-fast-tools-Hook,
  Banned Behaviors, Required Behaviors (inkl. worktree-Regel).
- Arbeitsweise („How Soenne and Claude Work Together"): Schreibkonvention,
  Workflow-Stufen trivial/non-trivial.
- Subagent-Routing: `code-searcher`, `ux-design-expert` — **ohne**
  `memory-bank-synchronizer` (entfällt mit der Memory-Bank).
- CLI Quick Reference, claude-docs-consultant-Verweis.

**Fliegt:**
- Sektion „Memory Bank System" (komplett).
- Alte Sektion „Flow as cross-device knowledge store" (deprecated `type: "agent"`,
  `docs/superpowers/**`-Spiegelregeln, `flow docs import`-Warnung).

**Kommt neu — Sektion „flow = erste Quelle der Wahrheit für Kontext":**
- Der `# flow context`-Block im Session-Kontext ist die erste Quelle der Wahrheit —
  er ersetzt MEMORY.md und Memory-Bank. Widerspricht lokaler Augenschein dem Block,
  gilt: Code/Repo für Fakten, flow für Stand/Absicht — Diskrepanz ansprechen.
- **Bootstrap-Fallback:** Fehlt der `# flow context`-Block in einer Session:
  `flow context install-hooks` ausführen (einmalig pro Maschine, idempotent) und für
  die laufende Session `flow_get_context` (MCP) bzw. `flow context` (CLI) rufen.
- **Schreibdisziplin-Kern:** neue dauerhafte Erkenntnis → `flow_create_doc` /
  `flow_update_doc` mit korrektem Typ (`spec`, `plan`, `memory`, `instruction`,
  `skill`, …); niemals natives Auto-Memory; erst `flow_search_docs`, dann schreiben.
- **Flush-Pflicht:** bei Handoff/Arbeitsende `flow_set_active_context`
  (wo war ich / was offen / nächster Schritt). Der Stop-Hook erinnert nur — die Regel
  gilt auch ohne Reminder.
- **Unbound repo:** Kontext meldet „repo not bound" → `flow_bind_project` vorschlagen.
- **Verweis:** „Detailregeln (Typen-Matrix, Kuratierung, Artefakte) liefert das
  instruction-Doc `claude-code-flow-kontrakt` im flow-Kontext."

## 5. Self-hosted Kontrakt-Doc in flow

- **Doc:** `instruction`-Doc, global (`project: none`), Pfad
  `instructions/claude-code-flow-kontrakt`, Kontext-Modus **immer** → kommt per Hook
  in jede Session jeder Maschine.
- **Inhalt:**
  - Doc-Typen-Matrix: was wohin — Specs → `spec`, Pläne → `plan`, Repo-`AGENTS.md` →
    `instruction` (repo-scoped), Arbeitsweise-/Fakten-Memories → `memory` mit
    passendem Scope-Tier (leaf/vorhaben/engagement/global), Skills → `skill`,
    Sonstiges → `free`/`project`.
  - Active-Context-Flush-Format (wo war ich / was offen / nächster Schritt + Stand/Branch).
  - Kuratier-Regeln: Pins sparsam, Priority/Modus via `flow_curate_context`,
    Budget-Wächter via `flow_context_inventory` (keine „pinned not shown"-Warnung dulden).
  - Artefakt-Nutzung (`flow_upload_artifact` mit `path`, free vs. node-gebunden).
  - Suchheuristiken (Hybrid-Suche, `project: "global"` für Cross-Projekt-Recall).
- **Effekt:** Regeländerungen = ein `flow_update_doc`, sofort cross-device wirksam.
  Die CLAUDE.md muss dafür nie wieder angefasst werden.

## 6. Stilllegung der Alt-Systeme

- **Natives Auto-Memory:** abschalten. Den korrekten Mechanismus (Setting vs.
  Verhaltensregel) klärt der Implementation-Plan via `claude-docs-consultant`;
  falls kein hartes Setting existiert, übernimmt die CLAUDE.md-Regel
  („niemals natives Auto-Memory") die Funktion. `MEMORY.md` wird nach der Migration
  zum Stub: „Memory lebt in flow — nichts hier ergänzen."
- **Memory-Bank:** Sektion + `memory-bank-synchronizer`-Routing aus CLAUDE.md
  entfernen. Vorhandene `CLAUDE-*.md` per Scan finden
  (`fd -H "CLAUDE-.*\.md" ~/SourceCode ~/.claude`), bei der Migration auswerten,
  Verwertbares nach flow, Dateien danach löschen (flow-Repo-Spiegel existiert
  größtenteils schon).

## 7. Kuratierte Migration der Alt-Memories (~50 Einträge)

Triage statt Blindkopie:

| Klasse | Ziel | Modus |
|--------|------|-------|
| `feedback_*` (Arbeitsweise-Regeln) | globale flow-`memory`-Docs | wenige harte (z. B. Multi-Tenant-Grundsatz, keine Monolithen) **immer**; Rest **auto** |
| `reference_*` | `memory`, global oder repo-scoped (flow, homelab, dotfiles) | auto |
| `project_*` | größtenteils DONE-Historie, liegt als Spec/Plan schon in flow → **nicht migrieren**; nur offene Backlog-Items (z. B. M4 Slice 4+5, L5-Runbook-Rest, tmux-Status-Gate) in Active Context bzw. Backlog-Doc überführen | — |

Nach der Migration: `flow_context_inventory` prüfen — die Globals dürfen das
12k-Budget nicht dominieren; sonst Modi/Prioritäten nachkuratieren.

## 8. Verifikation (Definition of Done)

Frische Claude-Code-Session nach dem Umbau:

1. `# flow context`-Block erscheint automatisch, enthält Kontrakt-Doc + Always-Memories.
2. Test-Write landet typisiert in flow (`flow_create_doc` mit Typ ≠ agent, korrektes Projekt).
3. Stop nach mutierender Arbeit ohne Flush → Reminder erscheint; nach
   `flow_set_active_context` → kein Reminder.
4. Budgetzeile ohne „pinned not shown"-Warnung; Kontrakt-Doc im „immer"-Standing
   (`flow_context_inventory`).
5. Kein Write mehr ins native Memory-Verzeichnis; MEMORY.md ist Stub.

## 9. Non-Goals

- Keine Änderungen an flow-server / flow / flow-mcp (alles Nötige existiert; offene
  Review-Findings wie F26 `install-hooks`-Härtung laufen im Review-Strang, nicht hier).
- Kein dotfiles-Management der Hooks oder der CLAUDE.md.
- Keine Migration der DONE-Projekthistorie.
- Projektlokale CLAUDE.md-Dateien (z. B. im flow-Repo) bleiben unangetastet.
- Kein Umbau der Superpowers-Skills/Workflows.

## 10. Risiken & Gegenmittel

- **Server down + leerer Offline-Cache** → Detailregeln fehlen in der Session.
  Gegenmittel: der Bootstrap-Kern in der CLAUDE.md deckt Schreib-/Flush-Disziplin ab;
  `flow context` degradiert kontrolliert („offline — …" statt Hard-Fail).
- **Budget-Verdrängung durch globale Immer-Docs** → Kontrakt-Doc schlank halten
  (Ziel < 1k Tokens), Inventory-Check als DoD-Punkt 4.
- **Subagenten ohne SessionStart-Hook-Kontext** → Dispatch-Regel bleibt: Subagenten
  bekommen vollen Kontext im Prompt mitgegeben (steht schon in der Arbeitsweise-Sektion).
