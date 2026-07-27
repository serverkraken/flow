# CLAUDE.md-Neufassung — flow always in the loop · Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** flow wird die erste Quelle der Wahrheit für Claude-Code-Kontext: schlanke Bootstrap-CLAUDE.md, self-hosted Kontrakt-Doc in flow (Modus „immer"), natives Auto-Memory aus, Alt-Memories kuratiert migriert.

**Architecture:** Keine Codeänderung an flow — reine Verdrahtung: (a) globales `instruction`-Doc in flow, das per SessionStart-Hook in jede Session kommt, (b) neu geschriebene `~/.claude/CLAUDE.md` als Minimal-Fallback, (c) `autoMemoryEnabled: false` + Hook-Verifikation, (d) Triage-Migration der Alt-Memories über flow-MCP-Tools.

**Tech Stack:** flow-MCP-Tools (`flow_create_doc`, `flow_update_doc`, `flow_curate_context`, `flow_context_inventory`, `flow_search_docs`, `flow_set_active_context`), `flow`-CLI, `~/.claude/settings.json`, Markdown.

**Spec:** `docs/superpowers/specs/2026-07-16-claude-md-flow-loop-design.md` (Commit `50c82d9`; flow: `specs/2026-07-16-claude-md-flow-loop-design`).

## Global Constraints

- **Keine Änderungen an flow-server / flow / flow-mcp Code** (Spec §9).
- **`type: "agent"` nie verwenden** — deprecated; gültige Typen: `daily, project, free, memory, instruction, skill, plan, spec, activecontext`.
- **Vor jedem `flow_create_doc` erst `flow_search_docs`** — existiert das Doc (gleicher Slug/Titel), dann `flow_update_doc` statt Duplikat.
- **Kontrakt-Doc < 1k Tokens** (Spec §10 Budget-Risiko); Server-Budget ist 12k.
- **`~/.claude/CLAUDE.md` und `CLAUDE-*.md` nie in git committen.** Nur die Plan-/Spec-Dateien unter `docs/superpowers/` werden im flow-Repo (`rebuild`-Branch) committet.
- **Maschinen-Status (dieses MacBook, 2026-07-16):** SessionStart+Stop-Hooks sind in `~/.claude/settings.json` **bereits installiert**; `autoMemoryEnabled` fehlt noch.
- Backups vor destruktiven Schritten: Alt-CLAUDE.md und Memory-Verzeichnis werden verschoben/kopiert, nie ersatzlos gelöscht.
- Schreibkonvention in allen verfassten Texten: *Soenne* / *Claude*, keine Pronomen „ich/du" in CLAUDE.md-Dateien; keine Emoji-Pictogramme (monospace Glyphen ok).

---

### Task 1: Kontrakt-Doc `claude-code-flow-kontrakt` in flow anlegen und auf „immer" kuratieren

**Files:**
- Kein Dateisystem — flow-Docs via MCP.

**Interfaces:**
- Produces: globales `instruction`-Doc, Pfad `instructions/claude-code-flow-kontrakt`, `project: none`, Kontext-Modus `immer`. Task 2 referenziert es namentlich in der CLAUDE.md; Task 7 prüft sein Standing.

- [ ] **Step 1: Duplikat-Check**

`flow_search_docs` mit `query: "claude-code-flow-kontrakt"`, `project: "global"`. Erwartet: kein Treffer mit diesem Pfad. Falls Treffer: statt Step 2 ein `flow_update_doc` mit demselben Body.

- [ ] **Step 2: Doc anlegen**

`flow_create_doc` mit `project: "none"`, `type: "instruction"`, `path: "instructions/claude-code-flow-kontrakt"`, `title: "Claude Code ↔ flow — Kontrakt (SSOT-Regeln)"`, `tags: ["claude-code", "kontrakt", "kontext", "ssot"]` und exakt diesem Body:

```markdown
# Claude Code ↔ flow — Kontrakt

flow ist die erste Quelle der Wahrheit für Kontext. Der `# flow context`-Block
(SessionStart-Hook) ersetzt natives Auto-Memory und Memory-Bank vollständig.

## Typen-Matrix — was wohin

| Inhalt | Typ | Projekt/Scope |
|--------|-----|---------------|
| Design-Spec | `spec` | Repo-Projekt (project weglassen), Pfad `specs/YYYY-MM-DD-<topic>-design` |
| Implementation-Plan | `plan` | Repo-Projekt, Pfad `plans/YYYY-MM-DD-<topic>` |
| Repo-Konventionen (AGENTS.md) | `instruction` | Repo-Projekt |
| Arbeitsweise-/Faktenwissen | `memory` | engster passender Tier: repo (leaf) → vorhaben → engagement → global (`project: "none"`) |
| Wiederverwendbare Anleitung | `skill` | global oder Repo |
| Projektnotizen/Backlog | `project` | Repo-Projekt, Pfad `notes/<topic>` |
| Ohne Projektbezug | `free` | `project: "none"` |

`agent` ist deprecated — nie verwenden. Vor jedem Create: `flow_search_docs`;
existiert das Doc, dann `flow_update_doc`/`flow_patch_doc` statt Duplikat.
Tags als flache Liste im Tool-Call, nie als YAML-Frontmatter im Body.

## Active Context (Flush-Pflicht)

Bei Handoff oder Arbeitsende `flow_set_active_context` mit: Stand (Datum,
Branch/Commit, clean/uncommitted) · Wo war ich · Was offen · Nächster Schritt.
Unter 600 Tokens halten; der Text ersetzt den gesamten vorherigen Stand.
Der Stop-Hook erinnert nur — die Pflicht gilt auch ohne Reminder.

## Kuratierung & Budget

- Kontext-Modus je Doc: `auto` (gerankt) · `immer` (jede Session) · `nie`.
  `immer` sparsam einsetzen — das Budget (Standard 12k Tokens) ist geteilt.
- Pins nur für akut Kritisches. Meldet die Budgetzeile `!! pinned not shown`,
  sofort beheben: `flow_context_inventory` → `flow_curate_context`.
- Nach größeren Doc-Wellen `flow_context_inventory` prüfen.

## Recall

- Vor Brainstorming, Planung und Session-Resume: `flow_search_docs`
  (Repo-Scope; `project: "global"` für Cross-Projekt-Recall).
- Die Suche ist hybrid (Keyword + semantisch) — natürliche Sprache funktioniert.

## Artefakte

Binäres/Generiertes (Screenshots, Reports, HTML): `flow_upload_artifact` mit
`path`-Parameter; `free: true` nur für Projektloses.

## Unbound Repo

Meldet der Kontext „repo not bound": `flow_bind_project` vorschlagen
(bei neuen Repos erst Soenne fragen, wohin der Node gehört).
```

- [ ] **Step 3: Auf „immer" kuratieren**

Mit der `id` aus der Create-Antwort: `flow_curate_context` mit `{id: "<id>", mode: "immer"}`. Erwartet: Antwort zeigt Standing `always` (bzw. included mit Modus immer).

- [ ] **Step 4: Verifikation im Render**

Run: `flow context --path "$PWD" | head -40`
Erwartet: Sektion `## Instructions` enthält `Claude Code ↔ flow — Kontrakt`; Budgetzeile ohne `!! pinned not shown`.

---

### Task 2: Neue globale `~/.claude/CLAUDE.md` schreiben

**Files:**
- Backup: `~/.claude/CLAUDE.md` → `~/.claude/CLAUDE.md.2026-07-16.bak`
- Ersetzen: `~/.claude/CLAUDE.md`

**Interfaces:**
- Consumes: Kontrakt-Doc-Name `claude-code-flow-kontrakt` (Task 1).
- Produces: die maschinenlokale Bootstrap-Datei; Task 6 löscht die dort nicht mehr erwähnten Memory-Bank-Bestände.

- [ ] **Step 1: Backup**

Run: `cp ~/.claude/CLAUDE.md ~/.claude/CLAUDE.md.2026-07-16.bak && ls -la ~/.claude/CLAUDE.md.2026-07-16.bak`
Erwartet: Backup-Datei existiert (6349 Bytes).

- [ ] **Step 2: Neue Datei schreiben (kompletter Inhalt, verbatim)**

```markdown
# CLAUDE.md

## HARD RULES — NEVER VIOLATE

### Banned CLI Tools

The PreToolUse hook `.claude/hooks/enforce-fast-tools.sh` blocks these. Do not attempt them.

| BANNED | USE INSTEAD | EXAMPLE |
|--------|-------------|---------|
| `tree` | `fd` | `fd . -t f` (files), `fd . -t d` (dirs) |
| `find` | `fd` | `fd "pattern"`, `fd -e js` |
| `grep`, `grep -r` | `rg` | `rg "pattern"`, `rg -i "term"` |
| `egrep`, `fgrep` | `rg` | `rg "pattern"` |
| `ls -R` | `fd` or `rg --files` | `rg --files`, `fd . -t f` |
| `cat file \| grep` | `rg` | `rg "pattern" file` |

**Exception:** `grep` after a pipe is acceptable (e.g. `rg --files | grep "pattern"`).

### Banned Behaviors

- NEVER speculate about code without reading it first. If Soenne references a file, Claude MUST open and inspect it before answering.
- NEVER commit CLAUDE.md or CLAUDE-*.md files. Never delete them without an approved migration plan.
- NEVER write to Claude Code's native auto-memory (`MEMORY.md` / memory directory). Persistent knowledge lives in flow.
- Ignore GEMINI.md and GEMINI-*.md files.

### Required Behaviors

- ALWAYS verify the solution works before declaring done.
- ALWAYS clean up temporary files, scripts, or helpers at the end of a task.
- For new branches or multi-file changes, run `git worktree list` first. If other worktrees exist, ask Soenne which one the work belongs in. For larger changes, propose `git worktree add` rather than working on the current branch directly.

## flow — erste Quelle der Wahrheit für Kontext

Der `# flow context`-Block im Session-Kontext (SessionStart-Hook) ist die
maßgebliche Wahrheit über Projektstand, Memories und Regeln. Er ersetzt
natives Auto-Memory und die frühere Memory-Bank. Bei Widerspruch gilt:
Code/Repo für Fakten, flow für Stand und Absicht — Diskrepanzen ansprechen.

- **Bootstrap-Fallback:** Fehlt der `# flow context`-Block in einer Session,
  einmalig `flow context install-hooks` ausführen (idempotent, pro Maschine)
  und für die laufende Session `flow_get_context` (MCP) bzw. `flow context`
  (CLI) rufen.
- **Schreibdisziplin:** Neue dauerhafte Erkenntnis → `flow_create_doc` /
  `flow_update_doc` mit korrektem Typ (`spec`, `plan`, `memory`,
  `instruction`, `skill`, …). Erst `flow_search_docs`, dann schreiben —
  Update schlägt Duplikat. Niemals natives Auto-Memory.
- **Flush-Pflicht:** Bei Handoff oder Arbeitsende `flow_set_active_context`
  (wo war ich / was offen / nächster Schritt). Der Stop-Hook erinnert nur —
  die Regel gilt auch ohne Reminder.
- **Recall:** Vor Brainstorming, Planung und Resume `flow_search_docs`.
- **Unbound repo:** Meldet der Kontext „repo not bound" → `flow_bind_project`
  vorschlagen.
- **Detailregeln** (Typen-Matrix, Kuratierung, Artefakte) liefert das
  instruction-Doc `claude-code-flow-kontrakt` im flow-Kontext.

## How Soenne and Claude Work Together

**Writing convention for CLAUDE.md files:** Always use *Soenne* (or "the human") and *Claude* instead of pronouns. Never use "I", "you", "me", "my", "your".

**Workflow — for non-trivial work** (multi-file changes, new features, refactors, anything ambiguous):

1. Discuss strategy before writing code.
2. Ask clarifying questions one at a time so Soenne can give complete answers.
3. Get approval on the approach.
4. Plan at high level (project goals/flow) and task level (files/details).
5. Implement only after both levels are approved.

**For trivial work** (rename, typo, single-line fix, clearly-scoped edit Soenne already specified): proceed directly. No ceremony, no trailing "any questions?".

## Subagent Routing

Prefer these specialized subagents over the built-in `Explore` / `general-purpose` agents:

- **`code-searcher`** — first choice for any codebase search, location lookup, forensic analysis, pattern detection, security review, or "where is X / how does Y work" question. Use this INSTEAD of the built-in `Explore` agent. Supports Chain-of-Draft mode if Soenne asks for "CoD", "concise", or "brief".
- **`ux-design-expert`** — trigger automatically for: dashboards, data visualizations (especially Highcharts), Tailwind CSS work, design systems / component libraries, complex user flows, premium-UI requests, accessibility (WCAG) reviews — anything where the user experience is the deliverable. Claude proposes this agent proactively, does not wait to be asked.

When delegating, give the subagent full context background — it does not see the conversation history, and it does not receive the `# flow context`-Block. Relevant flow-Kontext gehört in den Dispatch-Prompt.

## Claude Code Documentation

For work on Claude Code internals (hooks, skills, subagents, MCP servers, plugins), use the `claude-docs-consultant` skill to fetch from docs.claude.com selectively.

## CLI Quick Reference

```
"list/show/explore files"   → fd . -t f   OR   rg --files
"search text content"       → rg "pattern"
"find file by name"         → fd "name"
"directory structure"       → fd . -t d
"current directory only"    → ls -la
```

Search strategy: start broad → narrow (`rg "partial" | rg "specific"`); filter by type early (`rg -t py`); batch alternations (`rg "(a|b|c)"`); limit scope to a path when known.
```

- [ ] **Step 3: Verifikation**

Run: `rg -c "Memory Bank|memory-bank-synchronizer|type: \"agent\"|docs/superpowers/\*\*" ~/.claude/CLAUDE.md || echo "sauber"`
Erwartet: `sauber` (keine Alt-Reste). Zusätzlich: `rg -n "claude-code-flow-kontrakt|install-hooks|flow_set_active_context" ~/.claude/CLAUDE.md` → 3+ Treffer.

---

### Task 3: `autoMemoryEnabled: false` setzen + Hooks verifizieren

**Files:**
- Modify: `~/.claude/settings.json` (Top-Level-Key ergänzen; Hooks-Sektion unangetastet lassen)

**Interfaces:**
- Produces: deaktiviertes Auto-Memory (MEMORY.md wird nicht mehr geladen — offizieller Key laut code.claude.com/docs/en/memory: `autoMemoryEnabled`, Env-Alternative `CLAUDE_CODE_DISABLE_AUTO_MEMORY=1`).

- [ ] **Step 1: Setting setzen (JSON-sicher via python3)**

```bash
python3 - <<'EOF'
import json, pathlib
p = pathlib.Path.home() / ".claude" / "settings.json"
d = json.loads(p.read_text())
d["autoMemoryEnabled"] = False
p.write_text(json.dumps(d, indent=2) + "\n")
print("autoMemoryEnabled =", d["autoMemoryEnabled"])
EOF
```

Erwartet: `autoMemoryEnabled = False`.

- [ ] **Step 2: Hooks verifizieren (bereits installiert, idempotent bestätigen)**

Run: `flow context install-hooks`
Erwartet: `hooks already installed`. (Falls stattdessen „installed …": auch ok — dann waren sie auf dieser Maschine doch nicht komplett.)

- [ ] **Step 3: settings.json valide?**

Run: `python3 -c "import json,pathlib;json.loads((pathlib.Path.home()/'.claude'/'settings.json').read_text());print('valide')"`
Erwartet: `valide`.

---

### Task 4: Migration `feedback_*` / `reference_*` → flow-Memories

**Files:**
- Read: `/Users/msoent/.claude/projects/-Users-msoent-SourceCode-serverkraken-flow/memory/*.md` (Quelle; Bodies dort lesen)

**Interfaces:**
- Consumes: Typen-Matrix aus Task 1.
- Produces: je Alt-Memory ein flow-`memory`-Doc, `path` = Alt-Dateiname ohne `.md` (konsistent mit bestehenden Mirrors wie `reference_soenne_worktime_workflow`).

**Regeln:** Body = Inhalt der Alt-Datei ohne YAML-Frontmatter; Tags aus Frontmatter/Inhalt als flache Liste. Pro Doc erst `flow_search_docs` (query = Slug, `project: "global"`): existiert der Slug schon (etliche wurden früher gespiegelt) → `flow_update_doc` nur, wenn die Alt-Datei neuer/vollständiger ist, sonst überspringen und als „bereits in flow" notieren.

**Triage-Tabelle (vollständig — 34 Dateien):**

| Alt-Datei (ohne .md) | Ziel-Projekt | Modus |
|---|---|---|
| feedback_no_icons | none (global) | auto |
| feedback_always_recommend_with_pros_cons | none | **immer** |
| feedback_no_monoliths | none | **immer** |
| feedback_generic_features | none | auto |
| feedback_dont_descope_hobby_projects | none | auto |
| feedback_plan_main_wiring_task | none | auto |
| feedback_long_lived_integration_branch | none | auto |
| feedback_subagent_git_commits_isolated | none | auto |
| feedback_subagent_model_never_inherit_fable | none | auto |
| feedback_charm_v2_width_gotcha | none | auto |
| feedback_macos_keychain_2kb_limit | none | auto |
| feedback_go_keyring_base64_prefix | none | auto |
| feedback_authentik_blueprint_oidc_default_grants_empty | none | auto |
| feedback_authentik_subhash_unusable_with_static_allowlist | none | auto |
| feedback_authentik_device_flow_setup | none | auto |
| feedback_authentik_issuer_modes | none | auto |
| feedback_authentik_offline_access_refresh_token | none | auto |
| reference_homelab_study_gitops_quirks | none | auto |
| reference_charm_v2_api_skills | none | auto |
| reference_gemini_cli_oauth_dead | none | auto |
| feedback_flow_is_multi_tenant | flow-Repo (weglassen) | **immer** |
| feedback_navigation_discoverability_over_minimalism | flow-Repo | auto |
| feedback_tui_palette_contextual_commands | flow-Repo | auto |
| feedback_search_partial_word_recall | flow-Repo | auto |
| feedback_pgstore_goose_migrations | flow-Repo | auto |
| feedback_design_must_stay_easily_changeable | flow-Repo | auto |
| feedback_tailwind_v4_templ_gotchas | flow-Repo | auto |
| feedback_htmx_hxboost_blocks_oidc_redirect | flow-Repo | auto |
| feedback_flow_next_image_stale_mirror | flow-Repo | auto |
| feedback_flow_mcp_reauth_wedge | flow-Repo | auto |
| reference_flow_launch_modes | flow-Repo | auto |
| reference_soenne_worktime_workflow | flow-Repo | auto (existiert vermutlich schon) |
| reference_flow_dev_env | flow-Repo | auto |
| reference_flow_prod_deploy | flow-Repo | auto |

- [ ] **Step 1:** Inventar ziehen: `ls /Users/msoent/.claude/projects/-Users-msoent-SourceCode-serverkraken-flow/memory/` und mit der Tabelle abgleichen; nicht gelistete `feedback_*`/`reference_*`-Dateien nach denselben Regeln zuordnen (flow-spezifisch → flow-Repo, sonst global) und in der Abschlussmeldung ausweisen.
- [ ] **Step 2:** Für jede Zeile: Datei lesen → Duplikat-Check → `flow_create_doc` (`type: "memory"`, `path` = Slug, `project` laut Tabelle: `"none"` oder weglassen) bzw. Update/Skip laut Regeln.
- [ ] **Step 3:** Für die drei **immer**-Zeilen: `flow_curate_context` mit `mode: "immer"` (für `feedback_flow_is_multi_tenant` mit `repo`-Default = flow-Repo; für die zwei globalen ohne repo-Bezug ebenfalls über das flow-Repo kuratieren — Modus ist doc-global).
- [ ] **Step 4: Verifikation**

Run: `flow context --path "$PWD" | rg -c "Multi-Tenant|Monolithen|Empfehlung"` — Erwartet: ≥ 2 (immer-Memories erscheinen). Danach `flow_context_inventory`: kein `dropped`-Standing bei den drei immer-Docs, Budgetzeile ohne `!!`.

---

### Task 5: `project_*`-Triage — offene Backlog-Items in ein flow-Doc

**Interfaces:**
- Produces: `project`-Doc `notes/backlog-offene-slices` im flow-Repo-Projekt.

**Regel:** DONE-Historie wird NICHT migriert (liegt als Spec/Plan-Docs in flow). Nur offene Enden. Vor dem Schreiben jede Alt-`project_*`-Datei überfliegen, ob weitere offene Punkte genannt sind; die bekannte Startliste:

- M4 Slice 4+5: CLI `project`-Verben + TUI session-edit Picker (nicht gestartet)
- Project-Mgmt-UI-Lücke: create/delete/rename nur CLI; unbind-500-Bug
- L5-Runbook-Rest: 8 Globals → Modus „immer" + PROD-Deploy (Teil davon erledigt dieser Plan — abgleichen)
- Lesesaal L6 Artefakte: 8-Task-Plan ready, SDD-Ausführung ausstehend (Stand prüfen — Free-Artifacts/FR-Slice sind inzwischen DONE)
- tmux-Status Slice 1: Soenne-Live-Gate + dotfiles `bind E` → display-popup + Merge → rebuild
- Free-Artifacts: Browser-Gate + PROD-Deploy offen
- FR-Slice (node update + README): Browser-Dogfood + PROD-Deploy offen
- Embed poison-doc storm (Säule B): Spec+Plan ready, nicht ausgeführt; B5 Ollama-infra deferred
- K5 structural chip follow-up: H1 CSS `:has()`-Demote; Chip-Revival = Route-Ctx durch 16 AppShell-Caller
- Legacy TUI → `Sem()`-Migration: geplant, nicht gestartet
- Offener Review-Strang: F05/F49 (mehrstufige Writes, ICS-Token-Rotation atomar), danach F26 (`reviews/2026-07-15-rebuild-code-review`)

- [ ] **Step 1:** Duplikat-Check (`flow_search_docs`, query "backlog offene slices").
- [ ] **Step 2:** `flow_create_doc` (`type: "project"`, `path: "notes/backlog-offene-slices"`, `title: "Backlog — offene Slices & Gates (aus Alt-Memories, Stand 2026-07-16)"`, `tags: ["backlog", "triage"]`), Body = obige Liste als Markdown-Checkliste, je Item ein Satz Kontext + Quelle (`project_*`-Slug).
- [ ] **Step 3: Verifikation:** `flow_get_doc` auf das neue Doc; Liste vollständig gegen die Startliste.

---

### Task 6: Alt-Systeme stilllegen — MEMORY.md-Stub, Memory-Archiv, `CLAUDE-*.md`-Scan, stale flow-Mirrors

**Files:**
- Modify: `/Users/msoent/.claude/projects/-Users-msoent-SourceCode-serverkraken-flow/memory/MEMORY.md` (→ Stub)
- Move: alle anderen `*.md` dieses Verzeichnisses → Unterordner `_migrated-to-flow/`

- [ ] **Step 1: Memory-Dateien archivieren**

```bash
cd /Users/msoent/.claude/projects/-Users-msoent-SourceCode-serverkraken-flow/memory
mkdir -p _migrated-to-flow
fd -d 1 -e md -E MEMORY.md . -x mv {} _migrated-to-flow/
ls *.md          # erwartet: nur MEMORY.md
```

- [ ] **Step 2: MEMORY.md-Stub schreiben (verbatim, kompletter Inhalt)**

```markdown
# Memory lebt in flow

Dieses native Auto-Memory ist stillgelegt (`autoMemoryEnabled: false`).
Nichts hier ergänzen. Persistentes Wissen: flow (`# flow context`-Block,
`flow_search_docs`, `flow_create_doc`). Alt-Bestand: `_migrated-to-flow/`
(migriert am 2026-07-16, Plan `plans/2026-07-16-claude-md-flow-loop`).
```

- [ ] **Step 3: `CLAUDE-*.md`-Scan**

Run: `fd -H "CLAUDE-.*\.md" ~/SourceCode ~/.claude -E "*plugins*" -E "*_migrated*"`
Je Fund: Inhalt prüfen — Verwertbares als `memory`-Doc ins zuständige Repo-Projekt (Duplikat-Check! das flow-Repo ist größtenteils schon gespiegelt), dann Datei löschen. Leere/stale Dateien direkt löschen. Jede Löschung in der Abschlussmeldung auflisten.

- [ ] **Step 4: Stale flow-Mirrors archivieren**

`flow_list_docs` mit `project: "none"`, `type: "instruction"`: Docs, die die ALTE globale CLAUDE.md spiegeln (Titel/Pfad mit „CLAUDE.md", „claude-md" o. ä. — nicht das neue Kontrakt-Doc!), per `flow_archive_doc` archivieren (`confirm: true`). Ebenso via `flow_search_docs` (`query: "Memory Bank CLAUDE-activeContext"`, `project: "global"`) nach Memory-Bank-Spiegeln suchen, die den neuen Regeln widersprechen → archivieren.

- [ ] **Step 5: Verifikation:** Neue Session-Simulation: `flow context --path "$PWD"` enthält keinen Verweis mehr auf Memory-Bank/`type: "agent"`; `wc -l MEMORY.md` ≈ 7.

---

### Task 7: End-to-End-Smoke (DoD Spec §8)

- [ ] **Step 1 (automatisierbar):** `flow context --path "$PWD"`: (a) `## Instructions` enthält Kontrakt-Doc, (b) `## Always` bzw. Memories enthalten die drei immer-Docs, (c) Budgetzeile ohne `!! pinned not shown`.
- [ ] **Step 2 (automatisierbar):** Test-Write: `flow_create_doc` (`type: "free"`, `project: "none"`, `path: "notes/smoke-2026-07-16"`, Body „smoke") → Antwort `type: "free"` und korrektes Projekt → sofort `flow_delete_doc`.
- [ ] **Step 3 (automatisierbar):** `flow_context_inventory`: Kontrakt-Doc Standing `always`; keine `dropped` pinned/immer-Docs.
- [ ] **Step 4 (Soenne-Gate, frische Session):** Neue Claude-Code-Session in einem flow-gebundenen Repo öffnen: `# flow context`-Block erscheint automatisch, MEMORY.md-Inhalt erscheint NICHT mehr. Nach mutierender Arbeit ohne Flush stoppen → Reminder erscheint; nach `flow_set_active_context` → kein Reminder.

---

### Task 8: Abschluss — Plan-Status, Spiegel, Active Context

- [ ] **Step 1:** Diesen Plan in flow spiegeln: Duplikat-Check, dann `flow_create_doc` (`type: "plan"`, `path: "plans/2026-07-16-claude-md-flow-loop"`, Body = diese Datei) bzw. `flow_update_doc` mit finalem Stand (abgehakte Checkboxen).
- [ ] **Step 2:** `flow_set_active_context`: Umbau abgeschlossen, Soenne-Gate aus Task 7 Step 4 offen/erledigt, nächster Schritt (z. B. andere Maschinen: nur `flow context install-hooks` + neue CLAUDE.md kopieren — Datei ist maschinenlokal!).
- [ ] **Step 3:** Aufräumen: keine temporären Dateien übrig; `git status` im flow-rebuild-Repo clean bis auf committete Plan-/Spec-Dateien.

---

## Self-Review-Notizen (bereits eingearbeitet)

- Spec §4–§8 vollständig durch Tasks 1–7 abgedeckt; §6 „Auto-Memory-Mechanismus klären" ist aufgelöst (`autoMemoryEnabled: false`, verifiziert via claude-code-guide gegen code.claude.com/docs/en/memory).
- Hook-Installation war als Task geplant, ist auf dieser Maschine aber seit 2026-07-16 abends bereits erfolgt → Task 3 verifiziert nur noch idempotent.
- Zweite Maschine (Notebook B): dort sind nur zwei Schritte nötig — `flow context install-hooks` + neue CLAUDE.md übernehmen + `autoMemoryEnabled: false`; als Hinweis im Active Context (Task 8), bewusst kein eigener Task (andere Maschine, andere Session).
