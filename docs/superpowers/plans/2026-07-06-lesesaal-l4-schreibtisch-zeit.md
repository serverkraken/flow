# Lesesaal L4 — Schreibtisch + Zeit (Home als ruhiger Einstieg · Zeit als Tages-Ledger + Wochenskala) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Der **letzte sichtbare-App-Slice**. Zwei Flächen sterben aus der Kristall-Ära und werden neu gesetzt: der **Schreibtisch** (`/`, Home) wird einspaltig-schmal (860px) — **Jetzt** (Timer-Panelzeile) → **Weiterarbeiten** (MRU-Knoten) → **Zuletzt im Wissen** (Lesesaal-Doc-Zeilen) → **Puls** (LIVE-Feed); Saldo-Kacheln/Burndown/Logstream-Filter fallen. Die **Zeit**-Seite (`/zeit`) wird der einspaltige **Tages-Ledger** (Von–Bis Mono · Ziel · Dauer, LIVE-Zeile) → **Wochenskala** (Panel, Balken akzent/heute-grün, Soll-Zeile) → **Werkzeuge** (Export · Freie Tage · Statistik · Historie); der Heute/Woche/Historie-Sub-Tab-Strip stirbt. Flankierend ziehen die **Werkzeuge-Destinationen** (Woche, Historie, Frei, Export) auf Lesesaal-Tokens um (Minimal-Restyle wie der L3-Editor, kein Rebuild), damit **kein sichtbares Kristall** übrig bleibt (Spec §17: L1–L4 ersetzen die sichtbare App vollständig). Es gibt weiterhin **genau einen** Timer (Topbar-Pill + Cockpit-instr-Band; die Jetzt-Zeile ist eine Anzeige des einen Instruments, kein dritter Start-CTA — Spec §10).

**Architecture:** Server-rendered wie gehabt (templ + htmx + Tailwind, kein SPA, kein Node). **Kein `cmd/flow-server/main.go`-Change** — alle Usecases (`Stats`, `ListSessionsRange`, `ListNodes`, `GetRunningSession`, `ListDocuments`, `ListActivity`, `ListDayOffs`, `nodeMaps`) sind seit M1/K-Ära verdrahtet; L4 baut nur webui-Rendering + VM-Formung um, plus die MRU-Ableitung. Der **Live-Tick** ist Bestand: jedes `<span data-timer data-timer-fmt="clock" data-base="<sek>">` wird vom Inline-Skript in `base.templ` (nonce-gedeckt, CSP-konform) unabhängig getickt und nach `htmx:afterSwap` neu gebunden — Schreibtisch-Jetzt-Zeile und Zeit-LIVE-Zeile nutzen es ohne neues JS. Die **Puls-Zeile** wird geteilt: das in L2 gebaute, reviewte `cockpitPulseRow` (Lesesaal `.projrow`) wird zu einem paketweiten `pulseRow` promotet und auf Home wiederverwendet; `activityFeedRow` (Kristall) stirbt auf Home. **Reuse statt Neubau:** „Zuletzt im Wissen" nutzt das L3-`WissenRowVM` + `WissenRowFromDocument`; die MRU-Knoten leiten sich (wie die ⌘K-Palette) aus den letzten Sessions ab, kein neues Schema. Neue Anzeige-Logik lebt in reinen, unit-getesteten Go-Buildern (`webui`-Paket, domain-frei); die templ-Komponenten nehmen fertige VMs. **SSE-Live-Sync** bleibt: Schreibtisch-`#content` und Zeit-`#content` ziehen nach `session.*`/`document.*`/`dayoff.changed`/`activity.logged` nach.

**Tech Stack:** Go 1.x · templ · Tailwind v4.1.5 (CLI, `make web`) · htmx (vendored, SSE-Extension) · Schibsted Grotesk + JetBrains Mono (L1). Keine neuen Abhängigkeiten, keine Migration, kein neues Vendoring.

**Spec:** `docs/superpowers/specs/2026-07-04-lesesaal-webui-redesign-design.md` (§9 Zeilen **Schreibtisch** + **Zeit** · §10 Timer genau einer + Puls · §11 Eindämmung soweit relevant · §12 Responsive/<620px · §13 A11y · §17 L4-Definition). **Normatives Mockup:** `docs/superpowers/specs/assets/2026-07-03-lesesaal/lesesaal.html` (v2.4 — bei Zweifel gewinnt das Mockup; **Schreibtisch = Z. 348–440**, **Zeit = Z. 845–892**, **CSS = Z. 20–322**, davon Schreibtisch/Zeit-relevant: `.narrow` Z.87 · `.panelrow` Z.101 · `.row`-Familie Z.97–116 · `.puls` Z.263–266 · `.weekbar`/`.day` Z.252–261 · Responsive Z.284–321).

---

## Global Constraints

- Branch **`lesesaal-l4`** (frisch off `rebuild` `bc97092`, auszuchecken); Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`. **Committe NIE als Planner** — der Orchestrator committet nach Soennes Plan-Review; die Implementer-Dispatches committen am Task-Ende.
- **NIE `make fmt`** ausführen. **NIE `git stash`** in Dispatches (L3-T8-Vorfall). Nach jedem Task: `git log --oneline -3` prüfen (HEAD vorangegangen?) + `git diff --stat HEAD~1` — Subagent-Commits können den Branch-Ref verfehlen (Memory).
- `make ci` muss am Task-Ende grün sein (Gate 75 %, aktuell 85,6 %; `*_templ.go` ausgeschlossen; **pgstore-Tests brauchen den Podman-Socket** — `DOCKER_HOST` auf den Podman-Socket setzen). L4 berührt keine Migration/keinen Store — die pgstore-Docker-Tests bleiben unverändert grün.
- Nach JEDER `.templ`-Änderung: `make generate` und die `*_templ.go` mitcommitten. **Nach JEDER `web/tailwind.css`-Änderung UND nach jeder templ-KLASSEN-Änderung/-LÖSCHUNG: `make web`** und `internal/adapter/webui/static/app.css` mitcommitten. **LEHRE L3 (zweimal verify-css-Drift):** auch reine templ-Löschungen ändern den Tailwind-Klassen-Scan → `make web` ist Pflicht, sobald eine `.templ` angefasst wird, nicht nur bei `tailwind.css`. verify-css ist ein Drift-Diff.
- i18n: jede neue Nutzertext-Zeile in **beiden** Katalogen (`internal/i18n/catalog_de.go` + `catalog_en.go`); de+en-Parität ist test-enforced. Keine hartkodierten Anzeige-Strings; `components.T(ctx, "key")`/`components.Tn(...)`.
- Keine Emojis (monospace-Glyphen ● ◆ ⬡ ▶ ■ ✚ ✗ ○ · + SVG erlaubt), **keine Browser-Popups** (`verify-no-popups`; Löschen über `components.ConfirmDialog`, Bestand). Löschen/Nachbuchen behalten ihre Dialoge.
- **owner-scoped** bleibt überall unangetastet (jede Store-Query trägt `u.ID`; „ist nur ein User" ist keine Begründung, AGENTS.md §Grundsätze). Jede neue/umgebaute Datenfläche bekommt einen **Owner-Scope-Negativtest**, wo sie fremde Owner-Daten laden könnte (Schreibtisch-MRU, Schreibtisch-Wissen, Zeit-Ledger).
- **Farb-Gesetz (Spec §7):** Farbe pro Projekt existiert NUR im Avatar (`components.Avatar(Initials, AvatarTone, size)` / `AgentAvatar`). **Dokumenttyp-Farben fest & semantisch** (`.tc-*`, `DocTypeChipClass`/`DocTypeLabel`) — auf Typ-Chips. **Akteure:** Mensch = getönte Initialen; **Agent = gestrichelter Rahmen** (`.ava-agent`). Kinds bleiben neutral. Keine Kristall-Formcodierung/`kindToneClass`-Tönung/`swatchStyle`-Ecke auf L4-Flächen.
- **Timer genau EINER (Spec §10):** Start bleibt am Topbar-Pill (`/ui/timer/start`) + Cockpit-instr-Band. Die Jetzt-Panelzeile (Schreibtisch) und die LIVE-Zeile (Zeit) sind **Anzeigen** desselben laufenden Timers; ihr etwaiger Stop-Knopf postet auf das **eine** `/ui/timer/stop` (kein zweites Usecase, kein dritter Start-CTA). Der Live-Tick nutzt `data-timer`/`data-base` (Bestand, `base.templ`).
- **Design nur über Tokens/Primitives/benannte Klassen** (Gate-Punkt): wo das Mockup harte Maße vorgibt (860px, weekbar-Balken, Von–Bis-Mono), eine **benannte Klasse** in `web/tailwind.css` (Task 1) statt Arbitrary-`[px]`. Dynamische Balkenhöhe (`style="height:{Pct}%"`) ist erlaubte Datenbindung, kein Design-Token (Präzedenz: `wocheDayBarStyle`). Tokens `var(--panel)`/`--paper`/`--surface`/`--hair`/`--live-bright`/`--accent`/`--meta`/`--faint` stehen seit L1.
- **SSE-Regel:** Schreibtisch-`#content` (`/ui/home`) zieht auf `session.started/stopped/updated/deleted`, `document.created/updated/deleted`, `dayoff.changed`, `activity.logged` nach; Zeit-`#content` (`/ui/worktime`) auf `session.*`, `dayoff.changed`, `settings.changed`. **Jede** Mutation dieser Flächen läuft über bestehende Usecases, die ihr Event bereits emittieren (Timer→`session.*`, Nachbuchen/Edit/Delete→`session.*`, Frei→`dayoff.changed`) — kein neues Event. Der konsumierende Container ist je Task benannt.
- Tailwind-v4-Fallen (Memory): kein `<alpha-value>` in `@theme`; niemals `*/` in CSS-Kommentaren; `@source not`-Zeilen (`docs/`, `.claude/`) nicht anfassen.
- **rg-Verifikation vor jeder Bestandsnutzung (Prozess-Pflicht):** JEDES als „Bestand" referenzierte Symbol (Template, Helfer, Handler, VM-Feld, Komponente, Usecase-Feld, Test-Helper — z. B. `HomeVM`, `homeDataFor`, `HomeFragment`, `HeuteVM`, `heuteDataFor`, `HeuteFragment`, `worktimeSubnav`, `sessionRowVM`, `fmtClockRange`, `heuteWeekRows`, `SessionRowVM`, `WocheVM`, `WocheDayVM`, `wocheDataFor`, `WissenRowVM`, `WissenRowFromDocument`, `cockpitPulseRow`, `activityFeedRow`, `ActivityRowVM`, `BuildActivityRows`, `FmtRelTime`, `FmtVerbose`, `FmtSaldoVerbose`, `TimerWidgetVM`, `GetRunningSession`, `ListSessions`, `ListSessionsRange`, `StatsComputer.Week`, `EventNodeCreated`, `ListActivity`, `nodeMaps`, `BuildPaletteVM`, `Avatar`, `AgentAvatar`, `Initials`, `AvatarTone`, `ShortName`, `DocTypeChipClass`, `DocTypeLabel`, `Card`, `StatTileAccent`, `EmptyState`, `SessionDialog`, `BurndownBanner`, `renderToBuf`, `testCtx`, `i18n.WithLocale`) vor dem Tippen per `rg -n "<Name>" internal/ -g '!*_templ.go'` gegen den echten Code prüfen. **Bestand gewinnt** — Signaturen/Feldnamen exakt übernehmen, nichts erfinden.
- **Auflösung der Codex-#18-Schutzregel (L3):** In L3 blieben `DocRow`/`docRowFromDocument`/`swatchStyle`/`rowKind`/`BuildHomeNewest`/`homeNewestDocRow` **bewusst unberührt**, weil sie die noch-Kristall-Schreibtisch-Seite trugen. **In L4 wird der Schreibtisch ersetzt → diese Kette darf sterben** (verifiziert: `docRowFromDocument` hat außer `BuildHomeNewest`/`home_newest.go` **keinen** Konsumenten). ABER **zwei Ausnahmen, verifiziert am Code — beide BLEIBEN:** (1) `kindToneClass` (nodekind.go) hat einen zweiten Konsumenten `nodes.templ:135` (Node-Kind-Chip, Projekte); (2) `sortedDocuments` (wissen_vm.go:215) wird von `BuildWissenOverview` (wissen_vm.go:155) genutzt **und** von Home als Newest-Sort wiederverwendet. Retirement pro Symbol nur nach `rg`-Beweis „null Konsumenten" (Task 2/6).

## Timer-genau-einer + Live-Tick — Vorgabe (ENTSCHIEDEN, Spec §10; NICHT erneut konsultieren)

Es gibt **kein** neues Timer-Usecase und **keinen** neuen Start-CTA in L4. Start/Stop/Switch bleiben `/ui/timer/{start,stop,switch}` (Bestand, `webui_timer.go`), gerendert als Topbar-Pill (`/ui/timer`) + Cockpit-instr-Band. Die Jetzt-Panelzeile (Schreibtisch) und die Zeit-LIVE-Zeile zeigen den **laufenden** Timer (aus `s.GetRunningSession.Execute`, owner-scoped) mit tickender Mono-Uhr (`data-timer data-timer-fmt="clock" data-base={BaseSeconds}`). **VERIFIZIERT (webui_timer.go): `/ui/timer/stop` → `handleTimerStop` → `renderTimerWidget` rendert `webui.TimerWidget` (das Sheet-Body), NICHT `TimerPill`** — deshalb darf ein Stop von der Jetzt-Zeile **nicht** in `#timer-pill` swappen (das würde Sheet-Markup in den Pill-Mount kippen). **Korrektes Muster:** `hx-post="/ui/timer/stop" hx-swap="none"` — die Antwort wird verworfen; `#timer-pill` (eigener `sse:session.stopped`-Trigger) und `#content` (Schreibtisch-SSE) ziehen die Optik nach. **Zweite Verifikation:** `handleTimerStop` bucht bei **gebundenem** Running auf dessen Node (ok); bei **ungebundenem** Running ohne Node-Formularwert → Fehler `timer.needNode`. Deshalb zeigt die Jetzt-Zeile den Stop **nur bei gebundenem** laufenden Timer (`Now.NodeID != ""`); ein ungebundener Timer wird über das Topbar-Pill-Sheet (mit Node-Picker) gestoppt. **Kein** Start-Knopf auf Schreibtisch/Zeit (Idle-Zustand ist informativ, nicht handlungsauffordernd — Offene Entsch. #1/#2). Die Zeit-LIVE-Ledger-Zeile ist **anzeige-only** (kein Stop — Mockup Z.861–866).

## Agent-Besetzung & Dispatch-Protokoll (übernommen aus L1–L3, Auditor-Zeilen auf Schreibtisch/Zeit angepasst)

Rollen als Projekt-Agents in `.claude/agents/` (Modell + Effort im Frontmatter fest). Orchestrator-Session `/effort high`. Dispatches nennen das Modell NIE implizit (Memory: nie Fable erben).

| Task | Agent (`subagent_type`) | Modell · Effort |
|---|---|---|
| 1 L4-CSS-Klassen (narrow/panelrow/weekbar/day/num/dim) | `lesesaal-implementer` | Sonnet · medium |
| 2 Schreibtisch-Rebuild (Jetzt/Weiterarbeiten/Wissen/Puls; MRU-Builder; pulseRow-Promotion; Retirement) | `lesesaal-implementer-deep` | Sonnet · high |
| 3 Zeit-Hub-Rebuild (Ledger/Wochenskala/Werkzeuge; Subnav weg) | `lesesaal-implementer-deep` | Sonnet · high |
| 4 Werkzeuge-Destinationen Woche + Historie (Restyle + Statistik-Rolle) | `lesesaal-implementer-deep` | Sonnet · high |
| 5 Werkzeuge-Destinationen Frei + Export (Restyle) | `lesesaal-implementer` | Sonnet · medium |
| 6 Asset-Fingerprinting + Cache-Control (`/static/` — PROD-Cache-Bug) | `lesesaal-implementer-deep` | Sonnet · high |
| 7 Wiring-Gate (Leichen-Sweep · tote Keys · make ci · Live-Smoke · Breakpoints) | `lesesaal-implementer` | Sonnet · medium |
| jedes Task-Review | `lesesaal-task-reviewer` | Haiku · high |
| Slice-Ende: Whole-Branch | `lesesaal-final-reviewer` | Opus · xhigh |
| Slice-Ende: Design-Treue | `lesesaal-mockup-auditor` | Sonnet · medium |

**Protokoll pro Task:**
1. Dispatch Implementer mit: wörtlichem Task-Text + Global-Constraints-Block + Timer-Vorgabe + „Branch `lesesaal-l4`, Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`". Ein Task pro Dispatch.
2. Orchestrator verifiziert danach selbst: `git log --oneline -3` (HEAD vorangegangen?) + `git diff --stat HEAD~1`.
3. Dispatch `lesesaal-task-reviewer` mit Task-Text + Commit-Range (BASE = Task-Base, nie HEAD~1). `Rejected`/Critical → Fix-Dispatch an denselben Implementer; Minor darf der Orchestrator selbst fixen.
4. Ledger `.superpowers/sdd/progress.md` fortschreiben (Commits, Verdikt, ci-Stand).

**Protokoll Slice-Ende (feste Reihenfolge):**
1. `make ci` grün.
2. **Rest-Sweep** (mechanisch): `gemini-bigcontext` (agy) über `git diff --name-only rebuild..HEAD`; Fallback `code-searcher`. Dispatch-Text unten.
3. `lesesaal-final-reviewer` (Range `rebuild..HEAD`) → Findings fixen.
4. `lesesaal-mockup-auditor` → Abweichungen fixen (Referenzzeilen: Schreibtisch Mockup Z.348–440, Zeit Z.845–892, CSS Z.20–322).
5. **Soenne-Live-Gate** (Browser, nicht delegierbar) — inkl. laufender-Timer-Tick auf Schreibtisch UND Zeit gleichzeitig, Weiterarbeiten-MRU-Reihenfolge, Puls-Live, Zeit-Wochenskala (Balken akzent/heute-grün, Soll-Zeile), Werkzeuge-Links, Frei-Live-Weekbar, 960px- und 375px-Sichtprobe (kein horizontales Pannen; Werkzeuge-Destinationen ohne Kristall).
6. Nachlauf: Auto-Memory + flow-Mirror des Ledgers/Plans (`flow_update_doc`).

**Dispatch-Text Rest-Sweep (`<RANGE>` = `rebuild..HEAD`):**
> Lies vollständig: alle Dateien aus `git diff --name-only <RANGE>` plus `web/tailwind.css`, `internal/adapter/webui/static/app.css`. Finde ausschließlich: (a) **Kristall-Reste auf L4-Flächen** (home*.templ/heute*.templ/woche*.templ/historie*.templ/frei*.templ/export*.templ + deren _vm.go): `glass`, `shadow-soft`, `shadow-lift`, `font-display`, `bg-gradient-to-r`, `from-green`/`to-cyan`/`from-*`, `kindToneClass`, `swatchStyle`, `rowKind`, `KindGlyph`, `rounded-3xl`/`rounded-2xl` als Karten-Optik, `components.Card`/`StatTile`/`BurndownBanner` auf Home/Zeit-Hub, Formcodierungs-Glyphen ◆/▲/●; (b) **Arbitrary-Tailwind-Werte** (`text-[#`, `bg-[#`, `rounded-[`, `shadow-[`, `text-[.`, `text-[1`, `w-[`, `h-[`) auf L4-Flächen, wo eine benannte Lesesaal-Klasse existiert (Ausnahme: dynamische `style="height:…%"`/`width:…%"`-Datenbindung); (c) **verwaiste i18n-Keys** (definiert, nirgends per `T(`/`Tn(` referenziert) — besonders Home-Logstream-Filter (`activity.filter.*`, `activity.actor.all`) und Saldo/Burndown-Keys, falls die Umstellung sie entfernt hat, sowie tote `heute.*`/`stats.tile*`; (d) **verwaiste Symbole** (`DocRow`, `docRowFromDocument`, `swatchStyle`, `rowKind`, `BuildHomeNewest`, `homeNewestDocRow`, `activityFeedRow`, `logstreamHref`, `classToPrefix`, `handleHomeLogstream`, `logQuery`) mit **null** verbleibenden Konsumenten (`rg`-Zähler); `kindToneClass` NICHT melden (nodes.templ-Konsument, Bestand). Ausgabe: gruppierte Liste `Datei:Zeile — Befund`, KEINE Fixes, KEINE Stilurteile.

**Hinweis Memory-Bank:** keine `CLAUDE-*.md` im Repo → `memory-bank-synchronizer` wird übersprungen; Nachlauf ist Orchestrator-Arbeit (Auto-Memory + flow-Mirror).

---

### Task 1: Lesesaal-L4-Komponentenklassen — Schreibtisch-Panelzeile + Zeit-Wochenskala als benannte Klassen

**Files:**
- Modify: `web/tailwind.css` (`@layer components`, hinter den L1/L2/L3-Primitives, vor den `@media`-Blöcken bzw. deren Ergänzung)
- Modify: `internal/adapter/webui/components/styleguide.templ` (Lesesaal-L4-Sektion)
- Test: `internal/adapter/webui/components/styleguide_test.go` (Muster der L3-Sektion; Render-Smoke)

**Interfaces / Produces (für Tasks 2/3/4/5):** benannte Klassen exakt aus dem Mockup-CSS. **Vor dem Tippen prüfen, was L1–L3 schon hat** (`rg -n "\.narrow|\.panelrow|\.weekbar|\.day\b|\.num\b|\.dim\b|\.puls|\.pulse\b|\.row\b|\.livechip|\.sect\b|\.pagehead" web/tailwind.css` — vorhanden: `.row`-Familie, `.projrow`, `.sect`/`.sect-h`/`.more`, `.pagehead`, `.eyebrow`, `.livechip`, `.targetlink`, `.pulse .who`/`.pulse .agentname`, `.panel`, `.btn*`, Avatare, `.typechip`/`.tc-*`). **Nur Fehlendes ergänzen; Bestand gewinnt bei Konflikt nur, wenn es dem Mockup schon entspricht.** Neu:
- **`.narrow`** (Mockup Z.87): `max-width:860px` (auf den `.wrap`/Container-`div` der schmalen Seiten). Bestand: der Container ist heute `mx-auto w-full max-w-[1140px]` in `AppShell` — die schmalen Seiten (Home/Zeit) setzen ihren **inneren** Wrapper auf `.narrow` (`max-width:860px;margin:0 auto`). Verifizieren, wie L2/L3 die schmalen Seiten begrenzen (`rg -n "max-w-\[860|narrow|max-w-\[680" internal/adapter/webui`); falls es schon eine benannte 860er-Klasse gibt, diese nutzen.
- **`.panelrow`** (Mockup Z.101, Jetzt-Timer-Zeile): `background:rgb(var(--panel));border-radius:14px;padding:13px 16px;border-bottom:none;margin-top:12px` — die eine zweite Fläche der Jetzt-Zeile (Zwei-Flächen-Regel). Nutzt sonst die `.row`-Kinder (`.grow/.t/.s/.right/.v/.k`), die schon existieren.
- **`.weekbar` + `.day`** (Mockup Z.252–261, Zeit-Wochenskala — **vertikale** Balken, neu; die Bestand-Wochenstreifen `heuteWeekRows`/`wocheDayBarStyle` sind horizontal und bleiben unberührt): `.weekbar{display:flex;gap:26px;align-items:flex-end;padding:20px 18px 14px;background:rgb(var(--panel));border-radius:14px;margin-top:14px;overflow-x:auto}`, `.day{flex:1;min-width:36px;text-align:center}`, `.day .bar{height:74px;display:flex;align-items:flex-end;justify-content:center}`, `.day .bar i{display:block;width:22px;border-radius:3px 3px 0 0;background:rgb(var(--hair2));min-height:3px}`, `.day.has .bar i{background:rgb(var(--accent))}`, `.day.today .bar i{background:rgb(var(--live-bright))}`, `.day .d{margin-top:8px;font-size:11.5px;color:rgb(var(--faint))}`, `.day .v{font-family:"JetBrains Mono",…;font-size:11.5px;color:rgb(var(--meta));margin-top:2px;font-variant-numeric:tabular-nums}`.
- **`.num` + `.dim`** (Mockup Z.60–62, Zeit-Ledger Von–Bis): `.num{font-family:"JetBrains Mono",…;font-variant-numeric:tabular-nums}`, `.dim{color:rgb(var(--meta))}`. Für die Von–Bis-Zeile im Ledger (Mockup nutzt `<span class="num dim" style="font-size:13px">`) eine benannte Modifier-Klasse statt Arbitrary-Inline: **`.led-when`** (`font-family:mono;font-variant-numeric:tabular-nums;color:rgb(var(--meta));font-size:13px;flex-shrink:0`) — Task 3 nutzt sie. Kein inline `style="font-size:…"`.
- **Puls-Reconcile:** das Mockup nennt die Home-Puls-Sektion `.puls`; **Bestand ist `.pulse`** (`.pulse .who`/`.pulse .agentname`, Cockpit L2). **Bestand gewinnt → Home nutzt `.sect.pulse` + `pulseRow` (Task 2), kein neues `.puls`.** Nur wenn der Mockup-Auditor `.puls` strikt fordert, einen Alias `.puls{}` mit denselben Kind-Regeln — aber nicht doppeln; im Task vermerkt.
- **Soll-Zeile (Mockup Z.870):** die „Diese Woche"-Kopfzeile hat rechts `<span class="dim" style="font-size:12.5px;margin-left:auto">Soll … · bisher … · auf Kurs</span>` → benannte Klasse **`.sect-h .note`** (`margin-left:auto;font-size:12.5px;color:rgb(var(--meta))`) statt Arbitrary-Inline. (Prüfen: `.instr .note` existiert schon als andere Regel — hier eine `.sect-h`-scoped Variante.)
- **Responsive:** in den bestehenden 620px-Block ergänzen (nicht neuen Block): `.weekbar{gap:10px}` (Mockup Z.311) **und `.pagehead h1{font-size:27px}`** (Mockup Z.314 — **Gemini-Fund #5**: die bestehende `.pagehead h1`-Regel ist nur einmal bei 34px definiert, keine 620px-Variante; sie betrifft **beide** neuen Seiten (Schreibtisch + Zeit nutzen `.pagehead`). `.narrow` bleibt fluid (max-width, kein fixer Wert unter 960px nötig). (`.row .right .k{display:none}` <620px existiert aus L3 — greift auch für die neuen `.row`-Zeilen.)

**Zustände dieser Fläche:** /ui-Styleguide zeigt die Jetzt-`.panelrow` (mit LIVE + Stop), eine `.weekbar` mit 7 `.day`s (leer=`—`, has=Balken, today=grün), eine `.led-when`-Ledger-Zeile — Sichtprobe im Gate 375px (weekbar scrollt via `overflow-x:auto`, kein H-Pannen).

- [ ] **Step 1: Mockup-CSS + Bestand prüfen**
```bash
sed -n '87,101p;252,266p;284,321p' docs/superpowers/specs/assets/2026-07-03-lesesaal/lesesaal.html
rg -n "\.narrow|\.panelrow|\.weekbar|\.day\b|\.num\b|\.dim\b|\.puls|\.pulse\b|\.sect-h .note|max-w-\[860" web/tailwind.css internal/adapter/webui
```
- [ ] **Step 2: Failing Test** — in `styleguide_test.go` (Komponente heißt `StyleguidePage()` — L3-Lehre; `rg "templ StyleguidePage|func testCtx" internal/adapter/webui/components/` vorher):
```go
func TestStyleguide_HasLesesaalL4Section(t *testing.T) {
	var sb strings.Builder
	if err := components.StyleguidePage().Render(testCtx(t), &sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	for _, want := range []string{"panelrow", "weekbar", "led-when"} {
		if !strings.Contains(out, want) {
			t.Fatalf("Styleguide misses Lesesaal-L4 demo of %q", want)
		}
	}
}
```
- [ ] **Step 3: Test laufen lassen** — Expected: FAIL.
- [ ] **Step 4: Klassen in `web/tailwind.css` ergänzen** — wörtlich aus dem Mockup, `#hex` → `rgb(var(--token))`. `.day.has`→`--accent`, `.day.today`→`--live-bright`, `#fff`/Panel→`--panel`/`--surface`. `@source not`-Zeilen nicht anfassen.
- [ ] **Step 5: Styleguide-Sektion „Lesesaal L4"** — eine `.narrow`-Demo mit `.panelrow` (Jetzt: Avatar + „Timer läuft auf …" + `.livechip` + `.led-when` + Stop), eine `<div class="weekbar" role="img" aria-label="…">` mit 7 `.day` (has/today/leer — die `role="img"`+aria-label-Struktur zeigen, damit Task 3 sie kopiert), eine `.sect` mit `.sect-h .note` + zwei `.row`-Ledger-Zeilen. Bestehende Sektionen unangetastet.
- [ ] **Step 6: Bauen + Tests + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... -race
git add -A && git commit -m "feat(lesesaal): L4-Layout-Klassen (narrow/panelrow/weekbar/day/num/dim/led-when) als benannte Klassen"
```
Expected: PASS; `git status` zeigt geänderte `app.css`.

---

### Task 2: Schreibtisch (Home) — Jetzt · Weiterarbeiten · Zuletzt im Wissen · Puls (Reuse + Retirement)

**Files:**
- Create: `internal/adapter/webui/home_continue.go` + `internal/adapter/webui/home_continue_test.go` (MRU-Knoten-Builder; kein Monolith)
- Modify: `internal/adapter/webui/home.templ` + `internal/adapter/webui/home_vm.go`
- Modify: `internal/adapter/webui/activity_row.templ` (`pulseRow`/`pulseSection` promoten) + `internal/adapter/webui/cockpit_main.templ` (auf geteiltes `pulseRow` umstellen)
- Modify: `internal/adapter/httpserver/webui_home.go` (`homeDataFor` neu; `GetRunningSession` + MRU laden)
- Modify: `internal/adapter/httpserver/server.go` (Logstream-Route entfernen — Offene Entsch. #3)
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go`
- Test: `internal/adapter/webui/home_render_test.go`, `internal/adapter/httpserver/webui_home_test.go`, **`internal/adapter/webui/wissen_vm_test.go`** (die L3-Schutztests `TestSwatchStyle` (:168) + `TestDocRowUnaffected` (:183) sind selbst Konsumenten von `swatchStyle`/`BuildHomeNewest` — Codex-Fund #4; beim Retirement der Symbole **mit entfernen/anpassen**, sonst bricht die Kompilierung. Vor der Löschung gehören sie zum rg-Konsumenten-Zähler.)
- Retire (nach rg-Beweis null **Nicht-Test**-Konsumenten + Anpassung der obigen Schutztests): `home_newest.go` (`BuildHomeNewest`), `homeNewestDocRow` (home.templ), Home-Nutzung von `activityFeedRow`, `handleHomeLogstream`/`logstreamHref`/`classToPrefix`/`HomeVM.logQuery` + `webui_home_logstream_test.go`; `DocRow`/`docRowFromDocument`/`swatchStyle`/`rowKind` (verifiziert konsumentenfrei außer den zwei Schutztests). `sortedDocuments` + `kindToneClass` bleiben (siehe Global Constraint).

**Interfaces:**
- **`webui.RecentNode` + `BuildRecentNodes(sessions []domain.WorkSession, nodes []domain.Node, now time.Time, n int) []RecentNode`** (home_continue.go, domain-frei, unit-getestet). MRU aus Sessions wie die Palette (`rg -n "recent|MRU|last 30|ws.NodeID" internal/adapter/httpserver/webui_palette.go` — Muster verifizieren): distinkte **bookable** Knoten, nach jüngster Session absteigend, cap `n` (5). Felder: `{ ID, Name (ShortName), FullPath, Tone (AvatarTone), Initials, ValueStr (z. B. „2:41 h" laufend / „gestern" / letzte Session-Dauer), LabelKey/LabelStr (z. B. „läuft gerade"/„zuletzt aktiv") }`. **Kein neues Schema** — nur Ableitung. Kurzname/Pfad über `ShortName`/den Knoten-Pfad (verifizieren, wie L2 den vollen Mono-Pfad rendert: `rg -n "FullPath|\.path|Pfad" internal/adapter/webui/nodes.templ`).
- **`pulseRow(row ActivityRowVM)` + `pulseSection(titleKey string, rows []ActivityRowVM)`** in `activity_row.templ` — das L2-`cockpitPulseRow` **wörtlich** hierher promoten (identische Struktur: `.projrow` + `Avatar`/`AgentAvatar` + `.who`/`.agentname` + `.targetlink` + `.right .v` RelTime). `cockpit_main.templ` ruft danach `pulseRow` statt `cockpitPulseRow` (Cockpit-Render-Test muss grün bleiben — `rg -n "cockpitPulseRow" internal/adapter/webui/` alle Aufrufer umziehen). `activityFeedRow` bleibt zunächst (falls noch woanders genutzt — `rg`), stirbt aber auf Home.
- **`HomeVM`-Refactor** (`home_vm.go`): entfernen `TodaySaldo/Pos/Sub`, `WeekSaldo/Pos/Sub`, `MonthSaldo/Pos/Sub`, `Burndown`, `LogClass/LogActor/LogActors`, `NewestDocs []DocRow`, `logQuery()`. Neu: `Now *RunningNowVM` (nil = idle), `TodayLogged string` (für die Idle-/Jetzt-Zeile „heute 5:12 h"), `Continue []RecentNode`, `RecentWissen []WissenRowVM` (cap 5), `Puls []ActivityRowVM` (cap 8), `Err`. `RunningNowVM{ NodeID, NodeName (ShortName), NodeHref, Initials, Tone, BaseSeconds int64, SinceStr string, CountsWork bool }` (aus `GetRunningSession` + Node-Auflösung; `SinceStr` = „seit 14:32"; `CountsWork` → „zählt als Work/Privat", via `ResolveCountsTowardTarget`/Node-Attribut — verifizieren, sonst weglassen).
- **`homeDataFor`-Rewrite** (`webui_home.go`): owner-scoped. `s.GetRunningSession.Execute(ctx, u.ID)` (Guard: nil-Usecase → idle); **`s.ListSessions.Execute(ctx, u.ID, since)`** (`since = now.AddDate(0,0,-30)`) — **die exakt gleiche Signatur wie der zitierte Palette-Präzedenzfall** (`webui_palette.go:23`, Codex-Fund #6; **nicht** `ListSessionsRange`, das eine `(since, until)`-Signatur hat) — `s.ListNodes.Execute` → `BuildRecentNodes`; `s.Stats.Today` nur noch für `TodayLogged` (kein Saldo-Tile mehr); `s.ListDocuments.Execute` + `WissenRowFromDocument` (cap 5, newest-first — **`sortedDocuments` wiederverwenden**, das bleibt, statt `BuildHomeNewest`); `s.ListActivity.Execute(…, 8, 0)` + `BuildActivityRows` → `Puls`. **`nodeMaps`** weiter für Activity-Target-Namen.
- **`home.templ`-Rebuild:** `HomePage`/`homeBody`/`homeOuter`/`HomeFragment`-Signaturen **stabil** (Handler-Caller unberührt). `homeOuter` `#content` `hx-get="/ui/home"` `hx-trigger="sse:session.started, sse:session.stopped, sse:session.updated, sse:session.deleted, sse:document.created, sse:document.updated, sse:document.deleted, sse:dayoff.changed, sse:activity.logged, sse:settings.changed"` `hx-swap="innerHTML"` (**`settings.changed` behalten** — Bestand-Parität, Gemini-Fund #4; harmlos, kein settings-abhängiger Wert mehr auf Home, aber kein stiller Trigger-Verlust). `HomeFragment` rendert:
  - `.narrow`-Wrapper + `.pagehead` (`.eyebrow`=Datum lokalisiert, `h1`=„Schreibtisch", `.sub`).
  - **Jetzt** (`.sect` + `.sect-h`: `.eyebrow`=„Jetzt" + `.more` „Zeit ›"→`/zeit`): läuft → `.row.panelrow` (Avatar `av-36` + `.grow`(„Timer läuft auf" + `.targetlink`→`NodeHref`; `.s`=`SinceStr` + Work/Privat) + `.livechip` + `.right`(`<span data-timer data-timer-fmt="clock" data-base={BaseSeconds}>` + `.k`=„heute {TodayLogged}") + **Stop nur bei `Now.NodeID != ""`** (gebundener Timer): `.btn.btn-q.btn-s` `hx-post="/ui/timer/stop" hx-swap="none"` (Antwort verworfen; Pill + `#content` ziehen per SSE nach — **NICHT** in `#timer-pill` swappen; Begründung im Timer-Vorgabe-Block)). Idle → **eine ruhige `.row.panelrow`** ohne Start-CTA: „Kein Timer läuft" + `.k`=„heute {TodayLogged}" (Offene Entsch. #1/#2).
  - **Weiterarbeiten** (`.sect` + `.sect-h` „Weiterarbeiten" + `.more` „Alle Projekte ›"→`/nodes`): für jeden `RecentNode` ein `<a class="row" href="/nodes/{ID}">` (Avatar/Logo · `.t`=Name · `.path`=FullPath · `.right .v`=ValueStr · `.k`=LabelStr). Leer → ruhige Zeile „Noch keine Aktivität".
  - **Zuletzt im Wissen** (`.sect` + `.sect-h` „Zuletzt im Wissen" + `.more` „Wissen ›"→`/wissen`): für jeden `WissenRowVM` ein `<a class="row" href="/wissen/{ID}">` mit `.typechip {ChipClass}` + `.grow`(`.t`=Title · `.s`=Meta) + `.right`(`.v`=TimeStr). **Bewusste Mockup-Abweichung (Gemini-Fund #6):** das Mockup Z.401–403 zeigt `.right` mit **zwei** Kindern (`.v`=Uhrzeit + `.k`=„heute"); `WissenRowVM` (Bestand, geteilt mit `/wissen`) hat **kein** `.k`-Feld und `TimeStr` ist bereits `FmtRelTime` (relativ, z. B. „vor 3 Std") — also nur `.right .v`, **kein `.k`**. So bleibt der geteilte Typ unverändert und die Home-„Zuletzt im Wissen"-Zeile ist **konsistent mit der L3-`/wissen`-„Zuletzt aktualisiert"-Zeile** (die genauso `WissenRowVM.TimeStr` rendert). Wiederverwenden der L3-Wissen-„Zuletzt"-Zeile falls sie schon eine templ-Komponente ist (`rg -n "WissenRowVM|wissenRecentRow|wissenRow" internal/adapter/webui/wissen.templ`); sonst eine kleine geteilte `wissenRow(row)` bauen.
  - **Puls** (`.sect.pulse` + `.sect-h` „Puls" + `.livechip` „LIVE"): `pulseRow` je Eintrag; leer → „Noch keine Aktivität".
- **Retirement** (nach rg-Beweis): `homeNewestDocRow`, `HomeLogstream`/`homeLogstreamInner`, Home-`activityFeedRow`, `handleHomeLogstream` + Route `/ui/home/logstream` + `logstreamHref`/`classToPrefix`/`logQuery`, `BuildHomeNewest`/`home_newest.go`, und (verifiziert konsumentenfrei) `DocRow`/`docRowFromDocument`/`swatchStyle`/`rowKind`. **`sortedDocuments` BLEIBT** (BuildWissenOverview:155 + Home-Reuse). **`kindToneClass` BLEIBT** (nodes.templ:135). Vor jeder Löschung `rg -n "\b<Symbol>\b" internal/ -g '!*_templ.go'` = 0 (außer Definition); sonst stehen lassen + im Ledger vermerken (Task 7 räumt nach).

**Zustände dieser Fläche:** leer (kein Timer/keine MRU/kein Doc/kein Puls → ruhige „—"/„Noch keine …"-Zeilen, keine leeren Kacheln), lang (86-Zeichen-`FullPath` in `.row .path` bricht via `word-break:break-all`; langer Node-Name → `ShortName`+`title`), mobil 375px (`.narrow` volle Breite, `.row .right .k` weg <620px), **laufender Timer** (Jetzt-`.panelrow` mit tickender Uhr + Stop; Puls zeigt „Timer gestartet"; nach Stop zieht `#content` per SSE nach), Fehlerpfad (`vm.Err` als ruhige Zeile; degradierte Usecases → idle-Seite, keine Crashes — Guards wie Bestand).

- [ ] **Step 0: rg-Verifikation (Bestand gewinnt)**
```bash
rg -n "func .*homeDataFor|HomeVM|HomeFragment|homeOuter|homeNewestDocRow|BuildHomeNewest" internal/adapter/webui internal/adapter/httpserver -g '!*_templ.go'
rg -n "cockpitPulseRow|activityFeedRow|pulseRow" internal/adapter/webui -g '!*_templ.go'
rg -n "GetRunningSession|ListSessionsRange|ListActivity|nodeMaps|BuildPaletteVM|recent" internal/adapter/httpserver/webui_home.go internal/adapter/httpserver/webui_palette.go
rg -n "WissenRowVM|WissenRowFromDocument|DocTypeChipClass" internal/adapter/webui/wissen_vm.go
rg -n "ShortName|Initials|AvatarTone|func Avatar\b|AgentAvatar" internal/adapter/webui -g '!*_templ.go'
rg -n "\bDocRow\b|docRowFromDocument|swatchStyle\b|rowKind\b|sortedDocuments" internal/ -g '!*_templ.go'   # Retirement-Zähler
```
- [ ] **Step 1: Failing Builder-Test** — `home_continue_test.go`: `BuildRecentNodes` liefert distinkte bookable Knoten neuest-zuerst, cap 5, mit ShortName/FullPath/Tone; ein laufender Timer-Knoten steht vorne mit „läuft gerade". (Store-/Domain-Muster + `IsBookable` vorher per `rg` verifizieren.)
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3: `home_continue.go` implementieren** + `pulseRow`/`pulseSection` promoten + `cockpit_main.templ` umstellen (`cockpitPulseRow`→`pulseRow`).
- [ ] **Step 4: `HomeVM`-Refactor + `homeDataFor`-Rewrite** (GetRunningSession + MRU + RecentWissen + Puls; Saldo/Burndown/Logstream raus). Handler-Tests (`webui_home_test.go`) anpassen — Kristall-Assertions (`stats.tileToday`, Burndown, Filter-Chips) auf Lesesaal (`panelrow`, `.row`, `pulse`) umstellen, **Owner-Scope-Negativtest** für MRU + Wissen (User A sieht kein Doc/keine Session von User B).
- [ ] **Step 5: `home.templ`-Rebuild** + Retirement (Logstream-Route in `server.go` entfernen; `home_newest.go` löschen wenn frei; `homeNewestDocRow`/`HomeLogstream` raus). i18n-Keys (beide Kataloge):
```go
"home.desk":        "Schreibtisch",                 // en: "Desk"
"home.deskSub":     "Ein Ort für Arbeit und Wissen — weiter, wo Du aufgehört hast.", // en: "A place for work and knowledge — pick up where you left off."
"home.now":         "Jetzt",                        // en: "Now"
"home.timerOn":     "Timer läuft auf",              // en: "Timer running on"
"home.timerSince":  "seit %s · zählt als %s",       // en: "since %s · counts as %s"
"home.noTimer":     "Kein Timer läuft",             // en: "No timer running"
"home.todayTotal":  "heute %s",                     // en: "today %s"
"home.continue":    "Weiterarbeiten",               // en: "Continue"
"home.continueAll": "Alle Projekte",                // en: "All projects"
"home.runningNow":  "läuft gerade",                 // en: "running now"
"home.lastActive":  "zuletzt aktiv",                // en: "last active"
"home.recentWissen":"Zuletzt im Wissen",            // en: "Recently in Knowledge"
"home.puls":        "Puls",                         // en: "Pulse"
"home.emptyRecent": "Noch keine Aktivität",         // en: "No activity yet"
```
(Bestehende `home.greeting`/`home.activity`/`activity.on`/`activity.empty`/`nav.*` prüfen/wiederverwenden. `stats.tileToday/Week/Month`, `heute.legend*`, `activity.filter.*`, `activity.actor.all` → Task 7 prüft, ob nach der Umstellung tot.)
- [ ] **Step 6: VOLLE Suite + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -race 2>&1 | tail -20
git add -A && git commit -m "feat(lesesaal): Schreibtisch — Jetzt-Panelzeile/Weiterarbeiten-MRU/Zuletzt-im-Wissen/Puls (WissenRowVM+pulseRow reuse); Saldo/Burndown/Logstream entfernt; DocRow-Kette geräumt"
```
Expected: PASS; Cockpit-Render-Test grün (pulseRow-Umzug).

---

### Task 3: Zeit-Hub (`/zeit`) — Tages-Ledger + Wochenskala + Werkzeuge (Sub-Tab-Strip stirbt)

**Files:**
- Modify: `internal/adapter/webui/heute.templ` + `internal/adapter/webui/heute_vm.go`
- Modify: `internal/adapter/httpserver/webui_heute.go` (`heuteDataFor`: All-Time-Sub + 7-Tage-Wochenskala + Werkzeuge)
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go`
- Test: `internal/adapter/webui/heute_vm_*_test.go` (falls vorhanden), `internal/adapter/httpserver/webui_heute_test.go`, `webui_heute_internal_test.go`

**Interfaces:**
- **`worktimeSubnav` fällt vom Zeit-Hub** (Mockup Zeit hat keinen Sub-Tab-Strip): `heuteBody` ruft `AppShell("zeit", nil, nil, heuteOuter(vm))` (subnav = nil). **WICHTIG (Kompilier-Sicherheit): `worktimeSubnav` ist in `heute.templ:7` DEFINIERT und wird von `woche.templ`/`historie.templ` noch AUFGERUFEN** — beim Rebuild von `heute.templ` die **Definition stehen lassen** (nur den Aufruf in `heuteBody` entfernen), sonst brechen Woche/Historie die Kompilierung. Die Definition stirbt erst in **Task 4**, nachdem Woche/Historie auf pagehead + „‹ Zeit"-Rücklink umgezogen sind. `rg -n "worktimeSubnav" internal/adapter/webui -g '!*_templ.go'` — Aufrufer vor jeder Löschung zählen.
- **`HeuteVM`-Refactor:** behalten `Running *domain.WorkSession`, `Ledger []HeuteLedgerRow` (jetzt Lesesaal-Zeilen), `Nodes`/`HasProj`/`DayParam` (Nachbuchen-Dialog), `WeekTotal`/`WeekGoal` (für die Soll-Zeile). **Ersetzt (Gemini-Fund #3): die alte Mo–Fr-Strip-Kette `WeekRows []HeuteWeekRow` + `heuteWeekRows` (webui_heute.go) + die 6 Streifen-Helfer `heuteBarFill`/`heuteBarStyle`/`heuteLabelClass`/`heuteValueClass`/`heuteDotClass`/`heuteDotGlyph`/`heuteDotTitle` (heute_vm.go) sind nach dem Umstieg auf die vertikale 7-Tage-`.weekbar` VERWAIST (nur heute-lokal konsumiert — `rg -n "heuteWeekRows|HeuteWeekRow|heuteBarFill|heuteDotClass" internal/ -g '!*_templ.go'` = 0 außerhalb heute) → in diesem Task entfernen, Task 7 sweept nach.** Entfernen ebenso die Saldo-Tile-Felder (`LoggedDur`/`TargetDur`/`Balance`/`BalancePos`/`TargetPct`/`TargetVar` — Kacheln fallen; `LoggedDur` ggf. behalten für die Sub-Zeile). Neu: `DateTitle string` (h1 = „Donnerstag, 3. Juli"), `AllTimeSub string` (Σ-Zeile), `WeekDays []ZeitWeekDay` (7 Tage Mo–So, Builder `zeitWeekDays`), `WeekGoalLine string` („Soll 40:00 · bisher 21:34 · auf Kurs"), `Tools []ZeitTool`.
- **`ZeitWeekDay{ Label, ValueStr, Pct int, Has bool, Today bool }`** (vertikale Wochenskala): aus der **rohen Wochenberechnung**, NICHT aus dem lossy `WocheDayVM` (Codex-Fund #3: `WocheDayVM` trägt nur vorformatierte Strings `Dur string`/`Pct`, `Pct` ist bei `Target==0` unbrauchbar → `Has`/Balkenhöhe nicht ableitbar). **Verifizierte Quelle:** `s.Stats.Week(ctx, u.ID, time.Time{})` → `[]domain.WeekDay` (**7 Tage Mo–So**, `stats_computer.go:158`) mit rohem `wd.Total(now) time.Duration` + `wd.Target` + `wd.IsToday` + `wd.Date`. `heuteDataFor` ruft `s.Stats.Week` heute schon (für den Mo–Fr-Streifen) — L4 nutzt **alle 7** Tage. Neuer, unit-getesteter Builder **`zeitWeekDays(week []domain.WeekDay, now time.Time) []ZeitWeekDay`** (Muster `heuteWeekRows`, aber 7 Tage, kein Wochenend-Skip): `Has = wd.Total(now) > 0`; `Today = wd.IsToday`; `Pct = ClampPct(int(logged*100 / scale))` mit `scale = max(wd.Target, maxLoggedInWeek)` (proportionale Balkenhöhe, kein NaN bei Target==0); `ValueStr = FmtVerbose(wd.Total(now))`, bei 0 → „—", bei DayOff/Wochenende-ohne-Log → „frei" (DayOff-Flag via `s.ListDayOffs`, wie `wocheDataFor` es tut — `rg -n "ListDayOffs|DayOff" internal/adapter/httpserver/webui_woche.go`). `rg -n "type WeekDay struct|func .*Total\(|func .*\bWeek\(" internal/domain internal/usecase/stats_computer.go` — Felder/Signatur exakt.
- **`ZeitTool{ TitleKey, DescKey, Href }`** (Werkzeuge-Zeilen): `{export /export}`, `{dayoffs /dayoffs}`, `{stats /woche}` (Statistik = Woche-Destination, Offene Entsch. #5), `{historie /historie}` (Historie als 4. Zeile für Auffindbarkeit, Offene Entsch. #6). Mockup-Erweiterung (3→4) dokumentiert.
- **`AllTimeSub`** (Σ-Zeile, Mockup Z.851): „Σ {total} h in {n} Sessions seit {date} · {m} freie Tage gepflegt". Datenquelle **verifiziert owner-scoped**: `s.ListSessions.Execute(ctx, u.ID, time.Time{})` (Server-Feld `ListSessions usecase.ListSessions`, `since=time.Time{}` = all-time) → Σ `Elapsed(now)` + Count + earliest `Start`; Frei-Count aus `s.ListDayOffs.Execute(ctx, u.ID, …)`. Kein neuer Rechenpfad, kein `StatsComputer`-Zusatz. **Wenn der All-Sessions-Scan bei großen Tenants zu teuer wird:** vereinfachen auf „{n} Sessions · heute {logged}" (Offene Entsch. #8). Nie ungescoped.
- **`heute.templ`-Rebuild:** `HeutePage`/`heuteBody`/`heuteOuter`/`HeuteFragment` stabil; `heuteOuter` `#content` `hx-get="/ui/worktime"` `hx-trigger="sse:session.started, sse:session.stopped, sse:session.updated, sse:session.deleted, sse:node.created, sse:dayoff.changed, sse:settings.changed"` `hx-swap="innerHTML"` (**Gemini-Fund #4:** `dayoff.changed` **neu** → Wochenskala-frei-Markierung live; **`node.created` ersetzt das im Bestand genutzte tote `project.created`** — `rg -n "EventNodeCreated|project.created|node.created" internal/domain internal/adapter/webui/home.templ` bestätigt: der reale Event ist `node.created`, damit der Nachbuchen-Node-Picker nach einem Timer-Quick-Create live nachzieht). `HeuteFragment` rendert:
  - `.narrow` + `.pagehead` (`.eyebrow`=„Zeit", `h1`=`DateTitle`, `.sub`=`AllTimeSub`).
  - **Heute** (`.sect` + `.sect-h` „Heute" + `.more` „Nachbuchen ›" → öffnet `nachbuchen-dialog`): je Ledger-Zeile eine `.row`: `<span class="led-when">{TimeRange}</span>` + `.grow`(`.t`=Node/Title · `.s`=Note/Tags) + (laufend → `.livechip`) + `.right`(`.v`=Duration; laufend → `<span data-timer data-timer-fmt="clock" data-base={BaseSeconds}>`). **Laufende Zeile (Gemini-Fund #9, Mockup Z.862 „14:32 – läuft"):** `led-when` für die laufende Session zeigt „{Start} – {T(ctx,"heute.running")}" (Bestand-Key = „Läuft") statt `fmtClockRange`s „–…" — entweder `fmtClockRange` um einen laufend-Fall erweitern oder im templ fallunterscheiden (`row.Running`). Bearbeiten/Löschen wie Bestand (Zeile öffnet Edit-`SessionDialog`; abgeschlossene Zeile hat Delete-`ConfirmDialog`) — Funktionalität **erhalten**, nur Optik auf `.row`. Leer → ruhige „Noch keine Sitzung heute"-Zeile (kein Start-CTA; §10). Nachbuchen-`SessionDialog` + „✚ Nachbuchen"-Affordanz bleiben (`data-dialog-close-on-success`, L3-Bestand).
  - **Diese Woche** (`.sect` + `.sect-h` „Diese Woche" + `.sect-h .note`=`WeekGoalLine`): `<div class="weekbar" role="img" aria-label={ T(ctx,"zeit.weekAria") }>` (Spec §13 / Mockup Z.871 — **A11y-Pflicht, nicht optional**) mit 7 `.day` (`.day.has`/`.day.today`; `.bar > i style="height:{Pct}%"`; `.d`=Label; `.v`=ValueStr).
  - **Werkzeuge** (`.sect` + `.sect-h` „Werkzeuge"): je `ZeitTool` ein `<a class="row" href="{Href}">` mit `.grow`(`.t`=Titel · `.s`=Beschreibung) + `.right .v`=„›".
  - `.colo`-Kolophon optional (Mockup Z.889) — nur wenn L2/L3 Kolophone rendern (`rg -n "colo" internal/adapter/webui`); sonst weglassen.
- **StatTiles/BurndownBanner fallen vom Zeit-Hub** (die Metrik zieht auf die Statistik/Woche-Destination, Task 4 — Offene Entsch. #4).

**Zustände dieser Fläche:** leer (keine Session heute → ruhige Zeile; Wochenskala mit `—`-Tagen; keine Kacheln), lang (langer Node-Name → `ShortName`+`title`; Tags/Note umbrechen), mobil 375px (`.weekbar` gap:10 + `overflow-x:auto` scrollt, kein H-Pannen; `.row .right .k` weg), **laufender Timer** (LIVE-Ledger-Zeile mit tickender Uhr; Wochenskala „heute"-Balken grün `--live-bright`), Fehlerpfad (Server-Fehler → 500 wie Bestand; degradierte Stats → Sub-Zeile ohne Σ).

- [ ] **Step 0: rg-Verifikation** — `rg -n "func .*heuteDataFor|HeuteVM|HeuteFragment|heuteWeekRows|HeuteWeekRow|sessionRowVM|fmtClockRange|worktimeSubnav" internal/adapter/webui internal/adapter/httpserver -g '!*_templ.go'` · `rg -n "type WeekDay struct|func .*Total\(|func \(.*StatsComputer\) Week\(" internal/domain internal/usecase/stats_computer.go` (rohe Wochenskala-Quelle) · `rg -n "GetRunningSession|ListSessions\b|ListDayOffs|Stats\.|nachbuchen-dialog|data-dialog-close-on-success|EventNodeCreated" internal/adapter/httpserver/webui_heute.go internal/adapter/webui/heute.templ internal/domain`.
- [ ] **Step 1: Failing Test** — `webui_heute_test.go`/`webui_heute_internal_test.go`: `HeuteFragment` enthält `.weekbar`/`.day`, eine `.row.led-when`-Ledger-Zeile, die Werkzeuge-Zeilen (Export/Freie Tage/Statistik/Historie-Hrefs), **kein** `stats.tileToday`/`worktimeSubnav`; ein laufender Timer erzeugt eine `.livechip` + `data-base`-Zeile. **Owner-Scope-Negativtest** für den Ledger + All-Time-Aggregat.
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3: `heute_vm.go` + `heuteDataFor`** — VM-Refactor; 7-Tage-Wochenskala via `s.Stats.Week`(+`s.ListDayOffs`)→`zeitWeekDays` (NICHT `wocheDataFor`); alte Mo–Fr-Strip-Kette (`heuteWeekRows`/`HeuteWeekRow`+6 Helfer) entfernen; `AllTimeSub` (owner-scoped via `s.ListSessions`, mit Fallback); `Tools`. `heuteBody` subnav=nil (Definition von `worktimeSubnav` stehen lassen — Task 4).
- [ ] **Step 4: `heute.templ`-Rebuild** (Ledger `.row`/`.led-when`, Wochenskala `.weekbar`, Werkzeuge `.row`); Nachbuchen/Edit/Delete-Dialoge erhalten. i18n (beide Kataloge):
```go
"zeit.eyebrow":       "Zeit",                        // en: "Time"
"zeit.today":         "Heute",                       // en: "Today"
"zeit.nachbuchen":    "Nachbuchen",                  // en: "Add entry"   (oder heute.nachbuchen wiederverwenden)
"zeit.thisWeek":      "Diese Woche",                 // en: "This week"
"zeit.weekAria":      "Wochenübersicht der gebuchten Stunden", // en: "Weekly overview of logged hours" (aria-label .weekbar, §13/Mockup Z.871)
"zeit.weekGoal":      "Soll %s · bisher %s · %s",    // en: "Target %s · so far %s · %s"
"zeit.onTrack":       "auf Kurs",                    // en: "on track"
"zeit.behind":        "im Rückstand",                // en: "behind"
"zeit.tools":         "Werkzeuge",                   // en: "Tools"
"zeit.tool.export":   "Export",                      "zeit.tool.export.desc": "CSV · JSON · Markdown — mit Satz und Summen je Engagement",
"zeit.tool.dayoffs":  "Freie Tage",                  "zeit.tool.dayoffs.desc": "Urlaub, Feiertage, Gleittage — mit ICS-Übernahme",
"zeit.tool.stats":    "Statistik",                   "zeit.tool.stats.desc": "Saldo, Burndown, Tagesziele",
"zeit.tool.historie": "Historie",                    "zeit.tool.historie.desc": "Vergangene Sitzungen ansehen und bearbeiten",
"zeit.allTimeSub":    "Σ %s in %d Sessions seit %s · %d freie Tage gepflegt", // en: "Σ %s across %d sessions since %s · %d days off tracked" — VIER Platzhalter %s/%d/%s/%d in exakt dieser Reihenfolge (Gemini-Fund #7: TestCatalogsParity prüft nur Key-Existenz, NICHT Platzhalter-Anzahl/-Reihenfolge → die EN-Reihenfolge muss der DE-Reihenfolge folgen)
"zeit.emptyToday":    "Noch keine Sitzung heute",    // en: "No session today yet"
"zeit.live":          "LIVE",                        // en: "LIVE"  (oder cockpit.pulse.live wiederverwenden)
```
(Bestehende `heute.nachbuchen`/`heute.sessions`/`week.total`/`nav.today` prüfen/wiederverwenden; Datums-Titel via bestehendem Datums-Formatierer — `rg -n "Weekday|Format.*Januar|MonthName|dayLayout" internal/adapter/webui internal/adapter/httpserver`.)
- [ ] **Step 5: Bauen + Tests + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -race 2>&1 | tail -20
git add -A && git commit -m "feat(lesesaal): Zeit-Hub — Tages-Ledger + Wochenskala (7 Tage) + Werkzeuge; Sub-Tab-Strip + Saldo-Kacheln entfernt; dayoff.changed live"
```

---

### Task 4: Werkzeuge-Destinationen — Woche + Historie auf Lesesaal-Tokens (Statistik-Rolle + Metrik-Umzug)

**Files:**
- Modify: `internal/adapter/webui/woche.templ` + `internal/adapter/webui/woche_vm.go` (nur falls Klassen-Strings dort), `internal/adapter/webui/historie.templ` + `internal/adapter/webui/historie_vm.go`
- Modify: `internal/adapter/webui/heute.templ` (den `worktimeSubnav`-Helfer endgültig entfernen) — ODER wo er wohnt
- Modify: `internal/adapter/httpserver/webui_woche.go` (Burndown/Saldo-Metrik hierher, Offene Entsch. #4) — optional
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go`
- Test: `internal/adapter/httpserver/webui_woche_test.go`, `internal/adapter/httpserver/webui_worktime_handlers_test.go`, `woche_vm_internal_test.go`

**Interfaces:** **Kein Verhaltens-/Routen-Change** an Woche/Historie (KW-Nav, Kalender, Reassign, Bulk-Delete bleiben funktional). Nur:
- **Sub-Tab-Strip weg:** `worktimeSubnav` wird **entfernt** (kein Konsument mehr nach Task 3+4); Woche/Historie rufen `AppShell("zeit", nil, nil, …)` und bekommen einen **pagehead** (`.eyebrow`=„Zeit / Woche" bzw. „Zeit / Historie", `h1`) + einen **„‹ Zeit"-Rücklink** (`.spine .up`-Muster oder ein einfacher `.more`-Link → `/zeit`). `rg -n "worktimeSubnav|components.TabStrip" internal/adapter/webui` — sicherstellen, dass nach der Löschung kein Aufrufer bleibt.
- **Restyle** (Minimal, wie L3-Editor — kein Rebuild): `glass`/`shadow-soft`/`shadow-lift`/`bg-gradient`/`from-*`/`to-*`/`font-display`/`rounded-3xl`/`rounded-2xl`-Karten + `components.Card` → Lesesaal-Panels/-Zeilen/-Primitives (`.panel`, `.sect`, `.row`/`.projrow`, `.btn`/`.btn-q`, `.weekbar` für Woche-Balken falls passend, `.krow` für Kennzahlen). Ein Primär-Button pro Sicht. Die Woche-Balken dürfen die neue `.weekbar` (vertikal) ODER das Bestand-`wocheDayBarStyle` (horizontal, benannte Klassen) nutzen — konsistent zum Zeit-Hub bevorzugt `.weekbar`.
- **Statistik-Rolle (Offene Entsch. #4/#5):** Woche IST das Statistik-Ziel des Werkzeugs. **Empfehlung: die monatliche Burndown/Saldo-Metrik (bislang auf Home) auf die Woche-Seite umziehen** (als `.panel`-Kennzahlenblock oder `BurndownBanner` auf Lesesaal-Tokens), damit sie nicht verloren geht. `rg -n "BurndownBanner|burndownBannerVM|Stats.Burndown" internal/adapter/webui internal/adapter/httpserver` — Bestand wiederverwenden. Wählt Soenne die Alternative (Metrik ganz deferren), entfällt der Umzug.
- Historie: Kalender-Grid + Session-Liste + Reassign/Bulk-Delete-Controls auf Lesesaal-Tokens (Zeilen `.row`, Panels `.panel`, Buttons `.btn*`); Auswahl-JS (`historie-select.js`) unangetastet.
- **SSE-Härtung (Gemini-Fund #8, klein): Historie-`#content`-Trigger** (`historie.templ:21`/`:324`, heute nur `session.*, project.created`) um **`dayoff.changed`** ergänzen (der Kalender zeigt Feiertags-/Urlaubs-Markierungen → soll bei Frei-Änderung live nachziehen) und **`project.created`→`node.created`** (reales Event) korrigieren. Woche trägt `dayoff.changed` bereits (`woche.templ:22`). Kein Verhaltens-Change, nur ein fehlender Live-Trigger; Task 7-Live-Smoke prüft die Historie-Frei-Markierung mit.

**Zustände:** leer (keine Woche/History-Daten → ruhige Leerzeile), lang (Node-Namen `ShortName`+`title`), mobil 375px (Woche-Balken scrollen; Kalender-Grid bricht/scrollt im eigenen Rahmen), laufender Timer (Woche „heute"-Balken markiert; unbeteiligt sonst), Fehlerpfad (bestehende Fehleranzeige an Tokens angleichen).

- [ ] **Step 1: Bestand prüfen** — `rg -n "glass|shadow|gradient|font-display|rounded-3xl|rounded-2xl|components.Card|from-|to-cyan|worktimeSubnav" internal/adapter/webui/woche.templ internal/adapter/webui/historie.templ`.
- [ ] **Step 2: Failing Test** — Woche/Historie-Handler-Test: Fragment enthält **kein** `glass`/`worktimeSubnav`/`components.Card`, dafür `.panel`/`.row`/`.sect`; KW-Nav/Reassign-Routen antworten weiter 200. (Bestehende Assertions auf Kristall-Klassen umstellen, nie Verhalten wegtesten.)
- [ ] **Step 3: Restyle Woche + Historie** + `worktimeSubnav` entfernen + pagehead/Rücklink; Burndown/Saldo-Umzug (falls Empfehlung).
- [ ] **Step 4: i18n** — neue Keys für pagehead/Rücklink (`zeit.back`, `woche.title`, `historie.title`) in beide Kataloge; tote Woche/Historie-Kristall-Keys in Task 7.
- [ ] **Step 5: Bauen + Tests + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -race 2>&1 | tail -20
git add -A && git commit -m "feat(lesesaal): Woche + Historie auf Lesesaal-Tokens (Sub-Tab-Strip entfernt, Statistik-Rolle, Burndown-Umzug)"
```

---

### Task 5: Werkzeuge-Destinationen — Frei (Freie Tage) + Export auf Lesesaal-Tokens (Minimal-Restyle)

**Files:**
- Modify: `internal/adapter/webui/frei.templ` (+ `frei_vm.go` falls Klassen dort), `internal/adapter/webui/export.templ`
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go` (nur falls neue pagehead/Rücklink-Keys)
- Test: `internal/adapter/httpserver/webui_dayoffs_test.go`, `internal/adapter/httpserver/webui_export_test.go`

**Interfaces:** **Kein Verhaltens-/Routen-Change** (Frei add/delete/regen-token/bundesland, ICS; Export CSV/JSON/Markdown + Preview bleiben funktional identisch). Nur Optik: `glass`/`shadow-soft`/`shadow-lift`/`rounded-3xl`/`components.Card` → Lesesaal-Panels/-Primitives (`.panel`, `.sect`, `.row`/`.projrow`, `.btn`/`.btn-q`/`.btn-pri`, Formulare an L2/L3-Editor-Konvention). pagehead (`.eyebrow`=„Zeit / Frei" bzw. „Zeit / Export", `h1`) + „‹ Zeit"-Rücklink. Ein Primär-Button pro Sicht. Export-Preview-Tabelle: `.tblwrap`+`.prose table` (L3-Bestand) oder eine benannte Panel-Tabelle statt `glass`.

**Zustände:** leer (keine freien Tage → ruhige Zeile; leere Export-Vorschau), lang (lange Feiertagsnamen/Engagement-Namen umbrechen), mobil 375px (Formular einspaltig; Export-Tabelle scrollt im `.tblwrap`), laufender Timer (unbeteiligt), Fehlerpfad (Validierung/Token-Fehler an Tokens angleichen).

- [ ] **Step 1: Bestand prüfen** — `rg -n "glass|shadow|rounded-3xl|components.Card|font-display" internal/adapter/webui/frei.templ internal/adapter/webui/export.templ`.
- [ ] **Step 2: Failing Test** — Frei/Export-Handler-Test: Fragment ohne `glass`/`components.Card`, mit `.panel`/`.row`; add/delete/preview antworten 200.
- [ ] **Step 3: Restyle** Frei + Export auf Lesesaal-Tokens + pagehead/Rücklink.
- [ ] **Step 4: Tests reparieren + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -race 2>&1 | tail
git add -A && git commit -m "feat(lesesaal): Frei + Export auf Lesesaal-Tokens (Minimal-Restyle, kein Rebuild)"
```

---

### Task 6: Asset-Fingerprinting + Cache-Control (`/static/` — PROD-Cache-Bug)

> **Anlass (PROD, 2026-07-06):** `/static/` wird ohne `Cache-Control`/`ETag` serviert — die Assets liegen in `embed.FS` (Zero-Modtime → `http.FileServerFS` setzt weder `Last-Modified` noch `ETag`), und die URLs sind stabil (`/static/app.css` etc.). Cloudflare cached CSS/JS bis zu 4h → nach jedem Deploy sieht der Browser bis zu 4h **altes** CSS/JS zu neuem HTML (Lesesaal-Bruch bis zum Ablauf). Fix: **Content-Fingerprint** in der Asset-URL + langlebiger, `immutable`-Cache. (Empfehlungsentscheid, Offene Entsch. #11 — als L4-Task, weil L4 die Base-Template-nahe Asset-Schicht ohnehin berührt.)

**Files:**
- Modify: `internal/adapter/webui/static.go` (`StaticHandler` mit `Cache-Control` + Fingerprint-Helfer `AssetVersion()`/`AssetURL(path)` über `embed.FS`)
- Modify: `internal/adapter/webui/components/base.templ` (die `<link>`/`<script>`/`<link rel=preload>`-URLs auf `AssetURL(...)`/`?v=<hash>`) + `internal/adapter/webui/components/appshell.templ` (`dialog.js`) + jede weitere templ mit `/static/`-Referenz (`rg -n "/static/" internal/adapter/webui -g '*.templ'`)
- Modify: `internal/adapter/webui/static/js/mermaid-init.js` (die selbst-injizierte `s.src='/static/vendor/mermaid.min.js'` mit Version — aus einem `data-`-Attribut/`<meta>`, sonst bleibt Mermaid unversioniert → dann NICHT `immutable` für den Vendor-Pfad)
- Test: `internal/adapter/webui/static_test.go` (Header-Assertion), `internal/adapter/webui/components/base_render_test.go` (URLs tragen `?v=`)

**Interfaces:**
- **`webui.AssetVersion() string`** — ein **beim ersten Aufruf** (sync.Once) berechneter Hash über die eingebettete `staticFS` (z. B. FNV/SHA über die sortierte Datei→Bytes-Liste; kurz, hex, ~8–12 Zeichen). Deterministisch pro Build, kein Timestamp (reproduzierbar; identisch über alle Replicas). `rg -n "//go:embed|staticFS|func StaticHandler|http.FileServerFS" internal/adapter/webui/static.go` vor dem Tippen.
- **`webui.AssetURL(path string) string`** = `"/static/" + path + "?v=" + AssetVersion()` (ein Helfer, eine Quelle; keine Version-Streuung). Optional pro-Datei-Hash statt globalem Hash — **globaler Hash genügt** (jeder Deploy bricht alle Caches gemeinsam; einfacher, kein Manifest). Trade-off im Task vermerkt.
- **`StaticHandler`**: die `embed.FS` weiter via `http.FileServerFS`, aber in einen Wrapper, der `Cache-Control: public, max-age=31536000, immutable` setzt (nur zulässig **mit** dem `?v=`-Bust — sonst kein `immutable`). CSP unberührt (`script-src 'self'` deckt query-stringed same-origin). `verify-no-popups`/htmx unberührt.
- **Kein `main.go`-Change** (StaticHandler-Mount bleibt `server.go:306`); der Hash lebt im webui-Paket.
- **CSP/Nonce-Interaktion:** die URL-Versionierung ändert keine Inline-Scripts → keine Nonce-Berührung. Der Theme-Init- + Live-Timer-Inline-`<script nonce=…>` bleibt unversioniert (Inline, kein `/static/`). `mermaid-init.js`-Selbst-Inject: entweder die Version aus `document.currentScript.dataset.v` lesen (das Init-Tag trägt `data-v={AssetVersion}`) oder den Vendor-Pfad vom `immutable`-Cache ausnehmen — Implementer wählt, im Ledger vermerkt.

**Zustände dieser Fläche:** normal (Deploy → neuer Hash → neue URLs → Cache-Miss → frisches Asset), unverändert (kein Deploy → gleicher Hash → Cache-Hit, 0 Requests), Fehlerpfad (Hash-Berechnung schlägt nie fehl bei compile-time-embed — `panic` wie der Bestand-`fs.Sub`-Fehler ist ausgeschlossen; Test deckt nicht-leeren Hash), Offline/kein-CDN (lokal-dev: `immutable` stört nicht, Hard-Reload lädt neu).

- [ ] **Step 0: rg-Verifikation** — `rg -n "//go:embed|staticFS|StaticHandler|http.FileServerFS" internal/adapter/webui/static.go` · `rg -n "/static/" internal/adapter/webui -g '*.templ' -g '*.js'` (alle Referenzen, die auf `AssetURL` müssen).
- [ ] **Step 1: Failing Test** — `static_test.go`: ein GET `/static/app.css` durch `StaticHandler()` trägt `Cache-Control: public, max-age=31536000, immutable`; `AssetVersion()` ist nicht leer und über zwei Aufrufe stabil; `AssetURL("app.css")` enthält `?v=`.
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3: `AssetVersion`/`AssetURL` + Cache-Wrapper** implementieren; `base.templ`/`appshell.templ`/weitere templ-URLs auf `AssetURL(...)`; `mermaid-init.js`-Version.
- [ ] **Step 4: Live-Verifikation** — `curl -sI https://localhost:8080/static/app.css` zeigt `Cache-Control … immutable`; die gerenderte Seite trägt `app.css?v=<hash>`; nach einer erzwungenen Asset-Änderung ändert sich der Hash (Test/lokaler Check).
- [ ] **Step 5: Bauen + Tests + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... -race 2>&1 | tail -20
git add -A && git commit -m "fix(static): Asset-Fingerprinting (?v=hash) + Cache-Control immutable — behebt 4h-Stale-CSS/JS nach Deploy (embed.FS Zero-Modtime)"
```

---

### Task 7: Wiring-Gate — Leichen-Sweep, tote i18n, volles CI, Live-Smoke, Breakpoints

**Files:** nur was der Sweep findet.

- [ ] **Step 1: Composition-Root prüfen** — `rg -n "Stats|ListSessions\b|ListSessionsRange|ListNodes|GetRunningSession|ListDocuments|ListActivity|ListDayOffs" cmd/flow-server/main.go` — **`ListSessions`** (Task 2 MRU + Task 3 All-Time-Σ, Codex-Fund #5) explizit mitprüfen; alle L4-relevanten Usecases sind bereits verdrahtet; **kein neues Server-Feld/Konstruktor** in L4 (keine Migration, kein neues Usecase; Task 6 ändert nur den StaticHandler). Falls Task 2 die Logstream-Route entfernt hat, sicherstellen, dass kein toter Handler in `server.go` registriert bleibt.
- [ ] **Step 2: Leichen-Sweep** (L4-Flächen; **Home/Zeit sind jetzt IM Scope**, anders als L3)
```bash
rg -n "glass|shadow-soft|shadow-lift|font-display|bg-gradient-to-r|from-green|to-cyan|from-|kindToneClass|swatchStyle|rowKind|rounded-3xl|rounded-2xl|components.Card|StatTile|BurndownBanner|◆|▲|●" \
  internal/adapter/webui/home.templ internal/adapter/webui/heute.templ internal/adapter/webui/woche.templ \
  internal/adapter/webui/historie.templ internal/adapter/webui/frei.templ internal/adapter/webui/export.templ --glob '!*_templ.go'
rg -n "\bDocRow\b|docRowFromDocument|swatchStyle\b|rowKind\b|BuildHomeNewest|homeNewestDocRow|activityFeedRow|handleHomeLogstream|logstreamHref|classToPrefix|logQuery|worktimeSubnav|heuteWeekRows|HeuteWeekRow|heuteBarFill|heuteBarStyle|heuteLabelClass|heuteValueClass|heuteDotClass|heuteDotGlyph|heuteDotTitle" internal/ --glob '!*_templ.go'
```
Expected: 0 Kristall-Reste auf **allen sechs L4-Flächen**. `kindToneClass` **darf** treffen (nodes.templ-Konsument, Bestand — kein Fund). Jedes verwaiste Symbol (null Konsumenten) entfernen; jedes noch-referenzierte stehen lassen (Bestand gewinnt). `BurndownBanner`/`StatTile` dürfen auf der Woche-/Statistik-Destination treffen (Metrik-Umzug, Task 4), NICHT auf Home/Zeit-Hub.
- [ ] **Step 3: Tote i18n-Keys** — für in Task 2/3 ersetzte Keys prüfen und aus BEIDEN Katalogen entfernen, wenn nirgends mehr per `T(`/`Tn(` referenziert: `stats.tileToday/Week/Month`, `home.greeting`(falls ersetzt), `activity.filter.*`, `activity.actor.all`, `heute.legendMet/Miss/Today`, `heute.balance/target/todayTotal`(falls Zeit-Hub sie nicht mehr nutzt), tote Woche/Historie-Kristall-Keys. `rg -n "\"<key>\"" internal/ --glob '!catalog_*.go'` je Kandidat; de+en-Parität bleibt.
- [ ] **Step 4: Volles CI**
```bash
make ci    # lint, verify-generate, verify-css, verify-no-popups, cover ≥75 %, build; DOCKER_HOST auf Podman-Socket
```
- [ ] **Step 5: Live-Smoke** (Dev-Stack; Cookie-Flow wie L1–L3-Gate, Bearer trifft webAuth nicht)
```bash
make dev-run &   # https://localhost:8080 (self-signed)
sleep 2
for p in / /zeit /woche /historie /dayoffs /export; do echo "$p — 200, Lesesaal rendert, kein Kristall"; done
```
Zusätzlich Asset-Cache (Task 6): `curl -sI https://localhost:8080/static/app.css | rg -i "cache-control"` zeigt `public, max-age=31536000, immutable`; die gerenderte Seite trägt `app.css?v=<hash>` (kein nacktes `/static/app.css`).
Expected: `/` zeigt Jetzt/Weiterarbeiten/Zuletzt-im-Wissen/Puls; ein laufender Timer tickt gleichzeitig in Topbar-Pill **und** Jetzt-Zeile **und** (auf `/zeit`) in der LIVE-Ledger-Zeile; Stop auf der Jetzt-Zeile stoppt den einen Timer (Pill + Content ziehen per SSE nach); `/zeit` zeigt Ledger + Wochenskala (Balken akzent/heute-grün + Soll-Zeile) + Werkzeuge (Links führen zu Export/Frei/Woche/Historie); ein neuer freier Tag zieht die Wochenskala live nach (`dayoff.changed`); Nachbuchen-/Edit-/Delete-Dialoge funktionieren; Werkzeuge-Destinationen zeigen kein Kristall. Danach Server stoppen.
- [ ] **Step 6: Breakpoint-Sichtprobe für Soenne notieren** (Abschlusstext): **≤960px** (`.narrow` volle Breite, Sektions-Köpfe brechen um) und **375px** (kein horizontales Pannen; `.weekbar` scrollt im eigenen Rahmen; `.row .right .k` weg; `.panelrow` bleibt Panel; Werkzeuge-Zeilen lesbar).
- [ ] **Step 7: Abschluss-Commit (falls der Sweep etwas fand)**
```bash
git add -A && git commit -m "chore(lesesaal): L4-Gate — Leichen-Sweep + tote Keys + Live-Smoke (Schreibtisch/Zeit)"
```

---

## Offene Entscheidungen (Soennes Wahl — mit Empfehlung + Trade-offs)

> Die Task-Texte oben sind **nach den Empfehlungen** geschrieben. Wählt Soenne anders, greifen die genannten Kollaps-/Alternativpfade. Entscheidung am Ausführungsstart.

1. **Jetzt-Panelzeile im Idle-Zustand (kein laufender Timer).** — *Empfehlung: eine ruhige `.row.panelrow` „Kein Timer läuft · heute {X} h", ohne Start-Knopf.* §10 verbietet dritte **Start**-CTAs; die Jetzt-Zeile soll informieren, nicht zum Start auffordern (Start bleibt Topbar-Pill/Cockpit). Trade-off: die Zeile ist im Idle „leer" von Aktion — bewusst. **Alternative:** die Jetzt-Sektion im Idle ganz ausblenden (noch ruhiger, aber die Fläche „springt" beim Start). Empfehlung behält die Sektion stabil sichtbar.
2. **Stop-Knopf auf der Jetzt-Zeile (Mockup zeigt „Stop").** — *Empfehlung: Stop mitnehmen, aber nur bei **gebundenem** Timer und via `hx-post="/ui/timer/stop" hx-swap="none"` (SSE-Refresh), NICHT via `hx-target="#timer-pill"`.* Verifiziert (webui_timer.go): `/ui/timer/stop` rendert das `TimerWidget`-Sheet (nicht die Pill) und braucht bei ungebundenem Running einen Node → deshalb (a) Antwort verwerfen + SSE die Optik nachziehen lassen, (b) Stop nur zeigen, wenn `Now.NodeID != ""` (ungebundener Timer wird über das Pill-Sheet mit Picker gestoppt). Stop ist kein „Start-CTA", sondern dasselbe Instrument an zweiter Stelle — kein §10-Verstoß. Trade-off: bei ungebundenem Timer fehlt der Stop auf der Jetzt-Zeile (bewusst). Die Zeit-LIVE-Ledger-Zeile bleibt anzeige-only (Mockup zeigt dort keinen Stop). **Alternative:** Jetzt-Zeile rein informativ (kein Knopf), Stop nur an Pill/Cockpit — strenger an „ein Instrument", spart die bound-only-Sonderregel, aber gegen das Mockup.
3. **Schreibtisch-Puls: Filter-Chips + Actor-Selector fallen.** — *Empfehlung: fallen lassen — der Mockup-Puls ist ein schlichter LIVE-Feed (cap 8), kein Filter-Panel; das entlastet die Seite (Soenne: „Inhalt im Vordergrund").* Live-Sync bleibt (`activity.logged` auf `#content`), die Cockpit-Puls-Sektion (subtree-gefiltert) trägt weiter die kontextuelle Sicht. Trade-off: die klassen-/akteur-basierte Aktivitätsfilterung entfällt auf Home; `handleHomeLogstream`/`logstreamHref`/`classToPrefix` sterben. **Alternative:** eine kompakte Filterzeile behalten (mehr Fläche, weniger Mockup-Treue). Bei „behalten" bleibt die Logstream-Route + Task-2-Retirement entfällt teilweise.
4. **Saldo/Burndown-Metrik (heute auf Home) — wohin?** — *Empfehlung: auf die Statistik/Woche-Destination umziehen (Task 4), nicht verlieren.* Der Lesesaal-Schreibtisch trägt keine Kacheln mehr (Mockup); die Zahl bleibt aber wertvoll → sie gehört zur „Statistik" (Werkzeug). Trade-off: ein kleiner Zusatzblock auf der Woche-Seite. **Alternative:** die Metrik ganz deferren (nur via Export/Woche-Kennzahlen sichtbar) — spart Task-4-Umzug, verliert den Glance-Wert.
5. **Statistik-Werkzeug-Ziel.** — *Empfehlung: „Statistik" → `/woche` (reichste Bestand-Stats-Seite: Mo–So + Kennzahlen + Wochensummen; nimmt in #4 die Burndown/Saldo-Metrik auf).* Trade-off: keine dedizierte „Statistik"-Seite — Woche spielt die Rolle. **Alternative:** eine schlanke neue Statistik-Seite bauen — mehr Scope, nicht L4-nötig.
6. **Historie-Auffindbarkeit ohne Sub-Tab-Strip.** — *Empfehlung: Historie als 4. Werkzeug-Zeile aufnehmen (Export · Freie Tage · Statistik · Historie).* Der Sub-Tab-Strip stirbt; Historie (vergangene Sessions bearbeiten/reassign/bulk-delete) ist eine echte Funktion und darf nicht verwaisen (Memory: Sichtbarkeit > Redundanz-Elimination). Trade-off: das Mockup zeigt nur 3 Werkzeuge — die 4. Zeile ist eine dokumentierte, auffindbarkeits-getriebene Erweiterung. **Alternative:** Historie in die „Diese Woche"-Sektion als „Frühere Wochen ›"-Link, oder ganz aus der Navigation nehmen (Deep-Link only).
7. **Werkzeuge-Destinationen: Tiefe des Umbaus.** — *Empfehlung: Minimal-Restyle auf Lesesaal-Tokens (wie der L3-Editor) — Kristall-Optik weg, Struktur/Funktion bleibt; kein voller Rebuild.* So bleibt **kein sichtbares Kristall** (Spec §17), ohne Woche/Historie/Frei/Export komplett neu zu setzen. Trade-off: die Destinationen sind Lesesaal-getönt, aber nicht -neugedacht (z. B. Historie-Kalender bleibt strukturell). **Alternative:** voller Lesesaal-Rebuild der Destinationen — deutlich mehr Scope, in einen späteren Politur-Slice (L7) verschiebbar; dann bliebe in L4 minimal Kristall auf den tiefen Werkzeug-Seiten (gegen §17).
8. **Zeit-Pagehead-Σ-Zeile (All-Time).** — *Empfehlung: owner-scoped berechnen (Σ h · Session-Count · frühestes Datum · Frei-Count); bei zu teurem Full-Scan auf „{n} Sessions · heute {X} h" vereinfachen.* Trade-off: exaktes Σ vs. billige Kurzfassung. **Alternative:** die Σ-Zeile ganz weglassen (nur h1-Datum) — am billigsten, verliert die „seit 24. April"-Erzählung.
9. **MRU-Quelle „Weiterarbeiten".** — *Empfehlung: recente bookable Knoten aus den letzten Sessions ableiten (wie die ⌘K-Palette, `webui_palette.go`-Muster) — kein neues Schema.* Trade-off: MRU = Session-basiert (nicht „zuletzt geöffnet im Browser"). **Alternative:** eine echte MRU-Tabelle (Migration) — deutlich mehr, nicht L4-nötig.
10. **Deferred-Rollup (L3-Ledger).** — *Empfehlung: „Projekte-Summary seit <Datum>" (L2-Rest) bleibt weiter deferred (Projekte-Fläche, nicht L4-Scope). `readTimeLabel`/`ReadingTime`-Konsolidierung bleibt deferred — der Schreibtisch rendert keine Lesezeit, berührt sie also nicht → gehört in L7-Politur.* Trade-off: keiner (beide außerhalb L4-Scope). **Alternative:** die `readTimeLabel`-Konsolidierung in Task 7 mitnehmen, falls der Sweep die Duplizierung ohnehin anfasst.
11. **Asset-Fingerprinting + Cache-Control (PROD-Cache-Bug, 2026-07-06).** — *Empfehlung: als **L4-Task 6** mitnehmen.* Der Bug ist real und deploy-kritisch (`/static/` ohne `Cache-Control`/`ETag`, `embed.FS`-Zero-Modtime, stabile URLs → Cloudflare liefert bis zu 4h altes CSS/JS zu neuem HTML — der Lesesaal-Umbau selbst wäre nach jedem Deploy bis zu 4h zerbrochen). Fix (`AssetVersion()`-Hash über die embedded FS → `?v=<hash>` in den Base-/AppShell-Asset-URLs + `Cache-Control: public, max-age=31536000, immutable` im StaticHandler) ist klein und liegt in der Base-Template-nahen Schicht, die L4 ohnehin berührt → co-location spart einen Extra-Slice. Trade-off: der Task ist thematisch orthogonal zum visuellen Rebuild (Infra, nicht Optik) und bläht L4 um einen Task. **Alternative:** als separater Mini-Hotfix **außerhalb** L4 (eigener Branch, sofort nach PROD) — sauberere Slice-Grenze, aber ein zusätzlicher Review-/Merge-Zyklus, und der L4-Deploy müsste auf den Hotfix warten, um nicht selbst zu stalen. Unter-Entscheidung: **globaler Hash** (ein `?v=` bricht alle Caches gemeinsam; empfohlen, kein Manifest) vs. **pro-Datei-Hash** (granularer, aber Manifest-Aufwand); und **Mermaid-Selbst-Inject** — Version via `data-v`-Attribut am Init-Tag (empfohlen) vs. Vendor-Pfad vom `immutable`-Cache ausnehmen.

---

## Self-Review-Appendix

### Grounding-Herkunft
- **Primär: First-Hand-Reads** (kanonisch): Spec §9–17 + Mockup CSS Z.20–322 + Schreibtisch Z.348–440 + Zeit Z.845–892; L3-Formatvorbild + L3-Ledger vollständig; und der echte Code: `home.templ`/`home_vm.go`/`home_newest.go`/`webui_home.go`, `heute.templ`/`heute_vm.go`/`webui_heute.go`, `woche_vm.go`/`webui_woche.go`, `activity_row.templ`/`activity_row.go` (`ActivityRowVM`, `BuildActivityRows`, `FmtRelTime`), `cockpit_main.templ` (`cockpitPulseRow` — der Lesesaal-Puls-Präzedenzfall), `components/appshell.templ` (Topbar/Timer-Pill-Mount), `components/base.templ` (Live-Tick-Skript, `[data-timer]`/`data-base`), `components/sitenav.go` (PrimaryNav/UtilityNav/AreaFor), `timerwidget.templ`/`webui_timer.go` (`TimerWidgetVM`, `/ui/timer/*`), `wissen_vm.go` (`WissenRowVM`/`WissenRowFromDocument` — Reuse-Ziel), `webui_palette.go` (MRU-aus-Sessions-Muster), `web/tailwind.css` (@layer components: welche Lesesaal-Klassen stehen — `.row`/`.projrow`/`.sect`/`.pagehead`/`.pulse`/`.livechip`/`.panel`/Avatare vorhanden; `.narrow`/`.panelrow`/`.weekbar`/`.day`/`.num`/`.dim` fehlen → Task 1), `server.go` (Routen), Retirement-Zähler (`kindToneClass` hat Konsument `nodes.templ:135` → bleibt). SSE-Event `dayoff.changed` (`add_dayoffs.go`/`delete_dayoff.go`) verifiziert.
- **Sekundär: agy-Dossier** (gemini-bigcontext) über die L4-Flächen asynchron dispatcht — **starb am Session-Limit (Reset 17:20)**, wie beide Berater im ersten Lauf. **Degradations-Modus (vermerkt):** das Grounding ist deshalb **first-hand kanonisch** (alle im Plan verwendeten Signaturen direkt am Code verifiziert, u. a. `renderTimerWidget`→`TimerWidget`, `StatsComputer.Week`=7 Tage, `ListSessions`-Signatur, `wissen_vm_test.go`-Schutztests, `EventNodeCreated`); kein Abbruch. Die Berater wurden nach dem Reset erfolgreich neu dispatcht.
- **Flow-Recall:** `flow_search_docs` (project-scope, type plan) für „Lesesaal L4" — nur L1/L2/L3-Pläne (kein neuerer Remote-Stand; das L3-Doc nennt „Next: L4 Schreibtisch + Zeit"). Lokale Dateien kanonisch.

### Spec-Deckung L4 (§17-Scope) — jeder Spec-Absatz auf einen Task gemappt
- §9 Zeile **Schreibtisch** (einspaltig 860px: Jetzt/Weiterarbeiten/Zuletzt-im-Wissen/Puls) → T1 (CSS) + T2.
- §9 Zeile **Zeit** (Tages-Ledger Von–Bis/Ziel/Dauer/LIVE → Wochenskala Panel Balken akzent/heute-grün + Soll-Zeile → Werkzeuge Export/Freie Tage/Statistik) → T1 (CSS) + T3; Werkzeuge-Ziele restyled → T4/T5. **Bewusste Spec↔Mockup-Auflösung (Codex-Fund #7):** §9 nennt „Ziel" **pro Ledger-Zeile**, das normative Mockup (Z.856–866) zeigt pro Zeile nur Von–Bis + Dauer (kein Ziel-Feld) und trägt das Tages-/Wochen-Ziel in der **Soll-Zeile** der „Diese Woche"-Sektion („Soll 40:00 · bisher 21:34"). Regel „bei Zweifel gewinnt das Mockup" → das per-Zeilen-Ziel wird bewusst in die Wochen-Soll-Zeile absorbiert; keine Ziel-Spalte pro Ledger-Zeile.
- §10 **Timer genau einer** (Topbar-Pill + Cockpit-instr-Band; keine dritten Start-CTAs) → Timer-Vorgabe-Block + T2 (Jetzt = Anzeige, Stop = ein Instrument) + T3 (LIVE-Zeile). **Puls** → T2 (`pulseRow`).
- §11 **Eindämmung** (soweit relevant: `.weekbar overflow-x:auto`, `.row .path word-break`, `.narrow` fluid) → T1 + Gate 375px. (Markdown-Eindämmung ist L3-Bestand, keine L4-Fläche.)
- §12 **Responsive** (<960 stack, <620 kompakt, weekbar gap) → T1 + Gate.
- §13 **A11y/i18n**: Fokus (L1-Bestand), `aria-hidden` an Avataren, `role="img"`+aria-label an `.weekbar` (Mockup Z.871), i18n de/en Parität (jeder Key-Step) → T1/T2/T3.
- §17 **L4-Definition** (Schreibtisch + Zeit; „L1–L4 ersetzen die sichtbare App vollständig") → alle Tasks; die Werkzeuge-Destinationen (T4/T5) erfüllen „vollständig" per Minimal-Restyle (Offene Entsch. #7). **NICHT in L4 (bewusst, Spec §17):** Kontext-Kuratierung/Meter (L5), Artefakte (L6), Dunkel-Zwilling (L7).
- **Nicht-Spec-Task T6 (Asset-Fingerprinting):** kein §-Spec-Absatz, sondern ein **PROD-Infra-Fix** (Cache-Bug 2026-07-06), per Koordinator-Auftrag als Empfehlungsentscheid in L4 co-located (Offene Entsch. #11). Berührt keine sichtbare Fläche, nur die Auslieferungsschicht → keine Design-/Mockup-Kopplung; der Mockup-Auditor prüft ihn nicht.

### Carry-forwards / Deferred — Verbleib
1. **L3-Codex-#18-Schutzregel** (`DocRow`-Kette „bleibt bis L4") → **jetzt aufgelöst** (T2 räumt sie nach rg-Beweis; `kindToneClass` bleibt wegen nodes.templ).
2. **`readTimeLabel`/`ReadingTime`-Duplikat** (L3-Rollup) → bewusst weiter deferred (Home berührt Lesezeit nicht; L7 — Offene Entsch. #10).
3. **Projekte-Summary „seit <Datum>"** (L2-Rest) → bewusst weiter deferred (Projekte-Fläche, nicht L4).
4. **inline `style=` in Sektions-Headern** (L3-Rollup) → auf L4-Flächen durch benannte Klassen ersetzt (`.sect-h .note`, `.led-when` — T1); der Rest bleibt L7.
5. **Nachbuchen-Dialog-Self-Close** → in L3 (`data-dialog-close-on-success`) gefixt; T3 erhält den Mechanismus.

### Planner-Selbstprüfung (Raster a–d, VOR den Beratern)
- **(a) Spec-Absatz ohne Task:** keiner im L4-Scope (Mapping oben); L5–L7-Absätze bewusst außerhalb.
- **(b) Zustände je Task:** leer/lang/mobil-375/laufender-Timer/Fehler in T1–T5 explizit; T6 (Assets) mit seinen Zuständen (Deploy/unverändert/Fehlerpfad); T7 ist der Gate.
- **(c) Querschnitte:** main.go-Wiring → T7 Step 1 (kein neues Usecase; T6 ändert nur den StaticHandler-Wrapper, kein main.go); SSE je Mutation → `session.*`/`document.*`/`dayoff.changed`/`activity.logged` benannt (T2 `#content`, T3 `#content`); i18n beide Kataloge → jeder Key-Step; Responsive → T1 (960/620) + Gate; Owner-Scoping → Negativtests in T2 (MRU+Wissen) + T3 (Ledger+All-Time), `u.ID` in jedem Handler-Step.
- **(d) Tests + rg-Verifikation:** jeder Task failing-Test-first; alle Bestandsnamen unter Global-Constraint „rg-Verifikation" + task-lokale Step-0-Verifikationen; „Bestand gewinnt".

### Adversariale Lückensuche — Berater-Findings + Verbleib

Beide Berater liefen gegen Spec + Mockup + Plan-Entwurf + Dossier und prüften ihre Rohfunde per `rg` gegen den echten Code: **`codex exec`** (via codex-second-opinion, 7 Findings) und **`agy`/Gemini** (via gemini-bigcontext, 9 Findings). **Prozess-Hinweis:** der erste Berater-/Dossier-Lauf starb am Session-Limit (17:20-Reset); beide Berater wurden nach dem Reset neu dispatcht und lieferten vollständig. Das parallele agy-Grounding-Dossier starb ebenfalls → das Grounding ist **first-hand kanonisch** (alle im Plan verwendeten Signaturen direkt am Code verifiziert), Degradations-Modus vermerkt. **Vor** den Beratern hat der Planner selbst 4 Rohfunde am Code verifiziert und eingearbeitet (sortedDocuments bleibt, All-Time via ListSessions, Stop-Wiring-Vorfix, worktimeSubnav-Kompilier-Sicherheit) — zwei davon (Stop-Wiring, sortedDocuments) bestätigten die Berater unabhängig.

**Zwei CRITICALs — von BEIDEN Beratern unabhängig gefunden, eingearbeitet:**
1. **[eingearbeitet — Timer-Vorgabe-Block + Task 2]** (Codex #1 = Gemini #1) Der Jetzt-Zeile-Stop war auf `hx-target="#timer-pill" hx-swap="innerHTML"` verdrahtet, aber `/ui/timer/stop`→`renderTimerWidget` rendert `webui.TimerWidget` (Sheet-Panel), NICHT `TimerPill` → das große Formular-Panel wäre in den Pill-Slot geschwappt (sichtbares Flackern), und ein ungebundener Running hätte `timer.needNode` geworfen. → **`hx-post="/ui/timer/stop" hx-swap="none"`** (Antwort verworfen, `#timer-pill`+`#content` via SSE), **Stop nur bei gebundenem Timer** (`Now.NodeID != ""`); kein `webui_timer.go`-Change nötig (Response wird verworfen). Am Code verifiziert (webui_timer.go:89–108/44–47).
2. **[eingearbeitet — Task 2 Files + Retirement-Step]** (Codex #4 = Gemini #2) Retirement von `swatchStyle`/`BuildHomeNewest` kollidiert mit den L3-Schutztests `TestSwatchStyle`/`TestDocRowUnaffected` (`wissen_vm_test.go:167–186`), die diese Symbole direkt aufrufen → sie fehlten in Task 2s Files-Liste, der rg-Zähler hätte sie als „Konsument" gemeldet, und der Retire hätte am Compile gebrochen. → `wissen_vm_test.go` in Task 2s Files aufgenommen; die zwei Schutztests werden beim Retirement mit entfernt/angepasst.

**Codex-Findings (Rest, eingearbeitet):**
3. **[eingearbeitet — Task 3]** #3 `ZeitWeekDay.Has` (=geloggt>0) hatte keine Quelle: `WocheDayVM` trägt nur Strings (`Dur`) + `Pct` (0 bei Target==0). → Wochenskala aus der **rohen** `s.Stats.Week(ctx,u.ID,time.Time{})` → `[]domain.WeekDay` (7 Tage, `Total(now) time.Duration`+`IsToday`, verifiziert stats_computer.go:158); neuer Builder `zeitWeekDays`. Kein `wocheDataFor`-Reuse (löst zugleich Gemini #3-Doppelabfrage).
4. **[eingearbeitet — Global Constraint + Task 2]** #6 MRU nannte `ListSessionsRange`, der zitierte Palette-Präzedenzfall nutzt `ListSessions.Execute(ctx,owner,since)` (andere Signatur). → auf `s.ListSessions.Execute(ctx,u.ID,since)` korrigiert.
5. **[eingearbeitet — Task 7 Step 1]** #5 Gate-Composition-Root-rg listete `ListSessions` nicht (Task 2/3 nutzen es neu) → ergänzt.
6. **[eingearbeitet — Task 3 markup + i18n]** #2 `.weekbar role="img"`+aria-label (Spec §13/Mockup Z.871) im Self-Review als abgedeckt behauptet, aber in keinem Task-Step instruiert → Task 3-Markup + Key `zeit.weekAria` + Task-1-Styleguide-Demo.
7. **[eingearbeitet — Spec-Deckung]** #7 §9 „Ziel" pro Ledger-Zeile still fallengelassen → als bewusste Spec↔Mockup-Auflösung dokumentiert (Mockup zeigt kein per-Zeilen-Ziel; absorbiert in die Wochen-Soll-Zeile).

**Gemini-Findings (Rest, eingearbeitet):**
8. **[eingearbeitet — Task 3 + Task 2]** #4 SSE-Trigger-Regressionen: Zeit verlor `project.created` (→ korrigiert auf reales `node.created`, damit der Nachbuchen-Picker nach Timer-Quick-Create nachzieht); Home verlor `settings.changed` (→ behalten, Bestand-Parität, als bewusst dokumentiert).
9. **[eingearbeitet — Task 3]** #3 Wochen-Doppelabfrage + verwaiste `heuteWeekRows`/`HeuteWeekRow` + 6 Streifen-Helfer nach dem Mo–Fr→7-Tage-Umstieg → Task 3 baut aus `Stats.Week` (kein zweiter `wocheDataFor`-Aufruf) und **retiriert die verwaiste Kette**; Task-7-Sweep-Pattern um diese Symbole erweitert.
10. **[eingearbeitet — Task 1 Responsive]** #5 `.pagehead h1{font-size:27px}` (<620px, Mockup Z.314) fehlte (nur 34px definiert; betrifft beide neuen Seiten) → im 620px-Block ergänzt.
11. **[eingearbeitet — Task 2]** #6 „Zuletzt im Wissen"-`.k` (Mockup Z.401) fehlt + `WissenRowVM` hat kein `.k`-Feld → bewusst OHNE `.k` (nur `.right .v`=TimeStr), konsistent mit der L3-`/wissen`-Zeile, ohne den geteilten Typ zu ändern; dokumentiert.
12. **[eingearbeitet — Task 3/Task 2 i18n]** #7 Multi-Platzhalter-EN nur „gleichwertig" → explizite EN-Strings für `zeit.allTimeSub` (4 Platzhalter, feste Reihenfolge) + `home.deskSub`, mit Notiz dass `TestCatalogsParity` nur Key-Existenz prüft.
13. **[eingearbeitet — Task 4]** #8 Historie ohne `dayoff.changed`-Trigger (Kalender-Frei-Markierung zieht nicht nach) → Task 4 ergänzt `dayoff.changed` + `node.created`; Task-7-Smoke prüft es.
14. **[eingearbeitet — Task 3]** #9 laufende Ledger-Zeile: Mockup „14:32 – läuft" vs. Bestand-`fmtClockRange` „–…" → laufende Zeile zeigt „{Start} – {heute.running}".

**Non-Findings (von beiden Beratern explizit als sauber bestätigt, kein Plan-Change):** zwei gleichzeitige `data-timer`-Ticker (base.templ ist Multi-Instanz-fähig); `worktimeSubnav`-Zwischenzustand nach Task 3 kompiliert (3 Aufrufer bleiben); `WissenRowFromDocument(d,now)`-Signatur + `WissenRowVM`-Felder exakt; `cockpitPulseRow` genau ein Aufrufer (Promotion zu `pulseRow` Cockpit-sicher); `dayoff.changed` als reales Event; keine erfundenen/falsch signierten Symbole im Plan.

**Dissens:** keiner — die Berater überschnitten sich auf beiden CRITICALs ohne Widerspruch und waren sonst komplementär (Codex tiefer bei Retirement-/Signatur-Kanten, Gemini breiter bei SSE-Triggern/Responsive/Mockup-Zeilen-Details). Alle Sichten eingearbeitet.
