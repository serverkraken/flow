# Markdown-Overlay-Component — Design

Status: implemented, 2026-05-12 (Component live unter `internal/frontend/tui/components/markdown_overlay/`; Caller: worktime brief_view, today_note_view, kompendium browse)
Review-Follow-up: F4 (Component-Lift)

## Problem

Drei parallele Implementierungen desselben Konzepts — Markdown in einem
viewport mit close-keys, scroll, und etwas Chrome — sind im Tree
auseinandergewachsen:

| File | LOC | Form | Chrome | Features |
| --- | --- | --- | --- | --- |
| `internal/frontend/tui/screen/worktime/brief_view.go` | 134 | Struct + Methoden, Sub-State im `heute`-Model | `titlebox` + Footer-Hint | viewport, scroll-percent, resize |
| `internal/frontend/tui/screen/worktime/today_note_view.go` | 150 | Methoden auf `heute`, State direkt embedded | wie brief_view | wie brief_view + initial-load-err |
| `internal/kompendium/frontend/tui/view/model.go` (+ copy.go + styles.go + keymap.go + markdown_adapter.go) | 528+ | eigene `tea.Model` | rounded `frameStyle` + title + sep + body + footer + statusBar | viewport, Search-Mode, Match-Highlight, Code-Copy, Frontmatter-Card, Backlinks |

F1 hat den letzten direkten Behavior-Drift entschärft (today_note re-rendert
nach tmux-Pane-Resize), aber strukturell wird der nächste Drift kommen.

## Ziel

Eine gemeinsame Komponente
`internal/frontend/tui/components/markdown_overlay/` mit unified Chrome
und allen heute generischen Features. Drei Migrationen, ein zentraler
Code-Pfad.

## Scope-Entscheidungen

| Feature | Wo | Begründung |
| --- | --- | --- |
| viewport, scroll, resize, close-keys, scroll-percent | Base | Universell |
| **Search-Mode** (`/`, textinput, match-highlight, n/N cycling) | Base | Brief/Today sind lang genug, dass Suche real nützlich ist |
| **StatusBar** (mode-badge, title-path, scroll-percent, match-counter, copy-status) | Base | Standard-Markdown-Reader-UX |
| **Code-Copy** (`c`, OSC52 + local clipboard fallback) | Base | Notes & Briefs enthalten häufig Code |
| Frontmatter-Card | Caller-Markdown-Options | Kompendium-Content-Concern, schon heute `markdown.WithFrontmatter` |
| Backlinks-Footer | Caller-Markdown-Options | Kompendium-Content-Concern, schon heute `markdown.WithBacklinks` |
| Wikilink-Resolver | Caller-Markdown-Options | dito |

Chrome-Shape: unified, alle drei adoptieren das aktuelle Kompendium-
Shape (rounded frame + title-row + separator + body + footer +
statusBar). Brief/Today bekommen damit eine sichtbare UI-Änderung im
Worktime-Screen — von titlebox auf rounded frame mit StatusBar. Diese
Änderung ist explizit gewollt (UX-Konsistenz mit Kompendium-View, und
Status-Bar gibt Brief/Today erst die Möglichkeit, Match-Counter und
Scroll-Percent dauerhaft zu zeigen).

Form-Faktor: `tea.Model` (Init/Update/View). Brief/Today migrieren von
Sub-State-Methoden auf "embed-and-delegate" — `heute` hostet ein
`overlay markdown_overlay.Model`-Feld und routet WindowSizeMsg/KeyMsg
durch.

## Public API

```go
package markdown_overlay

// Model ist die markdown_overlay-Komponente. Implementiert tea.Model.
type Model struct { /* unexported */ }

// New baut eine neue Model. Renderer ist required; ohne ihn würde der
// Overlay nur den raw markdown anzeigen, und das wäre ein Bug, kein
// fail-soft. Alle anderen Optionen haben sinnvolle Defaults.
func New(renderer ports.MarkdownRenderer, opts ...Option) Model

// Option konfiguriert die Komponente beim Bauen. Composable.
type Option func(*config)

func WithTitle(title string) Option
func WithSource(src string) Option            // markdown body
func WithMarkdownOptions(...markdown.Option) Option  // frontmatter, backlinks, wikilinks
func WithSearch() Option                      // enable `/` key + search-mode
func WithCodeCopy() Option                    // enable `c` key + snippet cycling
func WithCloseKeys(keys ...string) Option     // default: q / esc / b
func WithFooterExtras(hints ...string) Option // appended to footer hint row

// In-place setters (return updated Model — bubbletea-immutable-style).
func (m Model) SetSize(w, h int) Model
func (m Model) SetTitle(title string) Model
func (m Model) SetSource(src string) Model
// SetError zeigt eine Fehlerzeile statt body (für initial-load-err
// in today_note_view).
func (m Model) SetError(err error) Model

// tea.Model contract.
func (m Model) Init() tea.Cmd
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd)
func (m Model) View() string

// ExitMsg wird vom Overlay emitted, wenn der User mit einem
// CloseKey schließt. Host konsumiert das und switched seinen Mode.
type ExitMsg struct{}

// Inspection-API (für Host-StatusBar / Tests).
func (m Model) CurrentMode() Mode
func (m Model) Query() string
func (m Model) Matches() []int
func (m Model) MatchIndex() int
```

## File-Layout (No-Monoliths-Regel)

```
internal/frontend/tui/components/markdown_overlay/
    doc.go              # Package-Doc
    model.go            # Model struct + New + tea.Model contract dispatcher
    options.go          # Option type + With*-Funktionen + config struct
    setters.go          # SetSize / SetTitle / SetSource / SetError
    chrome.go           # frame + title + sep + footer + statusBar rendering
    chrome_styles.go    # lipgloss styles (frame, title, sep, footer, statusBar, match-bar)
    search.go           # search mode state + key handling + match scan + highlight
    code_copy.go        # snippet extraction + cycling + OSC52 + local pbcopy fallback
    keymap.go           # KeyMap + defaultKeys()
    snapshots.go        # golden-file helpers (test-only)
```

Erwarteter Total-LOC: 600-700 (gegenüber heute 134 + 150 + 528 = 812
über drei Files mit redundanten Snippets).

## Migration

### Schritt 1 — Base bauen
TDD: Golden-File-Tests für jede Capability:
- Render small body (no scroll)
- Render big body (scroll-percent visible)
- Narrow width (chrome budgets)
- Search-Mode active, query with matches
- Search-Mode active, query without matches
- Code-Copy after `c` press (status bar updated)
- Initial error (SetError before SetSource)

Tests laufen gegen golden files in `testdata/`.

### Schritt 2 — kompendium-view ablösen
- `internal/kompendium/frontend/tui/view/` migriert von der eigenen
  Model auf `markdown_overlay.New(renderer, WithSearch(), WithCodeCopy(), WithMarkdownOptions(...))`
- browse routet `tea.Msg` weiter an den Overlay; ExitMsg → Mode-Switch
  zurück zu Browse-Liste.
- `view/model.go`, `view/copy.go`, `view/markdown_adapter.go`,
  `view/styles.go`, `view/keymap.go` werden gelöscht; nur ein dünner
  Constructor-Helper bleibt (`view.NewForNote(...)` oder das Wiring
  wandert direkt nach browse).
- bestehende kompendium-view-Tests (`model_test.go`, `copy_internal_test.go`)
  werden entweder in die markdown_overlay-Tests gezogen oder als
  Integration-Tests gegen den Overlay-Constructor angepasst.

### Schritt 3 — worktime brief_view ablösen
- `heute.briefView` von eigenem Struct auf `*markdown_overlay.Model`-
  Pointer wechseln (Pointer, weil mode-switched-on-active und nil-
  Test-Pattern).
- `newBriefView` baut `markdown_overlay.New(deps.MarkdownRenderer, WithTitle(title), WithSource(body), WithSearch(), WithCodeCopy())`
- `briefView.updateKey/resize/view` entfallen — heute delegated direkt.
- ExitMsg statt "ok bool" als Schließ-Signal.

### Schritt 4 — worktime today_note_view ablösen
- analog zu brief_view; State in heute reduziert sich auf `noteViewID` +
  `*markdown_overlay.Model`.
- F1-Resize-Test wandert mit auf den Overlay (Resize-Path ist jetzt
  generisch).
- SetError-Pfad wird benutzt für die `noteViewErr`-Anzeige.

### Schritt 5 — Aufräumen
- alte Sub-State-Felder aus heute löschen (`noteViewBody`, `noteViewVP`,
  `noteViewReady`, `briefView*`)
- `screenBaseline`-Entries in `lint/screen_baseline_test.go` für
  brief_view.go / today_note_view.go anpassen (Dateien werden klein →
  Baseline runter, evtl. ganz aus der Map).
- Snapshot der `make ci`-Coverage-Drift dokumentieren.

## Risiken & Mitigationen

| Risiko | Mitigation |
| --- | --- |
| Chrome-Wechsel bricht Worktime-Screen-Tests | Tests vor Migration listen + erwartete Diffs in Commit-Body dokumentieren. F1-Test bleibt grün (Resize ist generisch). |
| StatusBar zu breit für schmale Worktime-Pane (≤80 Spalten) | Bestehender truncate-Pfad in kompendium-view greift schon ab 5-Spalten-Avail. Test mit `width=40` fixture. |
| Kompendium-view-Tests vergleichen ANSI-Output Zeichen-genau | Migration einzeln per Sub-Commit, Snapshot-Diff im PR sichtbar. |
| Eingehende ExitMsg-Cmd verschluckt der Worktime-Host vs der Browse-Host | Konvention: jeder Host muss `markdown_overlay.ExitMsg` in seinem Top-Level-Update behandeln. Doc-Comment am `ExitMsg`. |

## Was draußen bleibt (YAGNI)

- Multi-Document-Navigation (forward/back-Stack) — kein Caller hat das
  heute, kann später als Option dazu
- Konfigurierbare Chrome (titlebox vs frame) — nur ein Chrome
- Markdown-Render-Caching — ist Renderer-Layer-Concern
- Wikilink-Resolver / Frontmatter / Backlinks — bleiben markdown.Option,
  reisen mit `WithMarkdownOptions(...)` durch
- Mausunterstützung — keine der drei Overlays hat heute Maus-Handling

## Open Questions

Nichts mehr offen — Scope (alle drei), Form (tea.Model), Chrome
(unified) und Naming (`markdown_overlay`) sind entschieden.

## Nächste Schritte

Nach Spec-Review: `writing-plans`-Skill für detaillierten
Implementation-Plan mit File-Level-Tasks.
