# flow rebuild M3 — TUI Design-System + Sidekick-Shell (Design Spec / Übersicht)

**Date:** 2026-06-16
**Milestone:** M3 — die erste *Surface-Design*-Vertikale des Rebuilds (nach M0 Spine, M1 Worktime a–e, M2 Kompendium a–e).
**Status:** approved (Brainstorm abgeschlossen), bereit für Slice-Pläne.
**Scope-Schnitt:** Diese Spec ist die **Übersicht** für M3. Ausführung in Slices **M3a–M3d** (je eigener Plan, ggf. eigene Detail-Spec). **WebUI-Design ist M4**, nicht Teil von M3.

## Problem / Motivation

Der Rebuild hat zwei funktionale Vertikalen (Worktime, Kompendium) über REST/WebUI/TUI — aber **kein zusammenhängendes TUI-Design**. Die Screens entstanden je als Done-Gate ihrer Slice und sind lose Einzel-Programme: nur `flow worktime` und `flow docs` starten je ein eigenes `tea.NewProgram`, es gibt **keine App-Shell, keine geteilte Navigation, keine gemeinsame Design-Sprache**. Der grobe `█/░`-Balken in `internal/tui/stats.go` ist symptomatisch.

Entscheidend: Auf `main` (Pre-Rebuild) existierte bereits ein **reifes, A11y-bewusstes, skill-getriebenes TUI-Design-System** — `theme.Palette`/`Sem()`-Farben, `components/glyphs` (Whitelist), `statusbar.BarColored` (schwellen-farbiger `▰▱`-Balken), `toast`, `help`, `confirm`, `form`, `picker`, `markdown_overlay` (Suche + Code-Copy), `screen/worktime` (Today/History inkl. Contribution-**Heatmap**). Der Orphan-Neustart in M0 hat das verworfen. **M3 belebt dieses Design wieder** (statt neu zu erfinden) und legt die in M0 verlorene **Navigation** als neue Sidekick-Shell darüber.

## Locked Decisions (aus dem Brainstorm)

1. **Surface-Reihenfolge:** M3 = Design-System + **TUI**. **WebUI = M4** (gleiches System, später).
2. **Struktur:** **sidekick-style App-Shell** — *ein* `flow`-TUI-Programm, alle Screens unter einer Root-Model mit persistenter Navigation.
3. **Tabs:** **Home-Dashboard** (Landing) + Worktime · Docs · Stats · DayOffs · Export · Projects.
4. **Navigations-Layout:** **Top-Tab-Strip + `:`-Command-Palette** (Mockup-Variante C). Sidebar verworfen (frisst Breite im tmux-Panel).
5. **Shell-Architektur:** **Navigations-Stack/Router** mit **pro-Tab-Stacks** (jeder Tab eigene Back-History; Docs-Drill-down `[[wikilink]]`→Backlink pusht/poppt). Bewusst gewählt trotz Over-Engineering-Flag — passt zur bestehenden In-TUI-Wikilink-Navigation (M2b) und zur Palette-als-Router-Idee.
6. **Design-Sprache:** das **alte `main`-Design-System wiederbeleben** (es kodiert bereits die `tui-usability`-Prinzipien: Tokyonight-Night-Semantik, Glyph-Whitelist, Spacing-Skala, A11y „nie Farbe allein", No-Monoliths). **Kein Design-from-scratch.**
7. **Komponenten-Bezug:** `bubbles-v2` für echte stateful-Widgets (`viewport`, `list`, `textinput`, `help`, `spinner`); **custom** fürs Chrome (`header`, `tabstrip`, `statusbar/keyhint`, `breadcrumb`, `overlay`, `toast`) + den portierten **`BarColored`** + den Markdown-Renderer.
8. **Fortschrittsbalken:** der alte **`statusbar.BarColored`** (`▰▱`, schwellen-farbig: cyan laufend / grün am Ziel / rot über Soll) — **nicht** der grobe `█/░`-Balken.
9. **Markdown-Rendering (TUI):** **custom goldmark→ANSI** mit `ui`-Tokens, **inline-interaktive Wikilinks** (`→ gültig` / `⊘ kaputt` + In-TUI-Navigation) + **OSC-8-Weblinks**, optional **chroma** fürs Code-Highlighting; gehostet in einem `bubbles-v2 viewport`. `markdown_overlay`s bisheriger **glamour-Renderer wird durch diesen ersetzt** (Suche + Code-Copy bleiben). glamour wird **nicht** eingeführt (Theming-Ceiling + Wikilink-Bruch).
10. **Today-Optimierungen:** originalgetreu wiederbeleben **+** „noch/ETA" prominenter (beim laufenden Tag die wichtigste Info). Shell-Einbettung ist inhärent. **Week-Pace-Strip auf Today: vorerst zurückgestellt** (im Brainstorm an-/abgewählt).

## Architektur

```
cmd/flow/ui.go         flow ui   (oder `flow` ohne Args)        → Shell (alle Tabs)
                       flow worktime|docs|…                     → Standalone-Host (1 Route, kein Chrome)

internal/tui/
  theme/               ← portiert von main: Palette + Sem()-Farben (Tokyonight-Night), Spacing-Skala
  ui/                  ← portiert von main (components/*): glyphs, statusbar(BarColored), toast,
                         help, confirm, form, picker, strings  + NEU: header, tabstrip, breadcrumb, overlay
  markdown/            ← NEU: custom goldmark→ANSI Renderer (ui-Tokens, Wikilinks, OSC-8, chroma-Code)
  shell/
    shell.go           Root-Model: Router + Tabstrip + Status/Keyhint-Bar + Palette-Layer
    router.go          Nav-Stack: pro-Tab []Route, push/pop/replace, Overlay-Layer
    route.go           Route-Interface
    palette.go         :-Command-Palette (Fuzzy über Routen + Aktionen)
  home/ worktime/ docs/ stats/ dayoffs/ export/ projects/      ← je Screen ein Route-Package
```

**Route-Interface** (jeder Screen, jedes Drill-down, jedes Modal):
```go
type Route interface {
    Init() tea.Cmd
    Update(tea.Msg) (Route, tea.Cmd)   // Selbst-Typ → Stack-Ersatz möglich
    View(Frame) string                 // Frame = nutzbare Größe (Chrome abgezogen)
    Title() string                     // Tab-Label / Breadcrumb
    KeyHints() []ui.KeyHint            // kontextuelle Footer-Hints (max 4; Rest im ?-Overlay)
}
```

**Navigationsmodell:** Tab-Strip wechselt den **aktiven Stack**; Drill-down **pusht** auf den Stack des aktiven Tabs (`esc` poppt mit Back-History); `:`-Palette pusht/ersetzt Routen oder feuert Aktionen (global); Modals (Help, Dialoge, Suche) sind Routen auf einem **Overlay-Layer** über dem aktiven Stack.

**Message-Routing:** Shell fängt globale Keys (Tab/⇧Tab, `1–7`, `:`, `?`, `q`) + **SSE-`ChangedMsg`-Broadcast** an alle Stacks ab; alles andere geht an die **oberste Route des aktiven Stacks**. Saubere Trennung Chrome ↔ Inhalt. Der **Router** ist die zentrale, gründlich getestete Einheit; der Rest delegiert.

## Design-System (M3a)

**`theme`** — semantische Farben über Tokyonight-Night (`Sem().Accent/Active/Success/Notice/Danger/Info/Border/FgMuted` …), Spacing-Skala `{0,1,2,4}`. Kein Hex-Hardcoding in Screens.

**Glyph-Whitelist** (aus `glyphs.go`): `▶` active · `‖` paused · `✓` done · `■` stopped · `▰▱` Balken · `›` info · `·` bullet · `▎` accent/focus · `●○` filled/empty · `◆` project · `★` holiday · `☼` vacation · `░▒▓█` heat. **Keine Emoji-Pictogramme** ([[feedback_no_icons]]).

**Komponenten-Inventar:**

| Komponente | Bezug | Zweck |
|---|---|---|
| `glyphs` | port | Whitelist-Vokabular |
| `statusbar.BarColored` | port | `▰▱` schwellen-farbiger Fortschrittsbalken |
| `toast` | port | `✓` Success / `›` Info, transient |
| `help` | bubbles-v2 + port-Wrapper | `?`-Overlay volle Keymap |
| `confirm` | port | destruktive Bestätigung |
| `form`/`textinput` | bubbles-v2 + port-Wrapper | Dialog-Inputs |
| `picker` | port | Section-Header + selektierbare Rows |
| `list` | bubbles-v2 | scroll-/selektierbare Listen |
| `viewport` | bubbles-v2 | Scroll-Container (Docs-Body, Overlays) |
| `spinner` | bubbles-v2 | API-Lade-Indikator |
| `header` | **neu** | App-Titel · User · Kontext |
| `tabstrip` | **neu** | Tabs aktiv/inaktiv, **Overflow** bei schmalem Panel |
| `statusbar/keyhint` | **neu** | `KeyHints()` der aktiven Route |
| `breadcrumb` | **neu** | Drill-down-Tiefe |
| `overlay` | **neu** (lipgloss-v2 compositing) | zentrierte Box + gedimmter Backdrop |

## Home-Dashboard (M3c)

Inhalt: laufende Session (Threshold-Farbe) · **Tagesziel** (`BarColored`) · Woche · letzte Docs · Projekte. **Layout responsiv:** zwei-spaltig (links Arbeit, rechts Wissen) wenn breit genug, sonst automatisch gestapelt (eine Spalte) als Fallback fürs schmale tmux-Panel.

## Worktime-Today (M3c)

Originalgetreu aus `today_render.go` portiert: Datumszeile · Headline (Total threshold-farbig + Status-Pille `▶ läuft`/`✓ Ziel erreicht` bold + `Ziel N%`) · `BarColored` · Summary (`Ziel · noch · ETA`) · Note-Chip · Sessions-Liste mit Pause-Trennern + Cursor-`▎` · Footer (max 4 Hints). **Tweak:** „noch/ETA" prominenter (beim laufenden Tag wichtigste Info). Daten via `apiclient.GetToday` + Live-Update via SSE-`ChangedMsg`.

## Migration / Porting-Strategie

- Gleicher Stack (`charm.land/{bubbletea,bubbles,lipgloss}/v2`) → tech-kompatibel.
- `theme` + `components` von `main` **präsentations-pur** → weitgehend 1:1 portieren **inkl. bestehender Unit-Tests** (→ `internal/tui/theme`, `internal/tui/ui`).
- `markdown_overlay`: glamour-RenderFunc → neuer custom **goldmark→ANSI**-Renderer (Suche/Code-Copy bleiben).
- Screens: Render-Layer portieren, **Daten auf Rebuild-`apiclient` + SSE umklemmen** (nicht alte Stores). Shell hostet sie als Routes.
- Shell (Router/Tabstrip/Palette/Status): **neu** auf den portierten Komponenten.

## Testing & Done-Gate

- **Unit-Tests** der Komponenten aus `main` mitportieren; **Router-Tests** (push/pop, pro-Tab-Isolation, Back-History, Overlay-Layer) + **Render-Golden-Tests** fürs Chrome (tabstrip-Overflow, statusbar, overlay-Compositing).
- **Done-Gate (live):** `flow ui` gegen den Dev-Stack — durch Home/Worktime/… tabben, `:`-Palette-Sprung, Drill-down + `esc`-Back, **SSE-Live-Update** im Today sichtbar; Standalone-`flow worktime` läuft weiter; `make ci` grün, Coverage ≥ 80 %.

## Slice-Schnitt (je eigener Plan)

| Slice | Inhalt | Done-Gate-Kern |
|---|---|---|
| **M3a** | Design-System-Revival: `theme` + `ui`-Komponenten (glyphs, BarColored, toast, help, confirm, form, picker, strings) portieren + Tests. Keine Screen-Änderung. | Komponenten kompilieren + Tests grün; ein winziges Demo/Golden-Render. |
| **M3b** | Sidekick-Shell: Router/Nav-Stack + `route.go` + `tabstrip` + `:`-Palette + globaler Status/Footer; `flow ui`-Entry + Standalone-Host. Hostet eine Placeholder-/Home-Route. | `flow ui` tabbt + Palette + Drill-down/Back; Router-Tests grün. |
| **M3c** | Home-Dashboard (responsiv) + Worktime-**Today** (originalgetreu + ETA-Tweak) auf apiclient+SSE als erste echte Routes. | Today live gegen Dev-Stack inkl. SSE-Update; Home zeigt echten Tagesstand. |
| **M3d** | Restliche Screens als Routes: Docs (+ custom **Markdown-Viewer** goldmark→ANSI/Wikilinks/OSC-8 im viewport + markdown_overlay), Stats (+**Heatmap**/Burndown), DayOffs, Export, Projects. | Alle Tabs live navigierbar; Docs-Wikilink-Drill-down + Back; Markdown gerendert. |

WebUI bleibt **M4** (separate Brainstorm→Spec→Plan-Runde, gleiches Design-System).

## Referenzen

[[feedback_no_icons]] · [[reference_soenne_worktime_workflow]] (TUI-primary) · [[reference_flow_launch_modes]] (sidekick vs. standalone) · [[feedback_navigation_discoverability_over_minimalism]] · [[project_charm_v2_migration]] (v2-Stack) · [[reference_charm_v2_api_skills]] (bubbletea-v2/bubbles-v2/lipgloss-v2-Skills) · `tui-usability`-Skill (kanonische Semantik) · [[feedback_plan_main_wiring_task]] · [[feedback_long_lived_integration_branch]] (`rebuild` bleibt Integrations-Branch).
