# Design System — Audit + Plan

**Datum:** 2026-05-03
**Scope:** `internal/frontend/tui/components/`, `internal/frontend/tui/markdown/theme/`, `internal/domain/status.go`, alle Screens unter `internal/frontend/tui/screen/`, `internal/kompendium/frontend/tui/`.

Dieses Dokument hat zwei Hälften:

1. **Audit** — was heute existiert und wo es hakt.
2. **Plan** — eine klare Designlinie, vollständige Komponenten-Specs auf bubbletea + lipgloss, ein Token-System, und ein Accessibility-Gerüst, das die ganze TUI mitträgt.

---

## Teil 1 — Audit

### 1.1 Summary

| | |
|---|---|
| Komponenten-Pakete reviewt | 10 (`confirm`, `form`, `help`, `picker`, `spinner`, `statusbar`, `theme`, `titlebox`, `toast`, `viewport`) |
| Token-Quellen | **3 parallele** (`components/theme`, `markdown/theme`, `domain.StatusPalette`) |
| Hartkodierte Hex-Werte in Nicht-Theme-Dateien | 1 (`internal/domain/status.go` — `DefaultStatusPalette`) |
| Inline `lipgloss.NewStyle()` in Screens | 98 (über 8 Dateien) |
| Component-Helper-Aufrufe in Screens | 44 |
| Score | **62 / 100** |

### 1.2 Token-Quellen — drei „Tokyonight" gleichzeitig

| Quelle | Bg | Fg | Anzahl Felder | Mechanik |
|---|---|---|---|---|
| `components/theme.fallback` | `#24283b` (Storm) | `#c0caf5` | 11 | Struct + tmux-`@tn_*`-Lookup |
| `markdown/theme.Tokyonight` | `#1a1b26` (Night) | `#c0caf5` | 22 | Struct + Package-Level-Globals + `SetActive` |
| `domain.DefaultStatusPalette` | — | — | 5 | Plain Hex |

Resultat: ein und dasselbe „Tokyonight" rendert mit zwei verschiedenen Hintergründen, je nach Surface. Cheatsheet-Box (Storm) und Markdown-Renderer (Night) sitzen im selben Frame nebeneinander und schauen sich quer ins Auge — `markdown/theme.BgHighlightSoft` ist exakt `components/theme.Bg`.

### 1.3 Naming- und Konsumptionsdrift

- Drei Namen für „gedimmt" (`Dim`, `FgDim`, `Muted`); das Markdown-Theme hat sogar zwei Stufen, das Component-Theme nur eine.
- Mischung aus semantischen (`Accent`, `Border`) und Hue-Tokens (`Blue`, `Cyan`, …) ohne klare Regel.
- Components: `Palette` per Wert injiziert (testbar, threadsafe). Markdown: Package-Level-Globals + `SetActive` (mutierbar, nicht parallel-test-freundlich) — selbst in CLAUDE-kompendium-plan §K3.E als „deferred" markiert.
- 98 Inline-`lipgloss.NewStyle()`-Stellen in Screens, nur 1 lokaler Helper (`stDim`/`stErr` in `worktime/today.go`), den die anderen Screens nicht kennen.

### 1.4 Komponenten-Vollständigkeit

| Komponente | Variants | States | Doku | Score |
|---|---|---|---|---|
| `titlebox` | 1 | 1 | inline godoc | 8/10 |
| `picker.Row` | selected/unselected | – | inline godoc | 7/10 |
| `picker.SectionHeader` | 1 | – | inline godoc | 7/10 |
| `statusbar.Hints` | 1 | – | inline godoc | 7/10 |
| `statusbar.Bar` | 1 | – | inline godoc | 7/10 |
| `spinner` | 1 | running/static | inline godoc | 7/10 |
| `toast` | **nur Success (`✓` grün)** | visible/dismissed | gut dokumentiert | **5/10** |
| `confirm` | 1 | open/answered | gut dokumentiert | 7/10 |
| `help` | 1 | – | inline godoc | 7/10 |
| `form.NewTextInput` | 1 (`CharLimit=80` magic) | default/focused | thin | 6/10 |
| `form.ChoiceModel` | 1 | – | inline godoc | 7/10 |
| `theme.Pill` | **5 hartkodierte States** | – | inline godoc | **5/10** |

Klaffende Lücken: kein Modal-Component (in `kompendium/browse/styles.go` mit DoubleBorder-Hand-Roll), kein Tabs-Component (Worktime baut die Tab-Strip selbst), keine Card-Komponente, kein Toast-Variant für Fehler/Warnung.

### 1.5 Accessibility-Risiken (heute)

- Keine systematische Kontrast-Prüfung. `H6: Faint().Italic()` auf `Muted` (`#565f89`) auf `Bg` (`#1a1b26`) liegt unter WCAG AA 4,5:1.
- Mehrere Stellen mit Color-only-Signaling (Statuspille rot/grün ohne Glyph-Unterschied).
- `Faint()` als zusätzliches Stilmittel auf bereits gedimmtem Foreground.

### 1.6 Dokumentation

CLAUDE.md beschreibt Architektur exzellent — aber kein Token-Inventar, kein Komponenten-Index, keine Glyph-Whitelist, keine kanonischen UI-Strings.

---

## Teil 2 — Plan

Das Ziel ist eine TUI mit **einer einzigen, ruhig dichten Designlinie**, in der jede Komponente vollständig auf bubbletea + lipgloss-Mustern aufsetzt, alle Farben aus einer Tokenquelle kommen und Accessibility nicht ein nachgeschalteter Lint, sondern Teil der API ist.

### 2.1 Designlinie

**Calm density.** flow ist ein Sidekick — viel Information auf wenig Raum, ohne Lärm. Vier Prinzipien:

1. **Eine Bühne.** Tokyonight Night (`#1a1b26`) ist kanonisch. Storm wird verworfen. Catppuccin Mocha bleibt als optionales Theme. Beide Surfaces (Cheatsheet + Markdown) wachsen aus demselben Bg.
2. **Hierarchie über Border, nicht Farbe.** Rounded für Hauptinhalt, Normal für Sub-Panels, Double für Modale. Farbe transportiert Status (Success/Warning/Danger), nicht Wichtigkeit.
3. **Selection ist ein Glyph.** Die Akzentbar `▎` ist die universelle Selection-Signatur — Picker-Zeile, Tab, fokussiertes Form-Feld. Bold-Foreground unterstützt, nie ersetzt sie.
4. **Glyph + Farbe, niemals Farbe allein.** Status-Pills, Tasks, Pace-Dots tragen einen monospace-Glyph mit, sodass Farbblindheit oder NO_COLOR den Inhalt nie verschluckt.

**Glyph-Whitelist** (kanonisch, im Repo dokumentiert):

| Glyph | Bedeutung |
|---|---|
| `▶` | aktive Session / läuft |
| `■` | gestoppt |
| `‖` | pausiert |
| `✓` | erfolgreich / erledigt |
| `✗` | gescheitert |
| `▲` | Anstieg / Streak |
| `▼` | Rückgang |
| `●` | gefüllter Punkt (heute, Ziel erreicht) |
| `○` | leerer Punkt (Ziel verfehlt) |
| `★` | Feiertag / Urlaub |
| `☼` | Frei-Sondermarker |
| `✚` | Zusatztermin |
| `▎` | Akzentbar / Selection |
| `▰ ▱` | Progressbar gefüllt / leer |
| `─ │ ╭ ╮ ╰ ╯` | Box-Drawing |

Keine Emojis (CLAUDE.md §Conventions bestätigt das, aber listet die Whitelist nicht). Keine Halffill-Glyphen wie `◐` — sie rendern in einigen Fonts mit Emoji-Width und brechen die Spaltenausrichtung.

**Kanonische Tastaturen** (jede Komponente hält sich daran):

- `j/k` oder `↓/↑` — Navigation
- `Enter` — bestätigen / öffnen
- `Esc` — abbrechen / zurück
- `y/n` — ja/nein in Confirm-Dialogen
- `/` — Suche / Filter starten
- `?` — Hilfe-Overlay
- `Tab` / `Shift+Tab` — Form-Field-Navigation
- `q` / `Ctrl+C` — Beenden

**Kanonische deutsche UI-Strings** (Konstanten in `internal/frontend/tui/components/strings`):

```go
const (
    HintConfirm       = "y/Enter → ja  ·  n/Esc → nein"
    HintCancel        = "Esc → abbrechen"
    HintFilter        = "/ → suchen"
    HintHelp          = "? → Hilfe"
    LabelLoading      = "lädt …"
    LabelEmpty        = "Keine Treffer."
    LabelError        = "Fehler:"
)
```

### 2.2 Token-System

Eine einzige Quelle: `internal/frontend/tui/theme/`. `components/theme` und `markdown/theme` werden zu dünnen Konsumenten, die genau dieselbe `Palette` lesen.

#### 2.2.1 Color Tokens

```go
type Palette struct {
    Name string

    // Surface — Backgrounds, vom dunkelsten zum hellsten.
    Bg            Color // Hauptbühne
    BgPanel       Color // Sub-Panel
    BgCode        Color // Code-Block
    BgChip        Color // Highlight-Hintergrund (selektierte Zeile)
    BgChipSoft    Color // Alternierende Zeile / dezente Tönung
    BgBar         Color // Heading- / Statusbar-BG
    BgDanger      Color // Callout-Danger-Fill
    BgSuccess     Color // Callout-Success-Fill

    // Foreground — Text-Stufen, vom hellsten zum gedämpftesten.
    Fg            Color // Body
    FgDim         Color // Sekundärtext
    FgMuted       Color // Hint, Meta — niemals tragender Inhalt

    // Hue — die rohen Farbpunkte. Keine semantische Bedeutung.
    Blue, Cyan, Green, Purple, Magenta, Yellow, Orange, Red, Teal Color

    // Tag-Rotation — stabile Hash-basierte Chip-Färbung.
    TagPalette []Color
}

// Semantic — Aliase auf Hues. Komponenten benutzen diese, nicht die Hues.
type Semantic struct {
    Accent       Color // Blue   — primärer interaktiver Akzent
    Active       Color // Cyan   — laufende / aktive Sache
    Success      Color // Green
    Warning      Color // Yellow
    Danger       Color // Red
    Info         Color // Cyan   — informativ ohne Aktion
    Highlight    Color // Purple — auffällige, nicht-aktionable Markierung
    BorderSubtle Color // BgChip
    BorderStrong Color // FgMuted
}

func (p Palette) Sem() Semantic { ... }
```

Regel: **Komponenten lesen ausschließlich `Sem()`**, nicht die Hues. Die Hues sind Implementierungsdetail der Palette und stehen Renderern offen, die wirklich freie Färbung brauchen (Markdown-Heading-Hierarchie, Tag-Chips).

#### 2.2.2 Spacing & Layout Tokens

```go
package theme

// Padding — horizontale Skala. Vertikal sind 0 oder 1 Zeile in der TUI.
const (
    PadNone = 0
    PadXS   = 1 // Standard für Chips, Pills, Statusbar
    PadSM   = 2 // Modal-Inhalt links/rechts
    PadMD   = 3 // Modal vertikal, Spaced-Sections
)

// Layout — wiederkehrende Spaltenbreiten.
const (
    PillWidth      = 4
    KeyHintWidth   = 12 // Spalte für Tasten-Labels in help-Overlays
    DayLabelWidth  = 3
    DateColWidth   = 9
    DefaultBox     = 60
    NarrowBox      = 40
    WideBox        = 80
)

// Z-Index für visuelle Hierarchie. Keine echte Z, aber Ordering-Hinweis
// für Border-Wahl und Padding-Wahl.
const (
    LayerSurface  = 0 // Bg
    LayerPanel    = 1 // BgPanel, NormalBorder
    LayerHover    = 2 // BgChipSoft
    LayerSelected = 3 // BgChip + AccentBar
    LayerOverlay  = 4 // RoundedBorder
    LayerModal    = 5 // DoubleBorder, BgPanel, höchstes Padding
)
```

#### 2.2.3 Typographic Tokens

Terminal-Typografie ist Monospace, also keine Font-Stack. Stilstufen statt Größen:

```go
// Style-Builder — pure (Palette, string) -> string.
package theme

func Heading1(p Palette, s string) string  // Bold + Accent
func Heading2(p Palette, s string) string  // Bold + Highlight
func Heading3(p Palette, s string) string  // Bold + Active
func Body(p Palette, s string) string      // Fg
func Dim(p Palette, s string) string       // FgDim
func Muted(p Palette, s string) string     // FgMuted
func Code(p Palette, s string) string      // Green + BgCode
func Strong(p Palette, s string) string    // Bold + Fg
func Emph(p Palette, s string) string      // Italic + Fg
func Success(p Palette, s string) string   // Green + Bold
func Warning(p Palette, s string) string   // Yellow + Bold
func Danger(p Palette, s string) string    // Red + Bold
```

Diese Builder ersetzen die ~70 wiederkehrenden Inline-`NewStyle()`-Aufrufe in Screens.

### 2.3 Komponenten-Spec — Vollausbau auf bubbletea + lipgloss

Jede Komponente folgt einer dieser zwei Formen:

- **Pure Render** — eine Funktion `Render(... p Palette) string`. Keine eigene State, kein eigener Lifecycle. Geeignet für Pills, Section-Headers, Boxes, Progress-Bars, Tabs.
- **`tea.Model`** — `Init() tea.Cmd`, `Update(tea.Msg) (Model, tea.Cmd)`, `View() string`. Geeignet für alles mit eigener State (Toast-Timer, Spinner-Tick, Form-Eingabe, Confirm).

Im Folgenden steht für jede Komponente: **Form**, **API**, **Variants**, **States**, **Tokens**, **A11y**, **Glyphs**.

#### 2.3.1 `box` (vorher `titlebox`)

| | |
|---|---|
| Form | Pure Render |
| API | `Render(opts BoxOpts, body string, p Palette) string` mit `BoxOpts{Title string; Border BorderKind; Tone Tone; Width int}` |
| Variants | `BorderRounded` (Hauptinhalt), `BorderNormal` (Sub-Panel), `BorderDouble` (Modal) |
| States | – |
| Tokens | `Sem.BorderSubtle` (default), `Sem.Accent` (focused), `Sem.Danger` (error tone), `Sem.Success` |
| A11y | Titel ist Strukturmarker; Border nutzt nur Box-Drawing-Zeichen |
| Glyphs | `╭ ╮ ╰ ╯ ─ │ ┌ ┐ └ ┘ ║ ═` |

#### 2.3.2 `pill` (vorher `theme.Pill`)

| | |
|---|---|
| Form | Pure Render |
| API | `Render(kind PillKind, label string, p Palette) string` |
| Variants | `PillNeutral`, `PillSuccess`, `PillWarning`, `PillDanger`, `PillActive`, `PillInfo`, `PillSkip` |
| States | – |
| Tokens | jeweils das `Sem.*` für Tone, `Bg` als Foreground |
| A11y | Glyph + Farbe (`✓ OK`, `✗ FAIL`, `▶ RUN`, `‖ PAUSE`, `○ SKIP`); Width fixed = `PillWidth` |
| Glyphs | siehe Whitelist |

#### 2.3.3 `chip` (NEU — generalisiert aus `tagChipStyle`)

| | |
|---|---|
| Form | Pure Render |
| API | `Render(label string, color Color, p Palette) string` plus `Hash(s string, palette []Color) Color` |
| Variants | `ChipSolid` (Bg = Color, Fg = Bg), `ChipOutline` (Fg = Color, Border) |
| States | – |
| Tokens | `TagPalette` für Hash-basierte Färbung |
| A11y | Bei Solid genug Kontrast über Bg-as-Fg; Outline-Variante als High-Contrast-Fallback |

#### 2.3.4 `picker.Row` und `picker.SectionHeader`

| | |
|---|---|
| Form | Pure Render |
| API | `Row(opts RowOpts, p Palette)` mit `RowOpts{Selected bool; Label, Hint string; Width int; Glyph rune; Truncate TruncMode}` |
| Variants | mit/ohne Hint, mit/ohne führenden Glyph |
| States | `default`, `selected` (`▎` + Bold-Fg), `disabled` (`Muted`) |
| Tokens | `Sem.Accent`, `Fg`, `FgMuted`, `BgChipSoft` (alternierend optional) |
| A11y | Selection per Akzentbar **plus** Bold (Glyph + Stil); `TruncEllipsis` ergänzt `…` statt überzulaufen |
| Glyphs | `▎` |

#### 2.3.5 `tabs` (NEU — generalisiert aus Worktime-Tab-Strip)

| | |
|---|---|
| Form | Pure Render |
| API | `Render(items []TabItem, active int, width int, p Palette) string` mit `TabItem{Label string; Glyph rune; Badge string}` |
| Variants | underline (default), pill (kompakt) |
| States | `active`, `inactive`, `disabled` |
| Tokens | `Sem.Accent`, `Fg`, `FgDim`, `BgChip` |
| A11y | Aktive Tab durch `▎`-Underline + Bold-Label, nicht nur Farbe |

#### 2.3.6 `card` (NEU — generalisiert aus markdown-Frontmatter-Card und Worktime-Heute-Block)

| | |
|---|---|
| Form | Pure Render |
| API | `Render(opts CardOpts, p Palette) string` mit `CardOpts{Badge string; BadgeKind PillKind; Title string; Meta string; Body string; Width int}` |
| Variants | mit/ohne Badge, mit/ohne Separator |
| States | – |
| Tokens | `Sem.Highlight` (Title), `FgDim` (Meta), `Sem.BorderSubtle` (Separator) |

#### 2.3.7 `statusbar.Hints`

| | |
|---|---|
| Form | Pure Render |
| API | `Render(items []HintItem, width int, p Palette) string` mit `HintItem{Key, Desc string}` |
| Variants | inline (`enter → run · q → quit`), wrapped (mehrzeilig) |
| States | – |
| Tokens | `FgMuted` (Default), `Sem.Accent` (Key) |
| A11y | Key-Highlight nutzt Bold + Accent — nicht nur Farbe |

#### 2.3.8 `statusbar.Bar` (Progressbar)

| | |
|---|---|
| Form | Pure Render |
| API | `Render(opts BarOpts, p Palette) string` mit `BarOpts{Pct, Cells int; Tone Tone; ShowLabel bool}` |
| Variants | nur Bar, Bar + `12%`-Label, Bar + Achievement-Glyph |
| Tokens | `Sem.Accent`, `Sem.BorderSubtle` |
| A11y | Filled/Empty-Glyphen verschieden + Farbe; Label optional zur reinen-Glyph-Lesbarkeit |
| Glyphs | `▰ ▱` |

#### 2.3.9 `spinner`

| | |
|---|---|
| Form | `tea.Model` |
| API | `New(label string, p Palette) Model` mit `WithTone(Tone)`, `WithKind(SpinnerKind)` |
| Variants | `Dot` (default), `Line`, `Pulse` |
| States | `running` (animiert), `still` (gestoppt) |
| Tokens | `Sem.Accent`, `FgDim` (Label) |
| A11y | Label ist Pflicht — Animation allein reicht nicht für Screen-Reader-Equivalent |

#### 2.3.10 `toast`

| | |
|---|---|
| Form | `tea.Model` |
| API | `New(text string, kind ToastKind, dur time.Duration, p Palette)` plus `NewSuccess`/`NewWarning`/`NewDanger`/`NewInfo` Convenience |
| Variants | Success, Warning, Danger, Info |
| States | `visible`, `dismissed` |
| Tokens | jeweils `Sem.Success`/`Sem.Warning`/`Sem.Danger`/`Sem.Info` |
| A11y | Glyph+Farbe (`✓`, `▲`, `✗`, `›`); Dauer ≥ 2 s |
| Glyphs | `✓ ▲ ✗ ›` |

#### 2.3.11 `confirm`

| | |
|---|---|
| Form | `tea.Model` |
| API | `New(question, detail string, p Palette)` plus Builder `.WithHint(string)`, `.WithKind(ConfirmKind)` |
| Variants | `ConfirmDefault`, `ConfirmDanger` (Border + Question in Danger-Tone) |
| States | `open`, `confirmed`, `denied` |
| Tokens | `Sem.Warning` (Frage default), `Sem.Danger` (Danger-Variante), `FgMuted` (Hint) |
| A11y | Hint kanonisch (`HintConfirm`); kein Default-Confirm bei Enter ohne `y` |
| Glyphs | – |

#### 2.3.12 `form.TextInput` und `form.ChoiceModel`

| | |
|---|---|
| Form | `tea.Model` (jeweils) |
| API TextInput | `New(opts InputOpts, p Palette)` mit `InputOpts{Placeholder, Help string; CharLimit int; Validate func(string) error}` |
| API Choice | `New(items []Choice, width int, p Palette)` |
| Variants | TextInput: `default`, `inline-error`, `password`. Choice: `default`, `with-glyph` |
| States | `default`, `focused`, `disabled`, `error` |
| Tokens | `Sem.Accent` (Cursor + Focus), `Sem.Danger` (Error-Border), `FgDim` (Placeholder) |
| A11y | Fokus per Akzentbar links **plus** Cursor-Farbe; Error-Message ist Pflichtfeld unter Input |

#### 2.3.13 `help`

| | |
|---|---|
| Form | Pure Render |
| API | `Render(title string, sections []Section, opts HelpOpts, p Palette) string` mit `HelpOpts{KeyWidth, Width int; ShowFooterHint bool}` |
| Variants | overlay (umgeben von `box`), inline (ohne Box) |
| Tokens | `Sem.Accent` (Section-Title), `Fg` (Key), `FgMuted` (Description) |
| A11y | Tasten-Spalte ist fix breit (`KeyHintWidth`) — Augenführung |

#### 2.3.14 `modal` (NEU — aus `kompendium/browse/styles.go` ins Kit gezogen)

| | |
|---|---|
| Form | `tea.Model` Wrapper mit Slot |
| API | `New(content tea.Model, opts ModalOpts, p Palette)` mit `ModalOpts{Title string; Kind ModalKind; Width int}` |
| Variants | `ModalDefault`, `ModalDanger`, `ModalSafe` |
| States | `open`, `closing` |
| Tokens | `Sem.Accent`/`Sem.Danger`/`Sem.Success` (Border je Kind), `BgPanel` (Inhalt-BG), `PadMD` (vertikal), `PadSM` (horizontal) |
| A11y | Esc immer schließt; erste fokussierbare Stelle erhält Fokus on Open |
| Glyphs | `║ ═ ╔ ╗ ╚ ╝` |

#### 2.3.15 `viewport`

Bleibt als bubble-Wrapper. Die Style-Hooks (Scrollbar-Glyph, Border) müssen aus dem neuen Token-System gespeist werden.

### 2.4 Markdown-Renderer

Der Markdown-Renderer ist eine **eigene Komponente, kein eigenes Theme**. Die Migration:

1. `MarkdownRolesFor(r *lipgloss.Renderer)` → `MarkdownRolesFor(r *lipgloss.Renderer, p Palette)`. Keine Package-Globals, keine `SetActive`. Test-Setup bleibt sauber, parallel-fähig.
2. `internal/frontend/tui/markdown/theme/palette.go` zieht die `Palette`-Definition aus `internal/frontend/tui/theme/`. Die heutige `Palette`-Struct dort wird zur internen Role-Map (`H1Bar`, `CodeFenceBg`, …) — die rohen Farben kommen aus dem geteilten Token-Set.
3. Heading-Hierarchie: H1 = Bar (`BgBar` + `Highlight` + Bold), H2 = Chip (`BgChip` + `Highlight` + Bold + `PadXS`), H3 = `Sem.Active` Bold, H4 = `Sem.Active`, H5 = `FgDim`, H6 = `FgMuted` (kein `Faint()` mehr — A11y).

### 2.5 Accessibility — als Teil der API

Accessibility ist hier kein separater Audit, sondern Teil dessen, wie das System gebaut wird. Sechs verbindliche Regeln:

#### A11y-1 — WCAG 2.1 AA als Mindestkontrast

Alle Token-Kombinationen, die als „Text auf Hintergrund" oder „Glyph auf Hintergrund" verwendet werden, müssen mindestens **4,5:1** Kontrast haben (3:1 für Glyphen ≥ 14 px equivalent — TUI ist Monospace, das praktisch ist alles ≥ 14 px).

**Umsetzung:** Ein Test in `internal/frontend/tui/theme/contrast_test.go`, der für jede registrierte Palette die folgenden Paare prüft:

```
(Fg, Bg), (FgDim, Bg), (Fg, BgPanel), (Fg, BgChip),
(Bg, Sem.Accent), (Bg, Sem.Success), (Bg, Sem.Warning),
(Bg, Sem.Danger), (Bg, Sem.Info), (Bg, Sem.Highlight),
(Sem.Accent, Bg), (Sem.Danger, Bg), …
```

Schlägt der Test fehl, kommt die Palette nicht ins Repo. **Tokyonight Night** und **Catppuccin Mocha** werden gegengeprüft, Lücken werden direkt geschlossen (z. B. `FgMuted` ggf. eine Stufe heller).

#### A11y-2 — Keine Farbe-allein-Signale

Jede Information, die heute nur durch Farbe transportiert wird, bekommt zusätzlich einen Glyph oder eine Bold/Italic-Stil-Komponente. Konkret:

- Pills tragen Glyph (siehe 2.3.2).
- Pace-Dots: gefüllt vs. leer (`●`/`○`) plus Farbe.
- Worktime-Status: Glyph wechselt mit Status (`▶ ‖ ■`) — schon heute korrekt, wird verbindlich.
- Wikilinks: Underline für `valid`, `?`-Marker plus Strikethrough für `broken` — Farbe ist Sekundärsignal.

#### A11y-3 — `Faint()` nicht auf Pflichttext

`Faint()` senkt die Helligkeit terminalseitig oft um 30–50 %. Auf bereits gedimmtem Foreground (`FgDim`, `FgMuted`) reißt das den Kontrast unter AA. Regel: `Faint()` nur auf dekorativen Begleitelementen, nie auf Inhalt, der zur Aufgabe gehört. H6 und FootnoteDef werden umgestellt.

#### A11y-4 — NO_COLOR-Pfad funktioniert end-to-end

Der Markdown-Renderer hat heute schon einen NO_COLOR-Pfad über `lipgloss.Renderer` mit `termenv.Ascii`. Die Tokens, Pills, Chips, Borders müssen auch in dem Profil nicht vom Sinn her brechen — d. h. **kein Status hängt allein an Farbe**. Tests rendern jeden Component-View in einem `Renderer`-Instance mit `termenv.Ascii` und prüfen, dass z. B. ein „FAIL"-Pill weiterhin als „FAIL" lesbar ist (Glyph + Label).

#### A11y-5 — Keyboard-Vollständigkeit

Jede interaktive Komponente:

- Hat einen sichtbaren Fokus-Indikator (`▎` + Bold), der ohne Farbe erkennbar ist.
- Kennt `Esc` als Cancel/Back.
- Dokumentiert ihre Tasten in einem `KeyMap`-Wert, den die Komponente exportiert. Das Sidekick-`?`-Overlay zieht sich daraus die Hilfe automatisch — heute ist das hand-gepflegt und drift-anfällig.

```go
type KeyMap struct {
    Up, Down, Confirm, Cancel, Help key.Binding
    // …
}

func (m Model) Keys() KeyMap { return m.keys }
```

#### A11y-6 — Touchpoints für Screen-Reader

Terminal-Screen-Reader (z. B. macOS VoiceOver mit `term-bridge`, NVDA über WSL) lesen den ANSI-Strom direkt. Drei Konsequenzen:

- Box-Drawing wird als Strukturlinie vorgelesen — okay.
- ANSI-Escape-Sequenzen werden ignoriert — d. h. die Reihenfolge der Render-Strings muss inhaltlich sinnvoll sein, auch ohne Farb-Markup. Tests rendern in `termenv.Ascii` und vergleichen den ge-strippten Text mit einer Snapshot-Version.
- Keine Information darf in Glyphen liegen, die im Screen-Reader-Wörterbuch fehlen. `▶`, `✓`, `✗` werden gut gelesen; `▰`, `▱` werden teilweise als „BLACK RECTANGLE" / „LIGHT RECTANGLE" vorgelesen — okay als Begleit-Visualisierung, aber nicht als alleinige Info; deshalb hat Progress-Bar optional ein Label.

### 2.6 Konsumenten-Disziplin

Drei Garde-Mechanismen, die die Designlinie schützen:

1. **`depguard` erweitern** — `internal/frontend/tui/screen/...` darf `lipgloss.NewStyle` nur aus dem `theme`- oder `components`-Paket aufrufen, nicht direkt importieren. Konkret: Wenn ein Screen `lipgloss` direkt importiert, darf er nur `lipgloss.JoinHorizontal`/`JoinVertical`/`PlaceHorizontal`-artige Layout-Helpers nutzen, keine `NewStyle`-Builder. Ein `revive`-Custom-Lint oder ein einfacher Grep im CI-Step reicht aus.
2. **Style-Builder im Kit** (siehe 2.2.3) — `theme.Dim`, `theme.Danger`, … werden ausgereicht. Code-Review-Standard: kein neuer Inline-`NewStyle()` ohne Kommentar, warum ein Builder nicht passt.
3. **Komponenten-Inventar als Doku** — `docs/design-system.md` (das hier) ist die Quelle. PRs, die ein neues UI-Pattern einführen, dokumentieren es zuerst hier oder begründen, warum es einmalig ist.

### 2.7 Roadmap (vier Phasen)

| Phase | Inhalt | Aufwand | Voraussetzung |
|---|---|---|---|
| **P1 — Tokens** | `internal/frontend/tui/theme/` mit `Palette` + `Sem` + Spacing/Layout-Tokens. Tokyonight Night und Catppuccin Mocha als kanonisch. Contrast-Test (A11y-1). `domain.StatusPalette` wird Adapter-View darüber. | M | – |
| **P2 — Markdown entkoppeln** | `MarkdownRolesFor(r, p)` mit Palette-Parameter. Globals weg. `H6` ohne `Faint()`. NO_COLOR-Test (A11y-4). | M | P1 |
| **P3 — Component-Kit ausbauen** | Style-Builder (`Dim`, `Danger`, …). `pill`, `toast`, `confirm` mit Variants. Neue Komponenten: `chip`, `tabs`, `card`, `modal`. `KeyMap`-Export pro Komponente (A11y-5). Glyph-Whitelist als Konstanten. Strings-Konstanten (kanonische deutsche UI-Strings). | L | P1 |
| **P4 — Screens migrieren** | Worktime, Projects, Cheatsheet, Palette-Screen, Kompendium-Browse/View ziehen Inline-`NewStyle` durch. Modal aus `kompendium/browse` ins Kit gehoben. `depguard`-Regel scharf gestellt. Doku in `docs/design-system.md` finalisiert. | L | P3 |

Jede Phase ist für sich shipbar. P1+P2 zusammen lösen die größte heutige Inkonsistenz (zwei „Tokyonight" auf einer Bühne). P3 macht die Komponenten erst wirklich vollwertig. P4 räumt die Screens auf.
