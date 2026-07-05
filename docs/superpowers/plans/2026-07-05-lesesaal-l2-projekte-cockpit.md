# Lesesaal L2 — Projekte + Cockpit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Die zwei zentralen Bestandsseiten auf Lesesaal-Sprache umbauen: **Projekte** (`/nodes`) wird der **Baum als Inhalt** (Engagement-Header mit Doppellinie → Vorhaben-Zwischenköpfe → Repo-Zeilen mit Kurzname groß + vollem Mono-Pfad), und das **Cockpit** (`/nodes/{id}`) wird das **Pfad-Rückgrat** (96er-Identität) + instr-Band + zwei-Flächen-Meta-Spalte (Kette · Bindings). Die alte Kristall-/Glyph-/Tab-Welt stirbt auf diesen Flächen; alle Funktionen (Timer start/stop/switch, Nachbuchen, Session-Edit/Delete, Bindings, Struktur/Move/Status, Rollup Work/Privat, Puls) bleiben erhalten.

**Architecture:** Das Cockpit wird **von Tab-Umschaltung auf eine einzige scrollende Seite** umgestellt (Mockup v2.4 hat keine Tabs; Spec §5/§9 „Pfad-Rückgrat ersetzt den Baum, Kinder stehen als Listen im Inhalt"). Drei benannte SSE-Container ersetzen die drei alten: `#cockpit-head` (Rückgrat + instr-Band, Fragment `GET /nodes/{id}/head`), `#cockpit-main` (Inhalts-Sektionen: Enthält · Wissen · Buchungen · Puls, Fragment `GET /nodes/{id}/main`), `#cockpit-rail` (Kette · Bindings, Fragment `GET /nodes/{id}/rail`). Der Server-Struct hat für alles bereits Felder — **kein `cmd/flow-server/main.go`-Change** außer, falls neue Usecase-Felder nötig werden (Task 7 prüft das explizit). Neue Anzeige-Logik lebt in reinen, unit-getesteten Go-Buildern (`webui`-Paket, domain-frei); die templ-Komponenten nehmen fertige Strings. Verwalten-Aktionen (Move) ziehen auf die bestehende Node-Edit-Seite (Spine-Meta „Bearbeiten" = die eine Verwalten-Tür; Status + Löschen liegen dort schon).

**Tech Stack:** Go 1.x · templ · Tailwind v4.1.5 (CLI, `make web`) · htmx (vendored, SSE-Extension) · Schibsted Grotesk + JetBrains Mono (L1 vendored). Logo-Backend (Migr 0027 width/height, `webui.LogoShape`, `GET /nodes/{id}/logo`) existiert und trägt das 96er-Render.

**Spec:** `docs/superpowers/specs/2026-07-04-lesesaal-webui-redesign-design.md` (§5/§7/§8/§9/§10/§11/§12) · **Normatives Mockup:** `docs/superpowers/specs/assets/2026-07-03-lesesaal/lesesaal.html` (v2.4 — bei Zweifel gewinnt das Mockup; Projekte = Zeilen 442–570, Cockpit = 571–675, CSS = 20–322).

## Global Constraints

- Branch **`lesesaal-l2`** (off `rebuild` `e6ceb45`, L1 gemerged); Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`.
- **NIE `make fmt`** ausführen. **NIE `git stash`** in Dispatches. Nach jedem Task: `git log --oneline -3` prüfen, dass HEAD vorangegangen ist (+ `git diff --stat HEAD~1`).
- `make ci` muss am Task-Ende grün sein (Gate 75 %, `*_templ.go` ausgeschlossen; pgstore-Tests brauchen den Podman-Socket — `DOCKER_HOST` auf den Podman-Socket setzen wie bisher).
- Nach JEDER `.templ`-Änderung: `make generate` und die `*_templ.go` mitcommitten. Nach JEDER `web/tailwind.css`-Änderung: `make web` und `internal/adapter/webui/static/app.css` mitcommitten (verify-css ist ein Drift-Diff).
- i18n: jede neue Nutzertext-Zeile in **beiden** Katalogen (`internal/i18n/catalog_de.go` + `catalog_en.go`); de+en-Parität ist test-enforced.
- Keine Emojis, keine Browser-Popups (`verify-no-popups` — Copy-Affordanz ohne `alert/confirm/prompt`), **owner-scoped** bleibt überall unangetastet (jede Store-Query trägt `u.ID`; „ist nur ein User" ist keine Begründung).
- **Farb-Gesetz (Spec §7):** Farbe pro Projekt existiert NUR im Avatar (`.av-a`…`.av-f`, deterministisch aus dem Namen). Kinds bleiben **neutrale Text-Chips** (`.typechip`, kein Glyph, keine Tönung pro Kind). Die alte Formcodierung ◆▲● ist auf allen L2-Flächen tot.
- **Eindämmung (Spec §11):** Jeder Pfad-/Namens-Render braucht Containment. Kurzname groß + voller Pfad als Mono-Zweitzeile mit `word-break:break-all` (`.fullpath`, `.row .path`); Ellipsis+`title` nur in schmalen Panel-Zeilen (`.krow .n`, `.timerpill .on`). Flex-Kinder mit Textinhalt: `min-width:0`. Die Seite pannt **nie** horizontal (html/body `overflow-x:clip` steht seit L1).
- **Design nur über Tokens/Primitives/benannte Klassen** (Gate-Punkt #6): wo das Mockup harte Maße vorgibt, eine **benannte Klasse** in `web/tailwind.css` anlegen (Task 1) statt neue Arbitrary-`[px]` zu streuen. Die L1-mockup-mandatierten Arbitrary-px (appshell/timerwidget/palette) bleiben unberührt.
- **SSE-Regel:** jede Mutation emittiert ihr Event; der Container, der es konsumiert, ist im jeweiligen Task benannt. Timer tickt **client-seitig** (`data-timer`), NICHT per SSE.
- Tailwind-v4-Fallen (Memory): kein `<alpha-value>` in `@theme`; niemals `*/` in CSS-Kommentaren; `@source not`-Zeilen nicht anfassen.
- **rg-Verifikation vor jeder Bestandsnutzung (Prozess-Pflicht):** JEDES im Plan als „Bestand" referenzierte Symbol (Template, Helfer, Handler, VM-Feld, Komponente — z. B. `NodesFragment`, `nodesOuter`, `nodeCrumbs`, `secStr`, `fmtClockHMS`, `activityFeedRow`, `AppShell`, `sessionDialogAddVM`, `bindingTarget`, `wissenTabDocs`) vor dem Tippen per `rg -n "<Name>" internal/ -g '!*_templ.go'` gegen den echten Code prüfen. **Bestand gewinnt** — Signaturen/Feldnamen exakt übernehmen, nichts erfinden. Die im Plan gezeigten Signaturen sind stichprobenverifiziert, aber der Implementer prüft am Punkt der Verwendung erneut.

## Agent-Besetzung & Dispatch-Protokoll (übernommen aus L1, gilt sinngemäß für L2)

Rollen als Projekt-Agents in `.claude/agents/` (Modell + Effort im Frontmatter fest). Orchestrator-Session `/effort high`. Dispatches nennen das Modell NIE implizit (Memory: nie Fable erben).

| Task | Agent (`subagent_type`) | Modell · Effort |
|---|---|---|
| 1 L2-CSS-Klassen | `lesesaal-implementer` | Sonnet · medium |
| 2 Go-Helfer (Dedup/Doc-Typ-Chip) | `lesesaal-implementer` | Sonnet · medium |
| 3 Projekte-Baum | `lesesaal-implementer-deep` | Sonnet · high |
| 4 Cockpit-Rückgrat + instr-Band | `lesesaal-implementer-deep` | Sonnet · high |
| 5 Cockpit-Rail (Kette + Bindings) | `lesesaal-implementer` | Sonnet · medium |
| 6 Cockpit-Inhalt (Enthält/Wissen/Buchungen/Puls) | `lesesaal-implementer-deep` | Sonnet · high |
| 7 Flatten-Wiring (Shell + SSE + Tabs sterben + Move→Edit) | `lesesaal-implementer-deep` | Sonnet · high |
| 8 Wiring-Gate | `lesesaal-implementer` | Sonnet · medium |
| jedes Task-Review | `lesesaal-task-reviewer` | Haiku · high |
| Slice-Ende: Whole-Branch | `lesesaal-final-reviewer` | Opus · xhigh |
| Slice-Ende: Design-Treue | `lesesaal-mockup-auditor` | Sonnet · medium |

**Protokoll pro Task:**
1. Dispatch Implementer mit: wörtlichem Task-Text + Global-Constraints-Block + „Branch `lesesaal-l2`, Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`". Ein Task pro Dispatch.
2. Orchestrator verifiziert danach selbst: `git log --oneline -3` (HEAD vorangegangen? — Subagent-Commits können den Branch-Ref verfehlen, Memory) + `git diff --stat HEAD~1`.
3. Dispatch `lesesaal-task-reviewer` mit Task-Text + Commit-Range. `Rejected`/Critical → Fix-Dispatch an denselben Implementer; Minor darf der Orchestrator selbst fixen.
4. Ledger `.superpowers/sdd/progress.md` fortschreiben (Commits, Verdikt, ci-Stand).

**Protokoll Slice-Ende (feste Reihenfolge):**
1. `make ci` grün.
2. **Rest-Sweep** (mechanisch): `gemini-bigcontext` (agy) über `git diff --name-only rebuild..HEAD`; Fallback `code-searcher` (gemini-CLI ggf. tot). Dispatch-Text unten.
3. `lesesaal-final-reviewer` (Range `rebuild..HEAD`) → Findings fixen.
4. `lesesaal-mockup-auditor` → Abweichungen fixen.
5. **Soenne-Live-Gate** (Browser, nicht delegierbar) — inkl. 375px-Sichtprobe (kein horizontales Pannen; laufender Timer-Fall).
6. Nachlauf: Auto-Memory + flow-Mirror des Ledgers/Plans (`flow_update_doc`).

**Dispatch-Text Rest-Sweep (`<RANGE>` = `rebuild..HEAD`):**
> Lies vollständig: alle Dateien aus `git diff --name-only <RANGE>` plus `web/tailwind.css` und `internal/adapter/webui/static/app.css`. Finde ausschließlich: (a) **Kristall-/Glyph-Reste** — `glass`, `shadow-lift`, `pill-tab`, `cockpitAccent`, `cockpitTileClass`, Formcodierungs-Glyphen ◆/▲/●/⬡ in UI-Markup auf L2-Flächen (nodes.templ/cockpit*.templ/activity_row.templ); (b) **Arbitrary-Tailwind-Werte** (`text-[#`, `bg-[#`, `rounded-[`, `shadow-[`, `w-[`, `h-[`, `text-[1`) auf L2-Flächen, wo eine benannte Lesesaal-Klasse existiert (§6-Token / Task-1-Klassen); (c) **verwaiste i18n-Keys** (in Katalogen definiert, nirgends per `T(`/`Tn(` referenziert) und **tote Cockpit-Tab-/Uebersicht-Reste** (`CockpitTabs`, `cockpitPanel`, `/tab/`, `CockpitUebersicht`, `pill-tabs`). Ausgabe: gruppierte Liste `Datei:Zeile — Befund`, KEINE Fixes, KEINE Stilurteile.

**Hinweis Memory-Bank:** keine `CLAUDE-*.md` im Repo → `memory-bank-synchronizer` wird übersprungen; Nachlauf ist Orchestrator-Arbeit (Auto-Memory + flow-Mirror).

---

### Task 1: Lesesaal-L2-Komponentenklassen — Mockup-Maße als benannte Klassen

**Files:**
- Modify: `web/tailwind.css` (`@layer components` — hinter den L1-Primitives `.panel`/`.typechip`/`.ava` ergänzen)
- Modify: `internal/adapter/webui/components/styleguide.templ` (Lesesaal-L2-Sektion)
- Test: `internal/adapter/webui/components/styleguide_test.go` (falls vorhanden; sonst ein Render-Smoke)

**Interfaces:**
- Produces (für Tasks 3–7): benannte Klassen exakt aus dem Mockup-CSS (Zeilen 20–322): `.pagehead .pagehead-h1 .sub` · `.sect .sect-h .more` · `.dotsep` · `.spine .up .sep .spine-main .spine-meta .fullpath` · `.instr .stats` · `.cock` (Grid 1fr/280px, Gap 56, unter 960px 1-spaltig) · `.rail .blk` · `.krow .pin .meter .meter-l .ctxrows` · `.eng .eng-h` · `.vh .lvl1 .lvl2` · `.projrow` (Repo-Zeile) · `.btn .btn-pri .btn-q .btn-s` · `.livechip .targetlink`. (L1 hat nur `.seg`, `.panel`, `.typechip`, `.ava`, `.eyebrow` — alles andere fehlt und wird HIER angelegt.) Diese Klassen ersetzen Arbitrary-`[px]`-Streuung auf allen L2-Flächen (Gate-Punkt #6).
- **Wichtig:** Werte 1:1 aus dem Mockup übernehmen (`--w:1140px`, `--panel`, `--hairp`, Radius 14, Gap 56). Tokens `var(--panel)` etc. stehen seit L1; hier werden nur Layout-Klassen ergänzt, keine neuen Farb-Token.

- [ ] **Step 1: Mockup-CSS verifizieren** — vor dem Tippen die Klassenblöcke im Mockup lesen und wörtlich übernehmen:
```bash
sed -n '141,239p' docs/superpowers/specs/assets/2026-07-03-lesesaal/lesesaal.html
sed -n '289,322p' docs/superpowers/specs/assets/2026-07-03-lesesaal/lesesaal.html
```

- [ ] **Step 2: Failing Test** — in `styleguide_test.go` (Muster der Nachbartests; falls keine Datei existiert, neue mit `renderToBuf`-Analog `components`-Render):
```go
func TestStyleguide_HasLesesaalL2Section(t *testing.T) {
	var sb strings.Builder
	if err := components.Styleguide().Render(testCtx(t), &sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	for _, want := range []string{"spine", "eng-h", "krow", "instr"} {
		if !strings.Contains(out, want) {
			t.Fatalf("Styleguide misses Lesesaal-L2 demo of %q:\n%s", want, out)
		}
	}
}
```
(`components.Styleguide()`-Namen + Test-Ctx-Helper vorher per `rg "templ Styleguide|func testCtx" internal/adapter/webui/components/` verifizieren — Bestand gewinnt.)

- [ ] **Step 3: Test laufen lassen** — Expected: FAIL.

- [ ] **Step 4: Klassen in `web/tailwind.css` ergänzen** (im ersten `@layer components`, hinter den L1-Primitives). Wörtlich aus dem Mockup, `#hex`-Literale → `rgb(var(--token))` wo ein Token existiert (Konsistenz mit L1). Kernblöcke:
```css
  /* ── Lesesaal L2: Layout-Klassen (Mockup v2.4 Z.141–322, benannt statt arbitrary) ── */
  /* Pfad-Rückgrat */
  .spine { padding: 40px 0 0; }
  .spine .up { display: flex; flex-wrap: wrap; align-items: center; gap: 7px; font-size: 13.5px; color: rgb(var(--meta)); }
  .spine .up a { font-weight: 500; }
  .spine .up a:hover { color: rgb(var(--accent)); }
  .spine .up .sep { color: rgb(var(--hair2)); }
  .spine-main { display: flex; align-items: center; gap: 18px; margin-top: 10px; }
  .spine-main h1 { font-size: 38px; font-weight: 700; letter-spacing: -.03em; line-height: 1.1; word-break: break-word; }
  .spine-meta { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; margin-top: 10px; font-size: 13px; color: rgb(var(--meta)); }
  .spine-meta .k { border: 1px solid rgb(var(--hair2)); background: rgb(var(--surface)); border-radius: 4px; padding: 1.5px 7px; font-size: 11.5px; font-weight: 600; letter-spacing: .04em; text-transform: uppercase; color: rgb(var(--meta)); }
  .fullpath { margin-top: 9px; font-family: "JetBrains Mono", ui-monospace, monospace; font-size: 12.5px; color: rgb(var(--faint)); word-break: break-all; }
  .fullpath button { color: rgb(var(--faint)); font-family: "JetBrains Mono", ui-monospace, monospace; font-size: 12px; margin-left: 6px; border: 1px solid rgb(var(--hair)); border-radius: 4px; padding: 0 5px; }
  .fullpath button:hover { color: rgb(var(--meta)); border-color: rgb(var(--hair2)); }
  /* Cockpit-Grid + instr-Band */
  .cock { display: grid; grid-template-columns: minmax(0,1fr) 280px; gap: 56px; padding-bottom: 70px; }
  .instr { display: flex; flex-wrap: wrap; align-items: center; gap: 14px; margin-top: 26px; padding: 14px 18px; background: rgb(var(--panel)); border-radius: 14px; }
  .instr .stats { margin-left: auto; font-family: "JetBrains Mono", ui-monospace, monospace; font-size: 13px; color: rgb(var(--meta)); font-variant-numeric: tabular-nums; }
  .instr .stats b { color: rgb(var(--ink)); font-weight: 600; }
  /* Meta-Spalte (rail) */
  .rail .blk { margin-top: 18px; background: rgb(var(--panel)); border-radius: 14px; padding: 16px 18px; }
  .rail .blk:first-child { margin-top: 46px; }
  .rail .blk > .eyebrow { display: block; padding-bottom: 9px; color: rgb(var(--meta)); }
  .krow { display: flex; justify-content: space-between; align-items: baseline; gap: 10px; padding: 9px 0; border-bottom: 1px solid rgb(var(--hairp)); font-size: 13.5px; }
  .krow:last-child { border-bottom: none; }
  .krow .n { font-weight: 500; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .krow a.n:hover { color: rgb(var(--accent)); }
  .krow .v { font-family: "JetBrains Mono", ui-monospace, monospace; font-size: 12.5px; color: rgb(var(--meta)); font-variant-numeric: tabular-nums; flex-shrink: 0; }
  /* Projekte-Baum als Inhalt */
  .eng { margin-top: 46px; }
  .eng-h { display: flex; align-items: center; gap: 14px; padding-bottom: 12px; border-bottom: 2px solid rgb(var(--ink)); }
  .eng-h h2 { font-size: 22px; font-weight: 700; letter-spacing: -.02em; }
  .eng-h .right { margin-left: auto; text-align: right; }
  .eng-h .right .v { font-family: "JetBrains Mono", ui-monospace, monospace; font-size: 14px; font-weight: 600; font-variant-numeric: tabular-nums; }
  .eng-h .right .k { font-size: 11.5px; color: rgb(var(--faint)); }
  .vh { display: flex; align-items: baseline; gap: 10px; margin: 26px 0 2px; padding-bottom: 7px; border-bottom: 1px solid rgb(var(--hair2)); }
  .vh .t { font-size: 15.5px; font-weight: 600; }
  .vh .c { font-size: 12px; color: rgb(var(--faint)); }
  .lvl2 { margin-left: 26px; }
  /* Buttons + Puls-Marker (Mockup Z.108, 262–267, 275–279) */
  .btn { display: inline-flex; align-items: center; gap: 8px; border-radius: 8px; padding: 9px 16px; font-size: 14px; font-weight: 600; border: 1px solid transparent; }
  .btn-pri { background: rgb(var(--accent)); color: #fff; }
  .btn-pri:hover { background: rgb(var(--accent-deep)); }
  .btn-q { border-color: rgb(var(--hair2)); color: rgb(var(--ink)); background: rgb(var(--surface)); }
  .btn-q:hover { border-color: rgb(var(--faint)); }
  .btn-s { padding: 6px 12px; font-size: 13px; }
  .livechip { font-size: 11px; font-weight: 600; letter-spacing: .06em; color: rgb(var(--live)); background: rgb(var(--live-wash)); border-radius: 4px; padding: 2px 7px; }
  .targetlink { color: rgb(var(--accent)); font-weight: 500; }
  /* Seiten-Gerüst + Sektionen (Mockup Z.69–76; L1 hat NUR .seg — Rest fehlt) */
  .pagehead { padding: 44px 0 10px; }
  .pagehead h1 { font-size: 34px; font-weight: 700; letter-spacing: -.025em; line-height: 1.15; }
  .pagehead .sub { margin-top: 8px; font-size: 13.5px; color: rgb(var(--meta)); }
  .sect { margin-top: 40px; }
  .sect-h { display: flex; align-items: baseline; gap: 12px; padding-bottom: 10px; border-bottom: 1px solid rgb(var(--hair2)); }
  .sect-h .eyebrow { color: rgb(var(--meta)); }
  .sect-h .more { margin-left: auto; font-size: 13px; font-weight: 500; color: rgb(var(--accent)); }
  .sect-h .more:hover { color: rgb(var(--accent-deep)); }
  .dotsep { color: rgb(var(--hair2)); }
```
Und die `.row`-Varianten für den Baum + Puls (Mockup Z.98–107) — nur ergänzen, was L1 nicht schon hat (`.panel` existiert):
```css
  .projrow { display: flex; align-items: center; gap: 16px; padding: 13px 2px; border-bottom: 1px solid rgb(var(--hair)); }
  a.projrow:hover { background: rgb(var(--wash)); }
  .projrow .grow { flex: 1; min-width: 0; }
  .projrow .t { font-size: 15px; font-weight: 500; line-height: 1.35; }
  .projrow .s { margin-top: 2px; font-size: 12.5px; color: rgb(var(--meta)); }
  .projrow .path { font-family: "JetBrains Mono", ui-monospace, monospace; font-size: 12px; color: rgb(var(--faint)); margin-top: 3px; word-break: break-all; }
  .projrow .right { text-align: right; flex-shrink: 0; }
  .projrow .right .v { font-family: "JetBrains Mono", ui-monospace, monospace; font-size: 13px; font-variant-numeric: tabular-nums; }
  .projrow .right .k { font-size: 11.5px; color: rgb(var(--faint)); margin-top: 1px; }
```
Responsive-Nachtrag (Mockup Z.289–321) — **vollständig** übernehmen (Spec §12 „Sektions-Köpfe brechen um" darf NICHT fehlen):
```css
@media (max-width: 960px) {
  .cock { grid-template-columns: 1fr; gap: 0; }
  .rail .blk:first-child { margin-top: 38px; }
  .spine-main h1 { font-size: 29px; }
  .eng-h, .spine-main, .instr { flex-wrap: wrap; }   /* §12 Sektions-Köpfe brechen um */
}
@media (max-width: 620px) {
  .projrow .right .k { display: none; }               /* §12 nur Hauptwert */
  .projrow .right .v { font-size: 12px; }
  .instr .stats { margin-left: 0; width: 100%; }
  .ava.av-96 { width: 64px; height: 64px; font-size: 22px; border-radius: 15px; }
  .spine-main h1 { font-size: 27px; }
}
```
(`.pagehead{flex-wrap:wrap}` gilt für die Projekte-Kopfzeile — im Projekte-Templ per Utility oder hier ergänzen. Die `.k`-Unterzeile entfällt <620px.)

- [ ] **Step 5: Styleguide-Sektion** — in `styleguide.templ` eine Sektion „Lesesaal L2" anhängen, die zeigt: ein `.spine`-Beispiel (Kurzname + `.fullpath`), ein `.eng-h` + zwei `.vh` + zwei `.projrow`, ein `.rail .blk` mit drei `.krow`, ein `.instr`-Band mit `.stats`, `.btn-pri`/`.btn-q`/`.btn-s`. (Bestehende Sektionen unangetastet — /ui ist die Sichtprobe für leer/lang.)

- [ ] **Step 6: Bauen + Tests + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... -race
git add -A && git commit -m "feat(lesesaal): L2-Layout-Klassen (Rückgrat/instr/Baum/Meta-Spalte) als benannte Klassen"
```
Expected: PASS; `git status` zeigt geänderte `app.css`.

**Zustände dieser Fläche:** /ui-Styleguide zeigt leer (—-Zeilen), lang (86-Zeichen-`.fullpath` bricht via `word-break`), mobil 375px (`.cock` einspaltig, `.k` weg) — Sichtprobe im Gate.

---

### Task 2: Go-Helfer — Kurznamen-Dedup + Dokumenttyp-Chip (Single Source, TDD)

**Files:**
- Modify: `internal/adapter/webui/identity.go` (Dedup ergänzen)
- Modify: `internal/adapter/webui/identity_test.go`
- Create: `internal/adapter/webui/doctypechip.go` + `internal/adapter/webui/doctypechip_test.go`

**Interfaces:**
- Produces: `webui.DisplayNames(names []string) map[string]string` — Kurzname pro Name; bei Kollision der Kurznamen im übergebenen (sichtbaren) Kontext **genau ein Elternsegment davor** (`gitlab / group`, `gitlab / project`, `oopii / base-infra`) (Spec §5.5, L1-Deferred #3). Deterministisch, owner-neutral (nur Strings).
- Produces: `webui.DocTypeChipClass(t domain.DocumentType) string` (→ `"tc-b"|"tc-v"|"tc-t"|"tc-o"|"tc-g"|"tc-r"`) + `webui.DocTypeLabel(t domain.DocumentType) string` (dt. Anzeigename) — Spec §7.1 feste semantische Zuordnung. Ersetzt `DocKindStyle`-Glyphen auf L2-Flächen (Cockpit-Wissen-Liste); `dockindstyle.go` selbst bleibt (Wissen-Regale = L3).
- Tasks 3–6 konsumieren exakt diese Namen.

- [ ] **Step 1: Failing Test — Dedup** — in `identity_test.go` ergänzen:
```go
func TestDisplayNames_DedupOnCollision(t *testing.T) {
	in := []string{
		"gitlab.com/dataalliance/infra/common/tf-modules/gitlab/group",
		"gitlab.com/dataalliance/infra/common/tf-modules/gitlab/project",
		"gitlab.com/dataalliance/products/oopii/infra/base-infra",
		"github.com/serverkraken/flow", // unique short → no parent segment
	}
	got := DisplayNames(in)
	want := map[string]string{
		in[0]: "gitlab / group",
		in[1]: "gitlab / project",
		in[2]: "base-infra",          // "base-infra" is unique here → no dedup
		in[3]: "flow",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("DisplayNames[%q] = %q, want %q", k, got[k], v)
		}
	}
}
```
(Hinweis: `base-infra` ist im Beispiel eindeutig → kein Präfix. Wenn ein zweites `base-infra` in der Liste stünde, ergäbe die Regel `oopii / base-infra`. Der Test deckt beide Zweige ab — ggf. einen kollidierenden Fall ergänzen.)

- [ ] **Step 2: Failing Test — Doc-Typ-Chip** — `doctypechip_test.go`:
```go
package webui

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestDocTypeChipClass(t *testing.T) {
	cases := map[domain.DocumentType]string{
		domain.DocProject:       "tc-b",
		domain.DocPlan:          "tc-v",
		domain.DocSpec:          "tc-t",
		domain.DocMemory:        "tc-o",
		domain.DocDaily:         "tc-g",
		domain.DocFree:          "tc-g",
		domain.DocActiveContext: "tc-v", // Spec §7.1 context → violett
	}
	for in, want := range cases {
		if got := DocTypeChipClass(in); got != want {
			t.Errorf("DocTypeChipClass(%v) = %q, want %q", in, got, want)
		}
	}
	if DocTypeLabel(domain.DocPlan) == "" {
		t.Fatal("DocTypeLabel must not be empty")
	}
}
```
(Die exakten `domain.Doc*`-Konstanten vor dem Tippen per `rg "Doc[A-Z].* DocumentType|DocumentType =" internal/domain/` verifizieren — Bestand gewinnt; Spec §7.1: project/notiz→blau, plan→violett, spec/notiz→petrol, memory→bernstein, daily/free→grün, context→violett, Reserve→koralle.)

- [ ] **Step 3: Laufen lassen** — Expected: FAIL (undefined).

- [ ] **Step 4: `DisplayNames` in `identity.go` implementieren**
```go
// DisplayNames liefert für jeden Namen seinen Kurznamen (ShortName). Kollidieren
// zwei Kurznamen im übergebenen (sichtbaren) Kontext, bekommt jeder betroffene
// Name genau ein Elternsegment davor ("gitlab / group"). Spec §5.5 — eine Quelle
// für Projekte, Cockpit, Palette, Pills.
func DisplayNames(names []string) map[string]string {
	short := make(map[string]string, len(names))
	count := map[string]int{}
	for _, n := range names {
		s := ShortName(n)
		short[n] = s
		count[s]++
	}
	out := make(map[string]string, len(names))
	for _, n := range names {
		s := short[n]
		if count[s] > 1 {
			out[n] = parentSlashLeaf(n)
		} else {
			out[n] = s
		}
	}
	return out
}

// parentSlashLeaf gibt "<vorletztes Segment> / <letztes Segment>" zurück
// (nur ein Segment mehr), Fallback = ShortName wenn kein Elternsegment existiert.
func parentSlashLeaf(name string) string {
	name = strings.TrimRight(strings.TrimSpace(name), "/")
	i := strings.LastIndex(name, "/")
	if i < 0 {
		return name
	}
	leaf := name[i+1:]
	rest := name[:i]
	j := strings.LastIndex(rest, "/")
	parent := rest
	if j >= 0 {
		parent = rest[j+1:]
	}
	return parent + " / " + leaf
}
```

- [ ] **Step 5: `doctypechip.go` implementieren** — `DocTypeChipClass` + `DocTypeLabel` als `switch` über die `domain.Doc*`-Konstanten (Spec §7.1). Labels aus dem Bestand ziehen (`DocKindStyle(t).Label` liefert dt. Namen — wiederverwenden, aber ohne Glyph). Beispiel-Skelett:
```go
package webui

import "github.com/serverkraken/flow/internal/domain"

// DocTypeChipClass mappt einen Dokumenttyp auf seine feste .tc-*-Chip-Klasse
// (Spec §7.1). Semantisch, überall gleich — NIE pro Projekt.
func DocTypeChipClass(t domain.DocumentType) string {
	switch t {
	case domain.DocPlan, domain.DocActiveContext: // context → violett (Spec §7.1)
		return "tc-v"
	case domain.DocSpec:
		return "tc-t"
	case domain.DocMemory, domain.DocInstruction, domain.DocSkill, domain.DocAgent:
		return "tc-o"
	case domain.DocDaily, domain.DocFree:
		return "tc-g"
	default: // DocProject/notiz + Rest → blau
		return "tc-b"
	}
}

// DocTypeLabel ist der dt. Anzeigename (ohne Glyph) — wiederverwendet DocKindStyle.
func DocTypeLabel(t domain.DocumentType) string { return DocKindStyle(t).Label }
```
(`domain.DocActiveContext` (="activecontext") existiert → im `switch` als `tc-v` (violett, Spec §7.1 context→violett) ergänzen. `DocAgent` ist DEPRECATED (→ tc-o mit den anderen agent-owned). Konstanten verifiziert: document.go:13–22.)

- [ ] **Step 6: Tests grün + Commit**
```bash
go test ./internal/adapter/webui/ -run 'TestDisplayNames|TestDocType' -race
git add -A && git commit -m "feat(lesesaal): Kurznamen-Dedup + Dokumenttyp-Chip-Mapping als Single Source (Spec §5.5/§7.1)"
```

---

### Task 3: Projekte — der Baum als Inhalt

**Files:**
- Create: `internal/adapter/webui/projects_vm.go` + `internal/adapter/webui/projects_vm_test.go`
- Modify: `internal/adapter/webui/nodes.templ` (`NodesFragment` + Zeilen-Templates neu; Formular-Templates unberührt)
- Modify: `internal/adapter/httpserver/webui_nodes.go` (`handleWebNodesHome`/`handleWebNodesList` bauen das neue VM)
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go`
- Test: `internal/adapter/webui/nodes_render_test.go` (neu oder bestehendes Render-Test-File)

**Interfaces:**
- Consumes: `s.ListNodes.Execute`, `s.ListSessionsRange.Execute` (Subtree-Stunden), `s.ListDocuments.Execute`/`ListDocumentsPage` (Doc-Counts), `webui.DisplayNames`/`Initials`/`AvatarTone`, `SubtreeHourTotals`, `components.Avatar`. Owner-scoped (u.ID) durchgehend.
- Produces: `webui.ProjectsVM` + `webui.BuildProjectsVM(...)`; templ `NodesFragment(ProjectsVM)`. Struktur (Mockup Z.442–570):
  - `ProjectsVM{ Summary string; Engagements []EngagementSection }`
  - `EngagementSection{ N domain.Node; Initials, Tone, HoursStr, RateNote string; Groups []VorhabenGroup; DirectRepos []ProjRow }`
  - `VorhabenGroup{ N domain.Node; Short, CountNote string; Rows []ProjRow }`
  - `ProjRow{ ID, Short, Full, Initials, Tone, KindLabel string; RightV, RightK string; IsVorhaben bool; PathWarn bool }`

**Zustände dieser Fläche (Pflicht benennen):**
- **leer:** kein Knoten → ruhige „Noch keine Projekte"-Zeile (`nodes.empty`), kein Kartenraster.
- **lang:** 86-Zeichen-Remote im `.path` bricht via `word-break:break-all`; Kurzname (`.t`) via `DisplayNames`.
- **mobil 375px:** `.projrow .right .k` entfällt (<620px, Task-1-Klasse), `.eng-h`/`.vh` `flex-wrap`.
- **laufender Timer:** Repo mit laufendem Timer → `RightK = "Timer läuft"` (Mockup) — aus dem Running-Session-NodeID.
- **Fehlerpfad / Schmutzdaten:** `PathWarn` (z. B. Pfad enthält `>` oder Leerzeichen) → dezente `RightK = "Pfad prüfen?"` (Spec §9, Mockup `gitlab.com>/…`).
- **Request-Fehler:** schlägt `ListNodes` fehl → 500 (wie Bestand); scheitern die Neben-Quellen (`ListSessionsRange`/`ListDocuments`/`GetRunningSession`), degradiert der Handler still — Stunden/Docs/Timer-Note fallen auf „—"/leer, die Seite rendert trotzdem (kein 500). Owner-scoped (`u.ID`) bleibt.

- [ ] **Step 1: Failing VM-Test** — `projects_vm_test.go`:
```go
package webui

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func TestBuildProjectsVM_GroupsAndDedup(t *testing.T) {
	eng := domain.Node{ID: "e1", Name: "RTL Extern", Kind: domain.KindEngagement}
	vor := domain.Node{ID: "v1", Name: "tf-modules", Kind: domain.KindVorhaben, ParentID: strptr("e1")}
	r1 := domain.Node{ID: "r1", Name: "gitlab.com/x/tf-modules/gitlab/group", Kind: domain.KindRepo, ParentID: strptr("v1")}
	r2 := domain.Node{ID: "r2", Name: "gitlab.com/x/tf-modules/gitlab/project", Kind: domain.KindRepo, ParentID: strptr("v1")}
	direct := domain.Node{ID: "r3", Name: "gitlab.com/x/products/foolu/product/backstage", Kind: domain.KindRepo, ParentID: strptr("e1")}
	nodes := []domain.Node{eng, vor, r1, r2, direct}
	vm := BuildProjectsVM(nodes, nil, nil, nil, time.Now())
	if len(vm.Engagements) != 1 {
		t.Fatalf("want 1 engagement, got %d", len(vm.Engagements))
	}
	es := vm.Engagements[0]
	if len(es.Groups) != 1 || es.Groups[0].Short != "tf-modules" {
		t.Fatalf("vorhaben group wrong: %+v", es.Groups)
	}
	// dedup within the visible engagement: group/project collide → "gitlab / …"
	got := map[string]string{}
	for _, row := range es.Groups[0].Rows {
		got[row.ID] = row.Short
	}
	if got["r1"] != "gitlab / group" || got["r2"] != "gitlab / project" {
		t.Fatalf("dedup failed: %+v", got)
	}
	if len(es.DirectRepos) != 1 || es.DirectRepos[0].Short != "backstage" {
		t.Fatalf("direct repos wrong: %+v", es.DirectRepos)
	}
}
```
(Test-Helper `ptr(s string) *string` existiert bereits in `node_tree_vm_internal_test.go` (package webui) — den nutzen, NICHT `strptr` neu erfinden.)

- [ ] **Step 2: Laufen lassen** — FAIL (undefined: BuildProjectsVM).

- [ ] **Step 3: `projects_vm.go` implementieren** — `BuildProjectsVM(nodes []domain.Node, sessions []domain.WorkSession, docCounts map[string]int, running *domain.WorkSession, now time.Time) ProjectsVM`:
  - Engagements = alle `KindEngagement` (name-sortiert).
  - Pro Engagement: direkte Kinder gruppieren — `KindVorhaben`-Kinder → je eine `VorhabenGroup` (deren direkte Kinder als `Rows`), `KindRepo`-Kinder → `DirectRepos`.
  - `DisplayNames` **pro Engagement-Sichtbereich** aufrufen (alle Row-Namen dieses Engagements) → Kurznamen mit Dedup.
  - `HoursStr` je Knoten aus `SubtreeHourTotals` (`FmtDurHMExport`), `RightV` = Doc-Count (`docCounts[id]` → „N Docs") oder Stunden, `RightK` = Status-Note (laufender Timer → `nodes.row.timerRunning`; sonst „ruhig"/leer; Schmutzdaten → `nodes.row.pathWarn`).
  - **Vorhaben-unter-Vorhaben (rekursiv erlaubt, `ValidParentKind` node.go:99–105; Mockup Z.474–484 „infra"→„base-infra"/„k8s-infra"):** ein Sub-Vorhaben erscheint als `.projrow.lvl2`-Zeile in der Gruppe seines Eltern-Vorhabens mit `IsVorhaben:true` (dim-Suffix „· Vorhaben" + „unter <parent>"), NICHT als eigener `.vh`-Kopf. Seine tieferen Kinder (Repos 2 Ebenen unter dem Engagement) werden auf der Projekte-Seite bewusst NICHT expandiert (Drill über das Cockpit) — mockup-treu. **Kein Knoten wird verschluckt:** wie `buildNodeTree` cycle/orphan-safe; ein Knoten ohne sichtbaren Pfad zu einem Engagement fällt in einen Fallback (an der Engagement-Ebene sichtbar), nie stumm gedroppt.
  - `Summary` = „N Engagements · M Vorhaben · K Repos · Σ … h" (i18n `Tn`/`Sprintf`).
  - Avatar: `Initials(Short)` + `AvatarTone(fullName)` (Farbe aus vollem Namen, wie L1).
  - **PathWarn**: `strings.ContainsAny(name, "> ")` oder doppelte Schrägstriche → true.
  - Deterministische Sortierung durchweg (name-stable) — cycle/orphan-safe wie `buildNodeTree` (Orphan-Engagement-Fallback nicht vergessen: Knoten ohne sichtbaren Parent an Engagement-Ebene 0).

- [ ] **Step 4: VM-Test grün** — PASS.

- [ ] **Step 5: `nodes.templ` `NodesFragment` neu** — rendert nach Mockup Z.442–570: `pagehead` (eyebrow „Projekte" + h1 `nodes.title` + `.sub` = `vm.Summary` + `.btn-q` „Neuer Knoten"), dann pro `EngagementSection` eine `.eng` mit `.eng-h` (`@components.Avatar(es.Initials, es.Tone, "av-36")` + h2 `ShortName` + neutraler `.typechip` „engagement" + `.right` v/k), dann pro `VorhabenGroup` ein `.vh` (`.t` = Short, neutraler `.typechip` „vorhaben", `.c` = CountNote) gefolgt von seinen `.projrow.lvl2`-Zeilen, dann `.vh` „Direkt am Engagement" + `DirectRepos`. Repo-Zeile:
```
<a class="projrow lvl2" href={ templ.SafeURL("/nodes/" + row.ID) }>
  @components.Avatar(row.Initials, row.Tone, "av-28")
  <div class="grow"><div class="t">{ row.Short }</div><div class="path">{ row.Full }</div></div>
  <div class="right"><div class="v">{ row.RightV }</div><div class="k">{ row.RightK }</div></div>
</a>
```
- **Glyph-frei:** `nodeKindBadge` NICHT verwenden — Kind = neutraler `.typechip` (nur Text). `nodeGlyphSwatch` entfällt hier. Die alten `nodeKindBadge`/`nodeGlyphSwatch`/`nodeTreeRow`/`nodeFilterChip`-Templates bleiben nur, falls sie außerhalb noch Nutzer haben (`rg` prüfen); sonst mit entfernen. **Formular-Templates (`NodeForm` … `nodeIconRadio`) NICHT anfassen** (Node-Edit-Seite = Task 7).
- Der SSE-Wrapper `nodesOuter #content` (hx-get `/ui/nodes/list`, Trigger `sse:node.*`) bleibt — nur der Inhalt wird das neue Fragment.
- **Filter-Chips (aktiv/archiviert/alle):** siehe Offene Entscheidung #5 — Empfehlung: als ruhige `.typechip`-Zeile behalten (Funktion), unter dem Pagehead. Wenn Soenne sie streicht, Chips + `nodeFilterChip` + `d.Status` entfernen.

- [ ] **Step 6: Handler + Owner-Scope-Negativtest** — in `webui_nodes.go` bauen `handleWebNodesHome`/`handleWebNodesList` das `ProjectsVM`: `all := s.ListNodes.Execute(ctx, u.ID)`, Sessions für Stunden (`ListSessionsRange` seit „2000" bis morgen — Muster aus `webui_cockpit.go` worktime), Doc-Counts (`ListDocuments` je Knoten ODER ein Gesamt-List gruppiert nach `NodeID`), Running-Session (`GetRunningSession`). Feld-/Helper-Namen (`s.ListSessionsRange`, `s.GetRunningSession`, `s.ListDocuments`, Handler-Namen) vor dem Tippen per `rg` an `server.go` abgleichen — **Bestand gewinnt**. **KEIN neuer Server-Feld/Konstruktor-Parameter** — alle Usecases existieren; also kein `main.go`-Change. **Owner-Scope-Negativtest (Pflicht, AGENTS.md-Grundsatz):** httpserver-Test mit Fake-Store, der Nodes zweier Owner hält — die `/nodes`-Seite von User A darf KEINEN Node von User B enthalten (Cross-Tenant-Leak = Critical).

- [ ] **Step 7: i18n** — beide Kataloge:
```go
"nodes.title":          "Alle Knoten",          // en: "All nodes"
"nodes.new":            "Neuer Knoten",          // en: "New node"
"nodes.summary":        "%d Engagements · %d Vorhaben · %d Repos", // via Sprintf/Tn
"nodes.group.direct":   "Direkt am Engagement",  // en: "Directly on the engagement"
"nodes.row.timerRunning": "Timer läuft",         // en: "Timer running"
"nodes.row.quiet":      "ruhig",                 // en: "quiet"
"nodes.row.pathWarn":   "Pfad prüfen?",          // en: "Check path?"
```
(Bestehende `nodes.subtitle`/`nodes.filter*`/`nodes.empty` prüfen und nur ergänzen/anpassen, nicht duplizieren. `nodes.empty`/`emptyHint` wiederverwenden.)

- [ ] **Step 8: Render-Test** — `nodes_render_test.go`: rendert `NodesFragment` mit einer 2-Engagement-Fixture und asserted: `>backstage<`, ein `.fullpath`/`.path` mit dem 86-Zeichen-Pfad ungekürzt, `class="typechip"` (Kind neutral, kein `◆`/`▲`/`●`), `av-28`/`av-36`, `Direkt am Engagement`. Ein Assert: `!strings.Contains(out, "◆")` (Glyphen tot).

- [ ] **Step 9: Bauen + Tests + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -race 2>&1 | tail -20
git add -A && git commit -m "feat(lesesaal): Projekte-Seite als Baum-Inhalt — Engagement/Vorhaben-Köpfe, Repo-Zeilen, Kurzname+Pfad, glyphfrei"
```
Expected: PASS; Tests, die die alte `nodeTreeRow`/Gradient-CTA asserten, auf die neue Realität anpassen (Assertions ändern, nie Verhalten wegtesten).

---

### Task 4: Cockpit — Pfad-Rückgrat + instr-Band (`#cockpit-head`)

**Files:**
- Create: `internal/adapter/webui/cockpit_head.templ`
- Modify: `internal/adapter/webui/cockpit_vm.go` (Head-Builder-Helfer; `NodeCockpit`-Felder ergänzen falls nötig)
- Create: `internal/adapter/webui/static/js/copypath.js` (Copy-Affordanz, kein Popup)
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go`
- Test: `internal/adapter/webui/cockpit_head_render_test.go`

**Interfaces:**
- Consumes: `NodeCockpit` (Bestand — `N`, `Ancestors`, `Timer`, `TodayHere`, `CountsWork`, `Rollup`, `Rate`, `LogoShape`), `webui.ShortName`, `Initials`, `AvatarTone`, `components.Avatar`, `NodeTimer`-States.
- Produces: `templ CockpitHead(d NodeCockpit)` = **nur das Pfad-Rückgrat/Spine** (Mockup Z.573–590), das im Mockup VOLLE Breite über dem `.cock`-Grid liegt. Das **instr-Band** (Mockup Z.592–597) liegt im Mockup INNERHALB `.cock > .main` — deshalb ist `cockpitInstr(d)` eine **eigene Komponente**, die Task 6 als erstes Element von `CockpitMain` rendert (NICHT Teil von `CockpitHead`). Task 4 liefert `CockpitHead` (Spine) + die exportierbare `cockpitInstr`-Komponente + Helfer, beides render-getestet; die Seiten-Verdrahtung macht Task 7 (die alte `CockpitRail` läuft bis dahin weiter).
- Produces: `webui.SpineCrumbs(d) []Crumb` (Vorfahren OHNE self, root→leaf, für die `.up`-Kette) — abgeleitet aus `nodeCrumbs`, aber ohne das letzte (self) Segment; self steht als `<h1>` groß.

- [ ] **Step 1: Failing Render-Test** — `cockpit_head_render_test.go` (package webui, `renderToBuf` + `seededCockpit()`):
```go
func TestCockpitHead_SpineShowsShortNameAndFullPath(t *testing.T) {
	d := seededCockpit()
	d.N.Name = "gitlab.com/dataalliance/products/foolu/product/backstage"
	d.N.Kind = domain.KindRepo
	out := renderToBuf(t, testCtx(t), CockpitHead(d))
	if !strings.Contains(out, "<h1") || !strings.Contains(out, ">backstage<") {
		t.Fatalf("spine must show ShortName as h1:\n%s", out)
	}
	if !strings.Contains(out, "class=\"fullpath\"") || !strings.Contains(out, d.N.Name) {
		t.Fatalf("spine must show full path in .fullpath (no truncation):\n%s", out)
	}
	if strings.Contains(out, "◆") || strings.Contains(out, "▲") {
		t.Fatalf("no kind glyphs in Lesesaal spine")
	}
	if !strings.Contains(out, "av-96") {
		t.Fatalf("spine misses 96er identity:\n%s", out)
	}
	// instr-Band gehört NICHT in den Spine (liegt in CockpitMain):
	if strings.Contains(out, "class=\"instr\"") {
		t.Fatalf("instr band must live in CockpitMain, not the spine")
	}
}

// cockpitInstr wird separat getestet (es ist eine eigene Komponente in CockpitMain):
func TestCockpitInstr_RunningShowsStop(t *testing.T) {
	d := seededCockpit()
	d.N.Kind = domain.KindRepo
	d.Timer = CockpitTimer{State: TimerHere, RunningBase: 754}
	out := renderToBuf(t, testCtx(t), cockpitInstr(d))
	for _, want := range []string{"class=\"instr\"", "class=\"stats\"", "/stop", "data-timer", `hx-target="#cockpit-main"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("running instr misses %q:\n%s", want, out)
		}
	}
}
```
(**Test-Kontext, verifiziert:** die webui-Render-Tests bauen `ctx := i18n.WithLocale(context.Background(), i18n.DE)` (siehe `activity_row_test.go:51`) für Tests mit dt. Textassertion, sonst `context.Background()`. `testCtx` existiert NUR im `components`-Paket — in allen webui-Snippets (T4–T7) steht `testCtx(t)` stellvertretend für diesen verifizierten webui-`ctx`.)

- [ ] **Step 2: Laufen lassen** — FAIL.

- [ ] **Step 3: `cockpit_head.templ`** — Spine + instr-Band. Kernstruktur (Mockup Z.573–598):
```
templ CockpitHead(d NodeCockpit) {
  <div class="spine">
    <div class="up">
      for _, c := range SpineCrumbs(d) {
        <a href={ templ.SafeURL(c.Href) }>{ c.Label }</a><span class="sep">/</span>
      }
    </div>
    <div class="spine-main">
      @cockpitIdentity(d)   // 96er Logo/Initialen (siehe Step 4)
      <h1>{ ShortName(d.N.Name) }</h1>
    </div>
    <div class="spine-meta">
      <span class="k">{ components.T(ctx, NodeKindStyle(d.N.Kind).LabelKey) }</span>
      <span>{ cockpitStatusWord(d.N.Status) }</span><span class="dotsep">·</span>
      <a class="targetlink" href={ templ.SafeURL("/nodes/" + d.N.ID + "/edit") } hx-boost="false">{ components.T(ctx, "common.edit") }</a><span class="dotsep">·</span>
      <a class="targetlink" href={ templ.SafeURL("/nodes/" + d.N.ID + "/edit") } hx-boost="false" title={ components.T(ctx, "cockpit.logo.hint") }>{ components.T(ctx, "cockpit.logo.add") }</a>
    </div>
    <div class="fullpath">{ d.N.Name } <button type="button" data-copy={ d.N.Name } title={ components.T(ctx, "cockpit.copyPath") } aria-label={ components.T(ctx, "cockpit.copyPath") }>⧉</button></div>
  </div>
}
// cockpitInstr wird NICHT hier gerendert — es lebt als erstes Element von
// CockpitMain (Task 6), Mockup-DOM: .cock > .main > .instr.
```
`cockpitStatusWord(s domain.NodeStatus) string` als kleinen Helfer in `cockpit_vm.go` NEU anlegen (existiert nicht — verifiziert): mappt Status → i18n-Key `node.status.active/paused/archived` und resolved via `components.T` NICHT im VM (VM bleibt domain-frei) — stattdessen gibt `cockpitStatusWord` den **Key** zurück und das templ ruft `components.T(ctx, key)`. Alternativ die bestehende `StatusBadge(s)` nutzen und NUR ihren ersten Rückgabewert (`label`, dt.) verwenden, den zweiten (Nicht-Token-Klassen) verwerfen. Empfehlung: i18n-Key-Weg (dt+en-Parität).

- `SpineCrumbs(d)` in `cockpit_vm.go`: `nodeCrumbs`-Logik, aber nur die Vorfahren OHNE self (self = h1). `NodeKindStyle(...).LabelKey` liefert die i18n-Kind-Bezeichnung (Glyph wird NICHT gerendert — nur `.LabelKey`). `cockpitStatusWord(status)` = i18n-Wort (aktiv/pausiert/archiviert) — **nicht** `StatusBadge` (dessen amber/slate/emerald sind Nicht-Token-Farben, Spec §7). „Logo hinzufügen" verlinkt vorerst auf die Edit-Seite (Upload liegt dort; eigener Inline-Upload = späterer Feinschliff).

- [ ] **Step 4: `cockpitIdentity(d)` — 96er Logo/Initialen** (L1-Deferred #2, `.av-96`/`.av-64`/`.ava-logo`):
```
templ cockpitIdentity(d NodeCockpit) {
  if d.N.LogoRef != "" {
    <span class="ava ava-logo av-96" aria-hidden="true"><img src={ "/nodes/" + d.N.ID + "/logo?v=" + d.N.LogoRef } alt="" class="h-full w-full rounded-[inherit] object-contain p-2"/></span>
  } else {
    @components.Avatar(Initials(ShortName(d.N.Name)), AvatarTone(d.N.Name), "av-96")
  }
}
```
Kein Hex-Clip mehr (Kristall tot). Mobil 64 kommt über die Task-1-Responsive-Regel (`.av-96`→64 <620px). Für Agenten-Knoten (falls je vorhanden) gilt der `.ava-agent`-Fall analog — im Cockpit sind Knoten aber Projekte, kein Agent-Avatar nötig.

- [ ] **Step 5: `cockpitInstr(d)` — instr-Band** (Mockup Z.592–597), Timer-Zustände aus `d.Timer.State`. **Ziel `#cockpit-main`** (das Band lebt in `CockpitMain`); Start/Stop/Switch swappen `#cockpit-main`, die SSE-Events triggern zusätzlich `#cockpit-rail` (Ketten-Stunden) und die Topbar-Pill:
```
templ cockpitInstr(d NodeCockpit) {
  <div class="instr">
    switch d.Timer.State {
      case TimerHere:
        <form hx-post={ "/nodes/" + d.N.ID + "/stop" } hx-target="#cockpit-main" hx-swap="innerHTML">
          <button type="submit" class="btn btn-pri"><span aria-hidden="true">■</span> { components.T(ctx, "cockpit.timer.stop") }</button>
        </form>
        <span class="font-mono tnum text-[13px] font-semibold" data-timer data-timer-fmt="clock" data-base={ secStr(d.Timer.RunningBase) } role="timer">{ fmtClockHMS(int64(d.Timer.RunningBase)) }</span>
      case TimerIdle:
        <form hx-post={ "/nodes/" + d.N.ID + "/start" } hx-target="#cockpit-main" hx-swap="innerHTML">
          <button type="submit" class="btn btn-pri"><span aria-hidden="true">▶</span> { components.T(ctx, "cockpit.timer.start") }</button>
        </form>
      case TimerOtherBound:
        <form hx-post={ "/nodes/" + d.N.ID + "/switch" } hx-target="#cockpit-main" hx-swap="innerHTML">
          <button type="submit" class="btn btn-q">{ components.T(ctx, "cockpit.timer.switch") }</button>
        </form>
        <span class="text-[13px] text-meta">{ components.T(ctx, "cockpit.timer.runningOn") } <a class="targetlink" href={ templ.SafeURL("/nodes/" + d.Timer.OtherID) } title={ d.Timer.OtherName }>{ ShortName(d.Timer.OtherName) }</a></span>
      case TimerUnbound:
        <a href="/" class="text-[13px] text-meta hover:text-ink">{ components.T(ctx, "cockpit.timer.unbound") } →</a>
      case TimerNotBookable:
        <span class="text-[13px] text-faint">{ components.T(ctx, "cockpit.timer.notBookable") }</span>
    }
    if domain.IsBookable(d.N.Kind) {
      <button type="button" class="btn btn-q" data-dialog-open="session-dialog">{ components.T(ctx, "cockpit.worktime.add") }</button>
    }
    <div class="stats">@cockpitStatsLine(d)</div>
  </div>
}
```
- `cockpitStatsLine(d)` (Mockup „hier heute 2:41 · Repo gesamt 2:41 · RTL Extern 304:46 h"): baut aus `d.TodayHere`, `d.Rollup.Total` (`FmtDurHMExport`) und dem Wurzel-Engagement-Namen (`d.Ancestors[len-1]`) + dessen Rollup — die Ketten-Zahl. Reine String-Komposition; i18n-Labels `cockpit.stats.hereToday`/`.repoTotal`/`.chainTotal`. **Nullen ohne Bühne** (Spec §4): 0-Werte als „—", nie „0:00".
- `secStr`/`fmtClockHMS` sind Bestand (cockpit.templ nutzt sie) — wiederverwenden.
- **SSE-Ziel `#cockpit-main`:** Start/Stop/Switch swappen das Main-Fragment (Task 7 verdrahtet die Handler auf `CockpitMain`; Instr-Band ist dessen erstes Element). Nachbuchen öffnet den einen `SessionDialog` (in Task 7 gemountet, Target `#cockpit-main`). `#cockpit-head` (Spine) reloadet nur auf `node.updated`/`node.moved` (Identität/Pfad/Status) — nicht auf `session.*`.

- [ ] **Step 6: `copypath.js`** — winzig, kein Framework, **kein Popup** (`verify-no-popups`): `navigator.clipboard.writeText`, kurzes visuelles Feedback über `data-copied`-Attribut/Titel-Wechsel, KEIN `alert`.
```js
// Kopiert data-copy in die Zwischenablage; visuelles Feedback ohne Popup.
document.addEventListener('click', function (e) {
  var b = e.target.closest('[data-copy]');
  if (!b) return;
  e.preventDefault();
  if (navigator.clipboard) navigator.clipboard.writeText(b.getAttribute('data-copy'));
  var old = b.textContent; b.textContent = '✓';
  setTimeout(function () { b.textContent = old; }, 1200);
});
```
(In Task 7 per `<script src="/static/js/copypath.js" defer>` auf der Cockpit-Seite einhängen.)

- [ ] **Step 7: i18n** — beide Kataloge:
```go
"cockpit.copyPath":      "Pfad kopieren",         // en: "Copy path"
"cockpit.logo.add":      "Logo hinzufügen",        // en: "Add logo"
"cockpit.logo.hint":     "PNG/JPG/WebP — ersetzt den Initialen-Avatar überall", // en: "…replaces the initials avatar everywhere"
"cockpit.stats.hereToday": "hier heute",           // en: "here today"
"cockpit.stats.repoTotal": "Repo gesamt",          // en: "repo total"
"cockpit.stats.chainTotal": "Kette",               // en: "chain"
```

- [ ] **Step 8: Test grün + Bauen + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... -race
git add -A && git commit -m "feat(lesesaal): Cockpit-Pfad-Rückgrat + instr-Band — 96er-Identität, Kurzname groß, voller Pfad, glyphfrei"
```

**Zustände:** leer (Repo ohne Zeit → Stats „—"), lang (86-Zeichen-`.fullpath` bricht), mobil 64er-Avatar + `.instr` `.stats` volle Breite, laufender Timer (Stop + tickende Uhr), Fehlerpfad (nicht buchbar → „nicht buchbar", unbound → Home-Link).

---

### Task 5: Cockpit-Meta-Spalte — Kette + Bindings (`#cockpit-rail`)

**Files:**
- Create: `internal/adapter/webui/cockpit_rail.templ`
- Modify: `internal/adapter/webui/cockpit_vm.go` (Kette-Builder)
- Test: `internal/adapter/webui/cockpit_rail_render_test.go`

**Interfaces:**
- Consumes: `d.Ancestors`, `d.Rollup`, `d.Rate`, `d.Bindings`, `webui.ShortName`, `FmtDurHMExport`, per-Knoten-Rollup (Ketten-Zahlen). `bindingTarget(b)` (Bestand in cockpit.templ — nach `cockpit_rail.templ` verschieben oder als exportierten Helfer belassen; `rg` prüfen).
- Produces: `templ CockpitRailBlocks(d NodeCockpit)` = zwei `.rail .blk` (Kette · Bindings) (Mockup Z.634–668, OHNE den „Kontext für Agenten"-Block — der ist **L5-Scope**, siehe Offene Entscheidung #6). In Task 7 in `#cockpit-rail` gemountet, Fragment `GET /nodes/{id}/rail`.
- Produces: `webui.ChainRows(d) []KetteRow{ Label, HoursStr string; Here bool; Href string }` — this (hier) → Vorfahren → geerbter Satz. Reine Strings.

**Zustände:** leer (Repo ohne Bindings → „Keine Bindings", Kette zeigt trotzdem Vorfahren + „—" bei 0h), lang (Binding-Pfad/`.krow .n` truncatet mit `title`), mobil (Rail stackt unter die Bühne — Task-1 `.cock` einspaltig), Fehlerpfad (Bind-Fehler = Task 7 Handler-Repoint), laufender Timer (Ketten-Stunden aktualisieren via SSE-Reload).

- [ ] **Step 1: Failing Render-Test** — `cockpit_rail_render_test.go`:
```go
func TestCockpitRail_ChainAndBindings(t *testing.T) {
	d := seededCockpit()
	d.N.Kind = domain.KindRepo
	d.Ancestors = []domain.Node{
		{ID: "n1", Name: "backstage", Kind: domain.KindRepo},
		{ID: "e1", Name: "RTL Extern", Kind: domain.KindEngagement},
	}
	d.ChainStats = map[string]domain.NodeRollup{
		"n1": {Total: 2*time.Hour + 41*time.Minute},
		"e1": {Total: 304*time.Hour + 46*time.Minute},
	}
	d.Rate = "87,50 €/h"
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, CockpitRailBlocks(d))
	// Kette-Block: echte Inhalte — beide Ketten-Ebenen + geerbter Satz (KEINE tote Assertion):
	for _, want := range []string{`class="blk"`, "Kette", `class="krow"`, "RTL Extern", "87,50", "304:46"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rail Kette misses %q:\n%s", want, out)
		}
	}
	// Bindings-Block vorhanden (leer → Empty-Label „Keine Bindings"):
	if !strings.Contains(out, "Bindings") {
		t.Fatalf("rail must show Bindings block:\n%s", out)
	}
	// Kontext-Block ist L5 → darf NICHT hier sein:
	if strings.Contains(out, "Kontext für Agenten") || strings.Contains(out, "Kuratieren") {
		t.Fatalf("Kontext block is L5, must not appear in L2 rail")
	}
}
```
(Imports: `context`, `time`, `.../internal/i18n`. „304:46"-Assertion setzt voraus, dass `ChainRows`/`FmtDurHMExport` das Format liefert — sonst auf das real gerenderte Format anpassen, aber eine ECHTE Inhalts-Assertion behalten.)

- [ ] **Step 2: Laufen lassen** — FAIL.

- [ ] **Step 3: `ChainRows(d)` + neues Feld `NodeCockpit.ChainStats`** — **Interface-Lücke schließen (Codex-Finding):** `NodeCockpit` trägt bisher nur `Rollup` (der eigene Subtree), aber `BuildChain`/`ChainRows` braucht die Rollups JE Ketten-Ebene. Deshalb ein neues Feld `ChainStats map[string]domain.NodeRollup` zu `NodeCockpit` ergänzen (Task 5), das der Handler (Task 7) je Vorfahr via `s.Stats.NodeStats` füllt. **Bestand wiederverwenden:** `BuildChain(node domain.Node, ancestors []domain.Node, statsByID map[string]domain.NodeRollup, ownerTotal time.Duration) []ChainRow` (`cockpit_uebersicht_vm.go:183`) liefert exakt „this → Vorfahren → Σ" (`ChainRow{Label,Kind,DurStr,Pct,This,Sum}`). `ChainRows(d)` ist ein dünner Adapter: `BuildChain(d.N, d.Ancestors, d.ChainStats, ownerTotal)` → Label „<Kurzname> (hier)" für `This`, Link `/nodes/{id}`, letzte Zeile geerbter Satz aus `d.Rate`, Nullen → „—". So bleibt `cockpit_uebersicht_vm.go` als Builder-Bibliothek erhalten (nur die Card-`.templ` stirbt in Task 7). (`ownerTotal` = owner-weiter Σ; Quelle im Handler klären — sonst `d.Rollup.Total` des Wurzel-Engagements.)

- [ ] **Step 4: `cockpit_rail.templ`** — `CockpitRailBlocks(d)`:
```
templ CockpitRailBlocks(d NodeCockpit) {
  <div class="blk">
    <span class="eyebrow">{ components.T(ctx, "cockpit.rail.chain") }</span>
    for _, row := range ChainRows(d) {
      <div class="krow">
        if row.Href != "" {
          <a class="n" href={ templ.SafeURL(row.Href) } title={ row.Label }>{ row.Label }</a>
        } else {
          <span class="n" title={ row.Label }>{ row.Label }</span>
        }
        <span class="v">{ row.HoursStr }</span>
      </div>
    }
  </div>
  <div class="blk">
    <span class="eyebrow">{ components.T(ctx, "cockpit.bindings.title") }</span>
    if len(d.Bindings) == 0 {
      <p class="text-[12.5px] text-faint">{ components.T(ctx, "cockpit.bindings.empty") }</p>
    } else {
      for _, b := range d.Bindings {
        <div class="krow">
          <span class="n font-mono text-[12px]" title={ bindingTarget(b) }>{ bindingTarget(b) }</span>
          <form hx-post={ "/nodes/" + d.N.ID + "/bindings/delete" } hx-target="#cockpit-rail" hx-swap="innerHTML">
            <input type="hidden" name="kind" value={ string(b.Kind) }/>
            <input type="hidden" name="slug" value={ b.RemoteSlug }/>
            <input type="hidden" name="machine" value={ b.MachineID }/>
            <input type="hidden" name="path" value={ b.Path }/>
            <button class="text-[11px] text-faint hover:text-warn" aria-label={ components.T(ctx, "cockpit.bindings.delete") }>✕</button>
          </form>
        </div>
      }
    }
    if d.N.Kind == domain.KindRepo {
      <form hx-post={ "/nodes/" + d.N.ID + "/bindings" } hx-target="#cockpit-rail" hx-swap="innerHTML" class="mt-2 flex items-center gap-2">
        <input name="remoteSlug" placeholder={ components.T(ctx, "cockpit.bindings.remotePlaceholder") } class="min-w-0 flex-1 rounded-lg border border-hair2 bg-surface px-2 py-1.5 text-[12px] font-mono"/>
        <button class="btn btn-q btn-s" type="submit">+</button>
      </form>
    }
    if d.PanelErr != "" {
      <p class="mt-2 text-[12px] text-warn" role="alert">{ d.PanelErr }</p>
    }
  </div>
}
```
- **SSE-Ziel `#cockpit-rail`:** Bind/Unbind re-rendern das Rail-Fragment (Task 7 repointet die Handler `handleWebNodeBindRemote`/`handleWebNodeUnbind` von `renderNodePanel` auf `renderNodeRail` mit Target `#cockpit-rail`). Beide Handler emittieren bereits `EventNodeUpdated` — der `#cockpit-rail`-Container triggert auf `sse:node.updated` (Task 7).
- i18n: `cockpit.rail.chain` neu (beide Kataloge): „Kette" / „Chain". Bindings-Keys existieren.

- [ ] **Step 5: Test grün + Bauen + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... -race
git add -A && git commit -m "feat(lesesaal): Cockpit-Meta-Spalte — Kette + Bindings als Zwei-Flächen-Panels (Kontext folgt L5)"
```

---

### Task 6: Cockpit-Inhalt — Enthält · Wissen · Buchungen · Puls (`#cockpit-main`)

**Files:**
- Create: `internal/adapter/webui/cockpit_main.templ`
- Modify: `internal/adapter/webui/cockpit_vm.go` (Wissen-Zeilen-VM mit Typ-Chip + Lesezeit)
- Test: `internal/adapter/webui/cockpit_main_render_test.go`
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go`

**Interfaces:**
- Consumes: `d.Children` (Enthält), `d.WissenRows` (Wissen), `d.SessionRows` (Buchungen), Puls. **VM-Builder wiederverwenden** (aus `cockpit_uebersicht_vm.go`, bleiben nach Task 7 erhalten): `FilterPulse(entries, subtreeIDs) []ActivityEntry` + `BuildActivityRows(entries, names, kinds, now) []ActivityRowVM` → Puls-Daten; `TopDocs(docs, subtreeIDs, now) ([]UebersichtDoc,int)` liefert Titel+Meta für die Wissen-Zeilen. `webui.DisplayNames`, `DocTypeChipClass`, `DocTypeLabel`, `components.Avatar`.
- **Puls-Zeile NEU in Lesesaal-Sprache** (NICHT `activityFeedRow` wiederverwenden): `activity_row.templ:11` ist Kristall-gefärbt (`text-blue`/`text-muted`/`border-purple/40`/`text-[.88rem]`/`ActorGlyph`) und wird von `home.templ` geteilt — Home ist L4, bleibt vorerst. Der Cockpit-Puls rendert eine eigene `.row`/`.projrow`-Zeile aus `[]ActivityRowVM`: `@components.Avatar`/`AgentAvatar` (Mockup nutzt `.ava s28` für Akteure, kein Glyph) + `.who`/`.agentname` + Verb + `.targetlink`-Ziel-Pill (glyphfrei) + Zeit. So bleibt Home unberührt und der Puls ist mockup-treu.
- Produces: `templ CockpitMain(d NodeCockpit)` — gestapelte Sektionen (Mockup Z.599–627 Repo-Fall; Enthält-Sektion für Nicht-Leaf ergänzt aus Spec §5.3 „Kinder als Listen im Inhalt"). In Task 7 in `#cockpit-main` gemountet, Fragment `GET /nodes/{id}/main`.
- Produces: `webui.WissenRow{ ID, Title, ChipClass, ChipLabel, Meta, ReadTime string }` + `BuildWissenRows(docs []domain.Document, ...) []WissenRow` (Lesezeit = Wortzahl/220, Spec §16.9; Meta = „Akteur · Zeit · Pfad").

**Sektionen (Reihenfolge):**
0. **instr-Band** (`@cockpitInstr(d)` aus Task 4) — ERSTES Element von `CockpitMain`, damit das Band im Mockup-DOM `.cock > .main > .instr` liegt (Codex-Finding: nicht in den Spine mergen). Enthält Start/Stop/Nachbuchen + Mono-Stats.
1. **Enthält** (nur Nicht-Leaf / hat Kinder): Kinder als `.projrow` (Avatar + Kurzname + Pfad/Note + Rollup-Stunden) + „Unterknoten hinzufügen" (`cockpit.struktur.add`, verlinkt `/nodes/new?parent={id}`). Ersetzt Struktur-Tab-Kinderliste **und** die alte Comp-Card. Leer → „Keine Unterknoten".
2. **Wissen** (Mockup „Wissen zu diesem Repo"): `.projrow`-artige Zeilen mit `.typechip`+`DocTypeChipClass` + Titel + Meta + Lesezeit rechts; Scope-Toggle (`.seg`/Segmented — nur Nicht-Repo) + „Neues Wissen". Leer → „Noch keine Dokumente".
3. **Buchungen** (nur buchbar; siehe Offene Entscheidung #2): Session-Ledger — die bestehenden `d.SessionRows` (Datum · Span · Tag · Dauer, Edit/Delete für abgeschlossene). Lesesaal-Zeilen statt Kristall-Karten. Leer → „Noch keine Buchungen".
4. **Puls** (Mockup Z.611–626): Subtree-gefilterte Activity mit `livechip` „LIVE"; **eigene Lesesaal-Zeile** (`.row` + `Avatar`/`AgentAvatar` + who/agentname + Verb + glyphfreie `.targetlink`-Ziel-Pill + Zeit), gespeist aus `BuildActivityRows`. NICHT `activityFeedRow` (Kristall, für Home/L4). Leer → „Noch nichts passiert".

- [ ] **Step 1: Failing Render-Test** — `cockpit_main_render_test.go`:
```go
func TestCockpitMain_WissenRowHasTypechipAndReadtime(t *testing.T) {
	d := seededCockpit()
	d.N.Kind = domain.KindRepo
	d.WissenRows = []WissenRow{{ID: "d1", Title: "Token-Integration", ChipClass: "tc-b", ChipLabel: "Projekt", Meta: "Claude · heute", ReadTime: "18 min"}}
	out := renderToBuf(t, testCtx(t), CockpitMain(d))
	for _, want := range []string{"typechip", "tc-b", "Token-Integration", "18 min", "livechip"} {
		if !strings.Contains(out, want) {
			t.Fatalf("cockpit main misses %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "◆") || strings.Contains(out, "pill-tab") {
		t.Fatalf("no glyphs / no tab strip in Lesesaal cockpit main")
	}
}

func TestCockpitMain_NonLeafShowsEnthaelt(t *testing.T) {
	d := seededCockpit()
	d.N.Kind = domain.KindVorhaben
	d.Children = []NodeChild{{N: domain.Node{ID: "c1", Name: "a/b/backstage", Kind: domain.KindRepo}, Total: "2:41 h"}}
	out := renderToBuf(t, testCtx(t), CockpitMain(d))
	if !strings.Contains(out, ">backstage<") || !strings.Contains(out, "2:41 h") {
		t.Fatalf("Enthält section must list children:\n%s", out)
	}
}
```
(Feld `WissenRows` zu `NodeCockpit` ergänzen; `Docs`/`WissenScope` bleiben als Rohquelle, `WissenRows` ist die gebaute Anzeige.)

- [ ] **Step 2: Laufen lassen** — FAIL.

- [ ] **Step 3: `BuildWissenRows` + `WissenRow` in `cockpit_vm.go`** — mappt `domain.Document` → Anzeige-Zeile: `ChipClass=DocTypeChipClass(doc.Type)`, `ChipLabel=DocTypeLabel(doc.Type)`, `Meta` = Akteur + Relativzeit + Pfad (Felder aus `domain.Document` per `rg "type Document struct" -A20 internal/domain/` verifizieren — `Title`, `Path`, `Type`, `UpdatedAt`, Akteur-Feld), `ReadTime` = Wortzahl/220 → „N min" (Body-Wortzahl; wenn kein Body im List-Ergebnis, ReadTime „" lassen — kein Blocker, Spec §16.9).

- [ ] **Step 4: `cockpit_main.templ`** — vier Sektionen (`.sect` + `.sect-h` mit `eyebrow` + `more`-Link, wie Mockup Z.599/611). Wissen-Zeile:
```
<a class="projrow" href={ templ.SafeURL("/wissen/" + row.ID) }>
  <span class={ "typechip", row.ChipClass }>{ row.ChipLabel }</span>
  <div class="grow"><div class="t">{ row.Title }</div><div class="s">{ row.Meta }</div></div>
  if row.ReadTime != "" { <div class="right"><div class="v">{ row.ReadTime }</div><div class="k">{ components.T(ctx, "cockpit.wissen.readtime") }</div></div> }
</a>
```
Enthält-Zeile = `.projrow` mit Avatar + Kurzname (`DisplayNames` über die Kinder) + Pfad + Rollup-Stunden. Buchungen = die `d.SessionRows` als schlanke Zeilen (Datum · Span · Tag · Dauer + Edit-Link/Delete-Dialog — die bestehenden Routen `/nodes/{id}/sessions/{sid}/edit|delete`, Target `#cockpit-main`). Puls = `activityFeedRow` über die Subtree-Activity mit `livechip`.
- **Scope-Toggle Wissen (Nicht-Repo):** zwei Links, `hx-get={ "/nodes/"+d.N.ID+"/main?scope=self" }` / `?scope=subtree`, Target `#cockpit-main` (ersetzt die alte `/tab/wissen?scope=`-Mechanik). Bestehende Keys `cockpit.wissen.scopeSubtree/scopeSelf` wiederverwenden.

- [ ] **Step 5: i18n** — beide Kataloge (nur Neues):
```go
"cockpit.wissen.sectionRepo": "Wissen zu diesem Repo",     // en: "Knowledge on this repo"
"cockpit.wissen.section":     "Wissen zu diesem Knoten",   // en: "Knowledge on this node"
"cockpit.wissen.readtime":    "Lesezeit",                  // en: "Reading time"
"cockpit.enthaelt.title":     "Enthält",                   // en: "Contains"
"cockpit.buchungen.title":    "Buchungen",                 // en: "Bookings"
"cockpit.pulse.subtree":      "Puls — dieses Repo & darunter", // en: "Pulse — this repo & below"
```
(Bestehende `cockpit.wissen.*`, `cockpit.worktime.*`, `cockpit.struktur.*`, `cockpit.pulse.*`, `activity.*` wiederverwenden.)

- [ ] **Step 6: Test grün + Bauen + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... -race
git add -A && git commit -m "feat(lesesaal): Cockpit-Inhalt — Enthält/Wissen/Buchungen/Puls als Haarlinien-Zeilen, Typ-Chips, Lesezeit"
```

**Zustände:** leer (jede Sektion eigene ruhige Leerzeile, keine 0-Kacheln), lang (Doc-Titel/Meta truncate via `.t`/min-w-0; Kinder-Pfad bricht), mobil (Sektionen volle Breite, Rail darunter), laufender Timer (Puls „LIVE", Buchungen-Zeile „läuft"), Fehlerpfad (Nachbuchen-Validierung → `d.PanelErr` in der Buchungen-Sektion, Task 7 Handler).

---

### Task 7: Flatten-Wiring — neue Cockpit-Seite, drei SSE-Container, Tabs sterben, Move→Edit

**Files:**
- Modify: `internal/adapter/webui/cockpit.templ` (`NodeView`/`cockpitBody` neu; `CockpitRail`/`CockpitTabsAndPanel`/`cockpitPanel`/`cockpitTabLink` + Tab-Reste löschen)
- Modify: `internal/adapter/webui/cockpit_vm.go` (`CockpitTabs`/`NormalizeTab`/`cockpitPanelSSE`/`cockpitPanelReloadURL` + `TabCounts`/`ActiveTab`-Nutzung entfernen, soweit tot)
- Delete: `internal/adapter/webui/cockpit_uebersicht.templ` + `cockpit_uebersicht_vm.go` + Tests **nur wenn** keine Nutzer mehr (rg-Check); sonst nur aus dem Cockpit lösen
- Modify: `internal/adapter/httpserver/webui_cockpit.go` (`nodeCockpitData` füllt head+main+rail; `handleWebNodeHead`→`CockpitHead`; neue `handleWebNodeMain`/`handleWebNodeRail`; `handleWebNodeTab` löschen; Mutations-Handler repointen)
- Modify: `internal/adapter/httpserver/server.go` (Routen `/main`+`/rail` rein, `/tab/{name}` raus)
- Modify: `internal/adapter/webui/nodes.templ` (`nodeFormInner` — Move-Formular ergänzen)
- Modify: `internal/adapter/httpserver/webui_nodes.go` (`handleWebNodeEdit` liefert `MoveTargets`)
- Modify: `internal/adapter/webui/activity_row.templ` (`activityTargetPill` glyphfrei)
- Test: alle betroffenen `_test.go` reparieren

**Interfaces:**
- Consumes: `CockpitHead` (T4), `CockpitRailBlocks` (T5), `CockpitMain` (T6).
- Produces: neue `NodeView(d)` = `AppShell("projekte", nil, nil, cockpitBody(d))` mit `cockpitBody`:
```
templ cockpitBody(d NodeCockpit) {
  <div id="cockpit-head"
    hx-get={ "/nodes/" + d.N.ID + "/head" }
    hx-trigger="sse:node.updated, sse:node.moved"
    hx-swap="innerHTML">
    @CockpitHead(d)   // nur Spine (Identität/Pfad/Status) — volle Breite über dem Grid
  </div>
  <div class="cock">
    <div id="cockpit-main" class="min-w-0"   // erstes Element: @cockpitInstr(d), dann die Sektionen
      hx-get={ "/nodes/" + d.N.ID + "/main" }
      hx-trigger="sse:session.started, sse:session.stopped, sse:session.updated, sse:session.deleted, sse:document.created, sse:document.updated, sse:document.deleted, sse:node.created, sse:node.updated, sse:node.moved, sse:node.deleted, sse:activity.logged"
      hx-swap="innerHTML">
      @CockpitMain(d)
    </div>
    <aside class="rail" id="cockpit-rail"
      hx-get={ "/nodes/" + d.N.ID + "/rail" }
      hx-trigger="sse:session.started, sse:session.stopped, sse:session.updated, sse:session.deleted, sse:node.updated, sse:node.moved"
      hx-swap="innerHTML">
      @CockpitRailBlocks(d)
    </aside>
  </div>
  @components.SessionDialog(sessionDialogAddVM(d))
  if d.EditSession != nil {
    @components.SessionDialog(sessionDialogEditVM(d))
  }
  <script src="/static/js/copypath.js" defer></script>
}
```
- **SSE-Regel je Mutation (benannt):**
  | Mutation | emittiertes Event (Bestand) | konsumierender Container |
  |---|---|---|
  | start/stop/switch | session.started/stopped | #cockpit-main (instr-Band + Buchungen/Puls) · #cockpit-rail (Ketten-Stunden) · Topbar-Pill (L1) |
  | Nachbuchen/Edit/Delete Session | session.updated/deleted | #cockpit-main · #cockpit-rail |
  | bind/unbind | node.updated | #cockpit-rail · #cockpit-head (Kind/Status) |
  | move/status/delete (edit page) | node.moved/updated/deleted | Projekte-Seite (`sse:node.*`) + Cockpit-Container (#cockpit-head/#cockpit-main) |
  Die Start/Stop/Switch-Handler emittieren bereits die Events (Bestand, webui_cockpit.go) — hier werden nur die **hx-Targets** der Formulare auf `#cockpit-main` gestellt und die Handler auf `CockpitMain` (via `renderCockpitMain`) gerendert.
  - **Puls live:** `EventActivityLogged` ("activity.logged") wird vom SSE-Emitter-Dekorator publiziert (`sse/emitter.go:36`, konsumiert in `home.templ:64`) — deshalb steht `sse:activity.logged` im `#cockpit-main`-Trigger (NICHT im HTTP-Handler `activity.go` emittiert — dort steht nichts; die Publikation läuft über den Emitter-Wrapper). Zusätzlich decken `session.*`/`document.*`/`node.*` die Puls-Auslöser ab.

- [ ] **Step 1: Failing Integrationstest** — `webui_cockpit_flatten_test.go` (httpserver) oder cockpit_render_test:
```go
func TestCockpitPage_IsFlatNoTabs(t *testing.T) {
	d := seededCockpit()
	out := renderToBuf(t, testCtx(t), NodeView(d))
	for _, want := range []string{`id="cockpit-head"`, `id="cockpit-main"`, `id="cockpit-rail"`, `class="cock"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("flat cockpit missing %q:\n%s", want, out)
		}
	}
	for _, gone := range []string{"pill-tabs", "pill-tab", "/tab/", "CockpitTabsAndPanel"} {
		if strings.Contains(out, gone) {
			t.Fatalf("tab remnant %q still present", gone)
		}
	}
}
```

- [ ] **Step 2: `nodeCockpitData` umbauen** — statt `ActiveTab`-Switch füllt der Builder JETZT head+main+rail in einem Durchgang: `Ancestors`, `Rollup`, `Rate`, `Timer`, `TodayHere`, `LogoShape`, `Contributors`, `Bindings`, `Children` (immer, nicht nur „struktur"), `WissenRows` (aus `wissenTabDocs` → `BuildWissenRows`), `SessionRows` (buchbar), Puls (aus `uebersichtData`-Puls-Quelle oder direkt Activity). Per-Ketten-Ebene-Stunden-Map für `ChainRows` hier bauen (`s.Stats.NodeStats` je Vorfahr). `fillPanelData`/`NormalizeTab`/`TabCounts` entfernen. **Owner-scoped** bleibt (jeder Store-Call trägt `u.ID`).

- [ ] **Step 3: Fragment-Handler** — `handleWebNodeHead`→`CockpitHead(d)`, neu `handleWebNodeMain`→`CockpitMain(d)` (respektiert `?scope=` für Wissen), neu `handleWebNodeRail`→`CockpitRailBlocks(d)`. `handleWebNodeTab` löschen. Routen in `server.go`: `GET /nodes/{id}/main`, `GET /nodes/{id}/rail` rein; `GET /nodes/{id}/tab/{name}` raus (statische Pfade vor `{id}`-Wildcards beachten — wie Bestand). **Fehlerpfad (Gemini-Finding):** jeder neue Fragment-Handler spiegelt die 404-Behandlung von `handleWebNodeHead` (Bestand): `errors.Is(err, ports.ErrNodeNotFound)` → `http.StatusNotFound`, sonst → 500. So liefert ein SSE-Reload nach zwischenzeitlichem `node.deleted` einen sauberen 404 statt Panic. `handleWebNodeView` (Seiten-Level) macht das bereits (Bestand) — nicht anfassen.

- [ ] **Step 4: Zwei neue Render-Helfer + Mutations-Handler repointen.** NEU anlegen (analog zum bestehenden `renderNodeHead`/`renderNodePanel`, die es real gibt): `renderCockpitMain(w, r, u, id, errMsg)` (baut `nodeCockpitData`+`fillMainData`, rendert `CockpitMain`, setzt `d.PanelErr`) und `renderNodeRail(w, r, u, id, errMsg)` (rendert `CockpitRailBlocks`). Dann die Handler umbiegen: **Start/Stop/Switch** → `hx-target="#cockpit-main"`, Handler `renderCockpitMain` (Timer-Band liegt jetzt im Main); **Nachbuchen/Session-Edit/Delete** → `#cockpit-main`, `renderCockpitMain`; **Bind/Unbind** → `#cockpit-rail`, `renderNodeRail`. `renderNodeHead` bleibt für `#cockpit-head` (nur Spine, reload auf node.*). Jeder Fehlerpfad setzt `d.PanelErr` in der jeweiligen Fläche. **Failing Test je neuem Helfer** (Handler-Test: POST → erwartetes Fragment + Status).

- [ ] **Step 5: Move→Edit-Seite (mit neuem VM-Feld + Test).** `NodeFormData` (node_tree_vm.go:125) hat heute NUR `Parents` — **Feld `MoveTargets []domain.Node` ergänzen** (verifiziert: existiert nicht). `handleWebNodeEdit` füllt es via `webui.MoveTargetsFor(all, *editing)` (nur im Edit-Modus). In `nodeFormInner` (nodes.templ) unter dem deaktivierten Parent-Select ein Move-Formular rendern (`method=post action=/nodes/{id}/move hx-boost=false`, Select über `d.MoveTargets`), nur wenn `editing != nil`. **Failing Test zuerst:** ein Render-Test, dass `NodeForm(d, editing)` mit gefüllten `MoveTargets` das Move-`<form action=".../move">` + die Ziel-Optionen enthält. Status + Löschen liegen bereits auf der Edit-Seite (nodes.templ Z.192/269) — nur Move zieht um; die alten `cockpitMoveForm`/`cockpitStatusForm` aus cockpit.templ entfernen.

- [ ] **Step 6: Tab-Leichen entfernen — aber Builder-Bibliothek behalten.** Löschen: `CockpitTabs`, `NormalizeTab`, `cockpitPanel`, `cockpitTabLink`, `CockpitTabsAndPanel`, `cockpitPanelSSE`, `cockpitPanelReloadURL`, `CockpitRail`, `cockpitRailHero`, `cockpitTimer`, `cockpitQuickActions`, `cockpitIdMeta`, `cockpitAccent`, `cockpitTileClass`, `glyphOrDefault`, `pill-tabs`-Script, `cockpit_uebersicht.templ` (die Card-Templates) + `s.uebersichtData` (Handler-Aggregator). **NICHT löschen:** die reinen Builder in `cockpit_uebersicht_vm.go` — `BuildChain`, `BuildComp`, `FilterPulse`, `TopDocs`, `BuildActivityRows`, `pctStyle` + ihre Typen `ChainRow`/`CompRow`/`UebersichtDoc` werden von T5/T6 weiterverwendet. `UebersichtVM` selbst entfernen, falls nur die Card-Templ sie nutzte (`rg "UebersichtVM" internal/ -g '!*_templ.go'` prüfen). `activityTargetPill` (activity_row.templ) glyphfrei machen (`k.Glyph` + `kindToneClass` raus → neutraler Link/`.typechip`). Vor jeder Löschung `rg -n "<Name>" internal/ -g '!*_templ.go'` — **Bestand gewinnt**, nur echt tote Symbole entfernen.

- [ ] **Step 7: Suite reparieren** — `go test ./... -race 2>&1 | tail -40`; Tests, die Tab-Strip/Übersicht/`CockpitRail`/`/tab/`/Kristall asserten, auf die flache Realität anpassen (Assertions ändern, nie Verhalten wegtesten). `cockpit_uebersicht_*_test.go` löschen, wenn die Übersicht weg ist. `webui_cockpit_uebersicht.go` entsprechend.

- [ ] **Step 8: Bauen + Commit**
```bash
make generate && make web && go test ./... -race 2>&1 | tail -20
git add -A && git commit -m "feat(lesesaal): Cockpit flach — drei SSE-Container statt Tabs, Move auf Edit-Seite, Übersicht/Tab-Strip entfernt"
```

**Zustände:** leer/lang/mobil/laufender-Timer/Fehler — über die drei Fragmente bereits in T4–T6 abgedeckt; hier zusätzlich: der **laufende-Timer-Mobil-Fall** (L1-Gate-Lehre, Fix `5d012f4`) explizit prüfen — `#cockpit-head` mit `TimerOtherBound` und 86-Zeichen-`OtherName` darf 375px nicht sprengen (`ShortName`+`title` steht in T4).

---

### Task 8: Wiring-Gate — Leichen-Sweep, volles CI, Live-Smoke, Mobil-Sichtprobe

**Files:** nur was der Sweep findet.

- [ ] **Step 1: Leichen-Sweep**
```bash
rg -n "pill-tabs|pill-tab|CockpitTabs|cockpitPanel|/tab/|CockpitUebersicht|uebersichtData|cockpitAccent|cockpitTileClass|nodeKindBadge|nodeGlyphSwatch|glyphOrDefault|nvdot|navDotClass|◆|▲|●" \
  internal/adapter/webui/nodes.templ internal/adapter/webui/cockpit*.templ internal/adapter/webui/activity_row.templ --glob '!*_templ.go'
rg -n "glass|shadow-lift|from-green to-cyan|rounded-3xl" internal/adapter/webui/nodes.templ internal/adapter/webui/cockpit*.templ
# Kristall-Farbklassen + Kristall-Puls-Reste auf L2-Flächen (Codex-Finding: die
# obigen Muster fangen ActorGlyph/text-blue NICHT):
rg -n "ActorGlyph|text-blue|text-muted|text-body|border-purple|animate-breathe|text-\[\.8|split-bar|chain-bar" internal/adapter/webui/nodes.templ internal/adapter/webui/cockpit_head.templ internal/adapter/webui/cockpit_main.templ internal/adapter/webui/cockpit_rail.templ
```
Expected: 0 Treffer auf L2-Flächen (Glyphen/Kristall-Klassen dürfen in `nodekind.go`/`dockindstyle.go`/`activity_row.templ` als Datenquelle bzw. für Home/L3–L4 bleiben, aber NICHT in nodes/cockpit-L2-Markup). Jeden echten Treffer beseitigen.

- [ ] **Step 2: Tote i18n-Keys** — für in T7 entfernte Cockpit-Tab-/Übersicht-Keys (`cockpit.tab.*`, `cockpit.ov.*`, `cockpit.wp.*`, `cockpit.comp.*`, `cockpit.chain.*`, `cockpit.qa.*`, `cockpit.rail.contributors`/`children` falls tot) prüfen:
```bash
for k in cockpit.tab.uebersicht cockpit.ov.total cockpit.wp.title cockpit.comp.title cockpit.chain.title cockpit.qa.book; do
  echo "$k:"; rg -n "\"$k\"|T\(ctx, \"$k" internal/ -g '!catalog_*.go' | head -2
done
```
Verwaiste Keys aus BEIDEN Katalogen entfernen (de+en-Parität bleibt). `activity.*`/`cockpit.pulse.*` bleiben (Puls lebt).

- [ ] **Step 3: Volles CI**
```bash
make ci
```
Expected: GREEN — lint, verify-generate, verify-css, verify-no-popups, cover ≥75 %, build. (pgstore-Tests: `DOCKER_HOST` auf Podman-Socket.)

- [ ] **Step 4: Live-Smoke** (Dev-Stack; sonst `make dev-up`)
```bash
make dev-run &   # https://localhost:8080 (self-signed)
sleep 2
# Cookie-Flow wie L1-Gate (Bearer trifft webAuth nicht) — echte Node-ID einsetzen:
NID=$(curl -sk -H "Authorization: Bearer $(make -s dev-token)" https://localhost:8080/api/v1/nodes | head -c 400)
echo "$NID"
for p in /nodes "/nodes/<ID>" "/nodes/<ID>/head" "/nodes/<ID>/main" "/nodes/<ID>/rail"; do
  echo "$p"  # im Browser gegen die Cookie-Session prüfen: 200, Rückgrat/Zeilen/Rail rendern
done
```
Expected: `/nodes` zeigt Engagement-Header + Repo-Zeilen; `/nodes/{id}` zeigt Rückgrat (96er) + instr-Band + Kette/Bindings; Start/Stop tickt und aktualisiert alle drei Container via SSE; kein horizontales Pannen. Danach Server stoppen.

- [ ] **Step 5: Browser-Sichtprobe für Soenne notieren** (Abschlusstext): Projekte-Baum (Kurzname groß, Pfad ungekürzt, Schmutzdaten-Note), Cockpit-Rückgrat + Copy-Pfad, Kette/Bindings-Panel, Logo-groß (96/64), Nachbuchen-Dialog + Session-Edit/Delete, Move über Edit-Seite, ⌘K/Topbar/Timer-Pill (L1) weiter intakt. **Zwei Breakpoints prüfen (Spec §12):** (a) **≤960px** — Meta-Spalte (Kette/Bindings) stackt unter die Bühne, `.cock` einspaltig, Sektions-Köpfe brechen um; (b) **375px** — kein horizontales Scrollen inkl. laufendem Timer mit langem `OtherName`, `.k`-Unterzeilen weg, 64er-Avatar.

- [ ] **Step 6: Abschluss-Commit (falls der Sweep etwas fand)**
```bash
git add -A && git commit -m "chore(lesesaal): L2-Gate — Leichen-Sweep + tote Keys + Live-Smoke"
```

---

## Offene Entscheidungen (Soennes Wahl — mit Empfehlung + Trade-offs)

> **ENTSCHIEDEN (Soenne, 2026-07-05): alle acht Empfehlungen übernommen.** Konkret: (1) Cockpit flach, Tabs sterben · (2) Buchungen-Sektion bleibt auf dem Cockpit · (3) Work/Privat nur als Rollup-Zahl + Note im instr-Band, Split-Card entfällt · (4) Move/Status/Delete auf der Node-Edit-Seite · (5) Projekte-Filter-Chips bleiben · (6) Kontext-Instrument-Block fehlt in L2 bewusst (L5) · (7) faint bleibt für rein dekorative Zweitwerte · (8) Dedup-Form `"parent / leaf"`, Reichweite pro sichtbarem Kontext. Die Task-Texte gelten damit unverändert wie geschrieben.

1. **Cockpit: Tabs auflösen (flach) vs. Tabs restyled behalten.** — *Empfehlung: flach* (Plan-Grundannahme). Mockup v2.4 hat keine Tabs; Spec §5/§9 „Rückgrat ersetzt den Baum, Kinder als Listen im Inhalt". Trade-off: großer, test-intensiver Umbau (T7) und ein längeres Scroll-Cockpit vs. mockup-treu + eine ruhige Fläche. Fallback (Tabs behalten) wäre kleiner, aber mockup-widrig — dann T4–T7 anders schneiden.
2. **Session-Ledger (Buchungen) Zuhause:** kompakte „Buchungen"-Sektion auf dem Cockpit (Plan) vs. nach L4/Zeit auslagern. — *Empfehlung: auf dem Cockpit lassen*, damit Session-Edit/Delete nicht verschwindet; Mockup zeigt keinen Ledger (bewusste Ergänzung, ruhig gehalten). Trade-off: eine Sektion mehr als das Mockup.
3. **Work/Privat-Split Zuhause:** L2 zeigt Work/Privat als Wort im instr-`stats`-Band bzw. gar nicht sichtbar (die Split-Card/Comp/Chain-Übersicht entfällt mit den Tabs). — *Empfehlung: in L2 nur die Rollup-Zahl + „zählt als Work/Privat"-Note im instr-Band führen; die dedizierte Split-/Comp-Card fällt weg* (Kette trägt die Kettenzahlen, Enthält die Kinder). Falls Soenne die Split-Visualisierung behalten will: als vierten Rail-`.blk` „Work/Privat" wieder aufnehmen. Trade-off: cockpit-story-Substanz (S1) wird auf eine Zeile reduziert.
4. **Move/Status/Delete Zuhause:** auf die Node-Edit-Seite (Plan; „Bearbeiten" = eine Verwalten-Tür; Status+Delete liegen dort schon, nur Move zieht um) vs. eigener Verwalten-Block auf dem Cockpit. — *Empfehlung: Edit-Seite* (mockup-sauberes Cockpit). Trade-off: eine Navigation mehr zum Verschieben.
5. **Projekte-Filter-Chips (aktiv/archiviert/alle):** behalten als ruhige `.typechip`-Zeile (Plan) vs. streichen. — *Empfehlung: behalten* (Funktion; Mockup zeigt keine, aber Archiv-Zugang ist real). Trade-off: minimale Mockup-Abweichung.
6. **Kontext-Instrument-Rail-Block:** in L2 **abwesend** (Rail = Kette + Bindings), kommt in L5. — *Empfehlung: bestätigen.* Das Mockup zeigt den Block (Budget-Meter/Pins/Kuratieren), aber Kontext-Kuratierung ist explizit L5-Scope. Trade-off: die Meta-Spalte ist in L2 kürzer als im Mockup — bewusst.
7. **faint-Kontrast für dekorative Meta** (geparkter L1-Gate-Punkt #7): Mockup nutzt `faint #98938A` (~3:1) für rein dekorative Meta (Pfade, `.k`-Zeilen), Spec §13 fordert ≥4.5:1 für Text. — *Empfehlung: faint für rein dekorative Zweitwerte belassen (Mockup gewinnt), der tragende Wert steht in `meta`/`ink` daneben* — wie in L1 entschieden. Trade-off: strenge WCAG-AA-Lesart würde `meta` statt `faint` verlangen.
8. **Kurznamen-Dedup-Form:** `"gitlab / group"` (Elternsegment + `" / "` + Leaf) wie im Mockup. — *Empfehlung: so übernehmen* (Mockup-normativ). Offen nur, ob die Dedup-Reichweite pro sichtbarem Kontext (Plan: pro Engagement-Sektion / pro Palette-Ergebnis) oder global sein soll — *Empfehlung: pro sichtbarem Kontext* (weniger Rauschen).

---

## Self-Review-Appendix

### Grounding-Herkunft
- **Primär: First-Hand-Reads** (cockpit.templ, cockpit_vm.go, node_tree_vm.go, nodes.templ, cockpit_uebersicht.templ, webui_cockpit.go, activity_row.templ, logoshape.go, nodekind/nodestyle/dockindstyle.go, domain/node.go, server.go, i18n-Katalog) + Mockup-CSS/HTML (Z.20–675) → Scratch-Dossier `l2-dossier.md`.
- **Jeder im Plan verwendete Bestandsname per `rg` verifiziert** (nichts erfunden): `BuildChain`(cockpit_uebersicht_vm.go:183), `FilterPulse`(:203), `TopDocs`(:216), `BuildComp`(:119), `BuildActivityRows`(activity_row.go:31), `pctStyle`(:247), Test-Helper `ptr`(node_tree_vm_internal_test.go:10) + `renderToBuf`/`seededCockpit`(cockpit_render_test.go), webui-Test-`ctx` = `i18n.WithLocale(...,i18n.DE)`(activity_row_test.go:51), Usecase-Signaturen (NodeStats/NodeAncestors/ListSessionsRange/GetRunningSession/ListDocuments/ExecuteByProject), `activity.go` emittiert NICHTS (→ kein `activity.logged`-Trigger).
- **Parallel-Dossier** via `gemini-bigcontext` (agy) als Redundanz angefordert; die im Plan zitierten Namen sind unabhängig per `rg` gegengeprüft, daher kein Abbruch-Risiko bei CLI-Ausfall.
- **Flow-Recall:** `flow_search_docs` (project-scope, type agent) für „Lesesaal L2" — keine neueren Remote-Docs; lokale Dateien sind kanonisch.

### Spec-Deckung L2 (§17-Scope)
- Baum als Inhalt ✓ (T3) · Pfad-Rückgrat ✓ (T4) · instr-Band ✓ (T4) · Kette/Bindings-Panels ✓ (T5) · Logo/Ersatz groß 96/64 ✓ (T4, L1-Deferred #2) · Kurznamen-Dedup ✓ (T2, L1-Deferred #3) · Glyph-Purge auf L2-Flächen ✓ (T3/T4/T6/T7, L1-Deferred #1) · tc-*-Chips echt verdrahtet ✓ (T2/T6, L1-Deferred #5) · benannte Klassen statt Arbitrary-px ✓ (T1, Gate #6) · Cockpit-Mobil-Umbau ✓ (T1/T4/T7, L1-Deferred #4) · Containment §11 ✓ (durchgehend). **NICHT in L2 (bewusst, Spec §17):** Lese-Ebene/Dokument/Wissen-Regale (L3), Schreibtisch+Zeit (L4), Kontext-Kuratierung/Meter (L5, Offene Entsch. #6), Artefakte (L6), Dunkel-Zwilling (L7).

### Adversariale Lückensuche — Berater-Findings + Verbleib
Beide Berater (`codex exec` + `gemini-bigcontext`/agy) haben den Entwurf gegen Spec+Mockup+Dossier geprüft; beide haben ihre Rohfunde selbst per `rg` gegen den echten Code gegengeprüft und False-Positives gestrichen. Jeder verbleibende Fund ist unten einzeln verbucht.

**Gemini-Findings (14):**
1. **[eingearbeitet]** ≤960px `flex-wrap` für `.eng-h/.spine-main/.instr` fehlte (Spec §12) → Task 1 Responsive-Block vollständig ergänzt.
2. **[eingearbeitet]** Projekte-Request-Fehlerpfad → Task 3 „Zustände" um Request-Fehler/Still-Degradation erweitert.
3.–5. **[eingearbeitet]** 404/Ladefehler der Fragment-Reloads (head/main/rail) → Task 7 Step 3: jeder Fragment-Handler spiegelt `ErrNodeNotFound→404` von `handleWebNodeHead`.
6. **[eingearbeitet/abgedeckt]** Seiten-Level-404 → `handleWebNodeView` macht das bereits (Bestand, nicht anfassen); in Task 7 Step 3 vermerkt.
7. **[eingearbeitet]** Gate prüfte nur 375px → Task 8 Step 5 um 960px-Breakpoint (Meta-Spalte stackt) ergänzt.
8. **[eingearbeitet]** Cross-Tenant-Negativtest fehlte → Task 3 Step 6: Owner-Scope-Negativtest (User A sieht Node von User B NICHT) als Pflicht.
9.–12. **[eingearbeitet]** fehlende rg-Verifikationsschritte für Bestandsnamen → neue Global-Constraint „rg-Verifikation vor jeder Bestandsnutzung" (nennt `NodesFragment`/`nodesOuter`/`nodeCrumbs`/`secStr`/`fmtClockHMS`/`activityFeedRow`/`AppShell`/`sessionDialogAddVM` explizit).
13. **[eingearbeitet]** `renderCockpitMain`/`renderNodeRail` nur im Fließtext → Task 7 Step 4: beide Helfer explizit NEU angelegt (analog `renderNodeHead`/`renderNodePanel`) + Failing-Test je Helfer.
14. **[eingearbeitet]** `cockpitStatusWord` erfunden ohne Impl/Test → Task 4 Step 3: Helfer explizit NEU angelegt (i18n-Key-Weg) ODER `StatusBadge`-Label wiederverwenden.

**Codex-Findings (10 verifiziert):**
1. **[eingearbeitet]** Vorhaben-unter-Vorhaben rekursiv (node.go:99–105, Mockup Z.474–484) → Task 3: Sub-Vorhaben rendert als `.projrow.lvl2` mit `IsVorhaben`, tiefere Kinder via Cockpit-Drill, kein Drop (orphan-safe).
2. **[eingearbeitet]** instr-Band-DOM-Mismatch (Mockup: `.instr` INNERHALB `.cock>.main`) → Struktur korrigiert: `CockpitHead`=nur Spine (volle Breite), `cockpitInstr` ist erstes Element von `CockpitMain`; Timer-Forms → `#cockpit-main`; SSE-Trigger/Tabelle angepasst.
3. **[eingearbeitet]** Puls bleibt Kristall (`activity_row.templ` text-blue/border-purple/ActorGlyph, shared mit Home) → Task 6: eigene Lesesaal-Puls-Zeile aus `[]ActivityRowVM` (Avatar statt Glyph), `activityFeedRow` bleibt für Home/L4 unberührt.
4. **[eingearbeitet — Selbstkorrektur]** `EventActivityLogged` existiert & wird via `sse/emitter.go:36` publiziert (mein Zwischen-Edit hatte es fälschlich entfernt, weil ein früher `rg` nur `activity.go` statt den Emitter-Dekorator prüfte) → `sse:activity.logged` wieder im `#cockpit-main`-Trigger + Notiz korrigiert.
5. **[eingearbeitet]** `ChainRows(d)` ohne Stats-Quelle → neues Feld `NodeCockpit.ChainStats map[string]domain.NodeRollup` (Handler füllt je Vorfahr via `s.Stats.NodeStats`), `ChainRows` adaptiert `BuildChain`.
6. **[eingearbeitet]** `.pagehead/.sect/.sect-h/.more/.dotsep` fehlten in Task 1 (L1 hat nur `.seg`) → CSS + Produces-Liste ergänzt.
7. **[eingearbeitet]** `DocActiveContext` ungetestet → Task 2 Test-Matrix + `switch` um `DocActiveContext→tc-v` (Spec §7.1 context→violett) erweitert.
8. **[eingearbeitet]** tote Assertion in Task-5-Test → durch echte Inhalts-Assertions ersetzt (RTL Extern/87,50/304:46/Kette/krow), `ChainStats` gesetzt.
9. **[eingearbeitet]** Task-8-Sweep fing Puls-Kristall nicht → zweiter `rg` mit `ActorGlyph|text-blue|text-muted|border-purple|animate-breathe|split-bar|chain-bar` auf L2-Flächen.
10. **[eingearbeitet]** Move→Edit unterspezifiziert → Task 7 Step 5: `NodeFormData.MoveTargets []domain.Node` NEU (heute nur `Parents`) + Failing-Render-Test für das Move-Formular.

**Codex „plausibel, ungeprüft" — Verbleib:** SSE-Target-Asymmetrie `node.deleted` (durch Fragment-404-Handling + `#cockpit-main` node.deleted-Trigger abgedeckt), i18n-Keys (jeder Key-Step in beiden Katalogen), `CountNote`-Format (Mockup-normativ, Implementer-Detail), `.projrow`/Dossier-Namens-Mismatch (`.projrow` ist eine NEUE Klasse, kein Bestandsname — kein Konflikt). Kein separater Plan-Change nötig.

**Zurückgewiesene Roh-Findings (begründet):** „Focus-visible-CSS fehlt" — existiert seit L1 (`tailwind.css:196`); „WissenRowVM-Widerspruch" — Halluzination (Feld heißt `WissenRow`/`WissenRows`); „Namen nicht im Dossier" (`AppShell`/`secStr`/`bindingTarget`/`wissenTabDocs`/`sessionDialogAddVM`) — alle real im Code (Dossier war nur nicht erschöpfend, kein Plan-Gap); TDD-Reihenfolge-Stilkritik — Stilurteil, außerhalb des Auftrags.

Planner-Selbstprüfung (Raster a–d):
- **(a) Spec-Absatz ohne Task:** §10 „Kontext-Instrument" → bewusst L5 (Offene Entsch. #6). §9 „Dokument/Wissen-Seite" → L3. Alles §9-Cockpit/Projekte + §5/§7/§8/§11/§12 → auf Tasks gemappt (s. Deckung).
- **(b) Zustände je Task:** leer/lang/mobil-375/laufender-Timer/Fehler in T3/T4/T5/T6/T7 explizit benannt.
- **(c) Querschnitte:** main.go-Wiring → T3/T7 prüfen explizit „kein neuer Server-Feld/Konstruktor" (alle Usecases existieren); SSE je Mutation → T7-Tabelle; i18n beide Kataloge → jeder Key-Step; Responsive → T1 + T4/T6; Owner-Scoping → jeder Handler-Step trägt `u.ID`.
- **(d) Tests + rg-Verifikation:** jeder Task hat failing-Test-first; Bestandsnamen (Document-Felder, Test-Helper, `s.ListSessionsRange`/`GetRunningSession`, `bindingTarget`, `domain.Doc*`) mit explizitem `rg`-Verifikationsstep, „Bestand gewinnt".
- **Interner-Widerspruch-Check:** `NodeCockpit` bekommt neue Felder `WissenRows` (T6) + `ChainStats` (T5), T7 entfernt tote Tab-Felder (`ActiveTab`/`TabCounts`/`Uebersicht`); Signatur-Konsistenz `CockpitHead/CockpitMain/CockpitRailBlocks(d NodeCockpit)` durchgängig. **DOM-Konsistenz:** `cockpitInstr` liegt in `CockpitMain` (nicht `CockpitHead`) → Mockup-DOM `.cock>.main>.instr`; Timer-Forms → `#cockpit-main`; `#cockpit-head` (Spine) reloadet nur auf `node.*`. `activity.logged` bleibt im `#cockpit-main`-Trigger (Emitter-verifiziert).
