# Lesesaal L1 — Fundament Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Die Lesesaal-Design-Grundlage in die echte App bringen: Schibsted-Typografie, Lesesaal-Token (Hell als Zuhause), Zwei-Flächen-Primitives + Avatar/Chip-Farbsystem, Topbar-Shell statt Sidebar, Timer-Pill, ⌘K-Palette — alle Bestandsseiten bleiben funktionsfähig (Alias-Token), ihr eigentlicher Umbau folgt in L2–L4.

**Architecture:** Token-Swap unter stabilen Utility-Namen (canvas/sunken/line/blue … zeigen auf Lesesaal-Werte) hält die 12 Seiten-Templates unangetastet; `AppShell(active, breadcrumb, subnav, content)` behält die Signatur und wird intern zur Topbar; die Timer-Pill ist die Evolution des existierenden `TimerChip` (Start/Stop/Switch-Handler + SSE-Refresh unverändert); die Palette ist ein htmx-Endpoint über `fuzzymatch` (bestehendes domain-freies Paket) + ein kleines vendored JS.

**Tech Stack:** Go 1.x · templ · Tailwind v4.1.5 (CLI, `make web`) · htmx (vendored) · Schibsted Grotesk (SIL-OFL, vendored woff2) + JetBrains Mono (vorhanden).

**Spec:** `docs/superpowers/specs/2026-07-04-lesesaal-webui-redesign-design.md` · **Normatives Mockup:** `docs/superpowers/specs/assets/2026-07-03-lesesaal/lesesaal.html` (bei Zweifel gewinnt das Mockup).

## Global Constraints

- Branch **`lesesaal`** (off `rebuild` `78ebac3`); Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`.
- **NIE `make fmt`** ausführen. **NIE `git stash`** in Dispatches. Nach jedem Task: `git log --oneline -1` prüfen, dass HEAD vorangegangen ist.
- `make ci` muss am Task-Ende grün sein (Gate 75 %, `*_templ.go` ausgeschlossen; pgstore-Tests brauchen den Podman-Socket wie bisher).
- Nach JEDER `.templ`-Änderung: `make generate` und die `*_templ.go` mitcommitten. Nach JEDER `web/tailwind.css`-Änderung: `make web` und `internal/adapter/webui/static/app.css` mitcommitten (verify-css ist ein Drift-Diff).
- i18n: jede neue Nutzertext-Zeile in **beiden** Katalogen (`internal/i18n/catalog_de.go` + `catalog_en.go`).
- Keine Emojis, keine Browser-Popups (`verify-no-popups`), owner-scoped bleibt überall unangetastet.
- Farb-Gesetz der Spec §7: Farbe pro Projekt existiert NUR im Avatar; Kinds bleiben neutrale Text-Chips.
- Tailwind-v4-Fallen (Memory): kein `<alpha-value>` in `@theme`; niemals `*/` in CSS-Kommentaren; `@source not`-Zeilen nicht anfassen.

## Agent-Besetzung & Dispatch-Protokoll (gilt für L1 und sinngemäß für L2–L4)

Die Rollen sind als Projekt-Agents in `.claude/agents/` hinterlegt — Modell + Effort + Regelwerk fest verdrahtet. Die Orchestrator-Session läuft auf `/effort high` (Fallback: erben Agents keinen Frontmatter-Effort, ist high die Grundlinie). Dispatches nennen das Modell NIE implizit — es steht im Agent (Memory: nie Fable erben).

| Task | Agent (`subagent_type`) | Modell · Effort |
|---|---|---|
| 1 Schriften | `lesesaal-implementer` | Sonnet · medium |
| 2 Token | `lesesaal-implementer` | Sonnet · medium |
| 3 Primitives | `lesesaal-implementer` | Sonnet · medium |
| 4 Topbar-Shell | `lesesaal-implementer-deep` | Sonnet · high |
| 5 Timer-Pill | `lesesaal-implementer` | Sonnet · medium |
| 6 Palette | `lesesaal-implementer-deep` | Sonnet · high |
| 7 Wiring-Gate | `lesesaal-implementer` | Sonnet · medium |
| jedes Task-Review | `lesesaal-task-reviewer` | Haiku · high |
| Slice-Ende: Whole-Branch | `lesesaal-final-reviewer` | Opus · xhigh |
| Slice-Ende: Design-Treue | `lesesaal-mockup-auditor` | Sonnet · medium |

**Protokoll pro Task:**
1. Dispatch Implementer mit: wörtlichem Task-Text + Global-Constraints-Block + „Branch `lesesaal`, Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`". Ein Task pro Dispatch, nie mehrere.
2. Orchestrator verifiziert danach selbst: `git log --oneline -3` (HEAD vorangegangen? — Subagent-Commits können den Branch-Ref verfehlen, Memory) + `git diff --stat HEAD~1`.
3. Dispatch `lesesaal-task-reviewer` mit Task-Text + Commit-Range. `Rejected`/Critical → Fix-Dispatch an denselben Implementer-Agent mit den Findings; Minor darf der Orchestrator selbst fixen.
4. Ledger `.superpowers/sdd/progress.md` fortschreiben (Commits, Verdikt, ci-Stand).

**Protokoll Slice-Ende:** `make ci` grün → `lesesaal-final-reviewer` (Range `rebuild..HEAD`) → Findings fixen → `lesesaal-mockup-auditor` → Abweichungen fixen → **Soenne-Live-Gate** (Browser, nicht delegierbar).

**Optional, ohne Setup vorhanden:** `gemini-bigcontext` (einmal pro Slice: Kristall-Reste/Arbitrary-Values-Sweep über alle geänderten Dateien) · `codex-second-opinion` (nur L3: Mermaid client-side vs. server-side) · `memory-bank-synchronizer` (nach gelandetem Slice).

---

### Task 1: Schriften — Schibsted Grotesk rein, Clash Display + Inter raus

**Files:**
- Create: `internal/adapter/webui/static/fonts/SchibstedGrotesk-Variable.woff2`
- Delete: `internal/adapter/webui/static/fonts/ClashDisplay-Variable.woff2`, `internal/adapter/webui/static/fonts/Inter-Variable.woff2`
- Modify: `web/tailwind.css` (@font-face-Block Z.184–198, @theme-Fonts Z.162–164)
- Modify: `internal/adapter/webui/components/base.templ` (Preloads Z.30–32)
- Test: `internal/adapter/webui/components/base_test.go`

**Interfaces:**
- Produces: Font-Familien `"Schibsted Grotesk"` (400–900, `--font-sans` UND `--font-display`) + `"JetBrains Mono"`; Preload-Pfad `/static/fonts/SchibstedGrotesk-Variable.woff2`. Spätere Tasks verlassen sich darauf, dass `font-display`/`font-sans` dieselbe Familie sind.

- [ ] **Step 1: Font-Datei holen**

```bash
curl -sf -o internal/adapter/webui/static/fonts/SchibstedGrotesk-Variable.woff2 \
  "https://fonts.gstatic.com/s/schibstedgrotesk/v7/Jqz55SSPQuCQF3t8uOwiUL-taUTtap9IayojdSFOd1I.woff2"
ls -la internal/adapter/webui/static/fonts/
```
Expected: Datei ~20 KB. (Falls der gstatic-Hash 404 liefert: `curl -s -A "Mozilla/5.0 ... Chrome/126.0" "https://fonts.googleapis.com/css2?family=Schibsted+Grotesk:wght@400..900&display=swap"` ausführen und die dort gelistete latin-woff2-URL nehmen.)

- [ ] **Step 2: Failing Test schreiben** — in `base_test.go` ergänzen:

```go
func TestBase_PreloadsLesesaalFonts(t *testing.T) {
	var sb strings.Builder
	if err := components.Base("t", components.Empty()).Render(testCtx(t), &sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.Contains(out, "/static/fonts/SchibstedGrotesk-Variable.woff2") {
		t.Fatalf("Schibsted preload missing:\n%s", out)
	}
	for _, gone := range []string{"ClashDisplay", "Inter-Variable"} {
		if strings.Contains(out, gone) {
			t.Fatalf("stale font reference %q still present", gone)
		}
	}
}
```
(`Empty()`/`testCtx` wie in den bestehenden Tests dieser Datei; existiert dort kein Helper, `templ.NopComponent` und `i18n`-Kontext der Nachbartests übernehmen.)

- [ ] **Step 3: Test laufen lassen — muss failen**

Run: `go test ./internal/adapter/webui/components/ -run TestBase_PreloadsLesesaalFonts -race`
Expected: FAIL („Schibsted preload missing").

- [ ] **Step 4: base.templ Preloads tauschen** — Z.30–32 ersetzen durch:

```html
<link rel="preload" as="font" type="font/woff2" href="/static/fonts/SchibstedGrotesk-Variable.woff2" crossorigin/>
<link rel="preload" as="font" type="font/woff2" href="/static/fonts/JetBrainsMono-Variable.woff2" crossorigin/>
```

- [ ] **Step 5: tailwind.css Fonts tauschen** — @font-face-Block (Z.184–198) ersetzen:

```css
@font-face {
  font-family: "Schibsted Grotesk";
  src: url("/static/fonts/SchibstedGrotesk-Variable.woff2") format("woff2");
  font-weight: 400 900; font-style: normal; font-display: swap;
}
@font-face {
  font-family: "JetBrains Mono";
  src: url("/static/fonts/JetBrainsMono-Variable.woff2") format("woff2");
  font-weight: 400 800; font-style: normal; font-display: swap;
}
```

und im `@theme` (Z.162–164):

```css
  --font-display: "Schibsted Grotesk", ui-sans-serif, system-ui, sans-serif;
  --font-sans:    "Schibsted Grotesk", ui-sans-serif, system-ui, sans-serif;
  --font-mono:    "JetBrains Mono", ui-monospace, SFMono-Regular, monospace;
```

- [ ] **Step 6: Alte Font-Dateien löschen**

```bash
git rm internal/adapter/webui/static/fonts/ClashDisplay-Variable.woff2 internal/adapter/webui/static/fonts/Inter-Variable.woff2
```

- [ ] **Step 7: Bauen + Tests**

Run: `make generate && make web && go test ./internal/adapter/webui/... -race`
Expected: PASS; `git status` zeigt geänderte `app.css`, `base_templ.go`.

- [ ] **Step 8: Commit**

```bash
git add -A internal/adapter/webui/static/fonts web/tailwind.css internal/adapter/webui/static/app.css internal/adapter/webui/components/base.templ internal/adapter/webui/components/base_templ.go internal/adapter/webui/components/base_test.go
git commit -m "feat(lesesaal): Schibsted Grotesk als eine UI-Familie, Clash/Inter ausgemustert"
```

---

### Task 2: Lesesaal-Token — Hell als Zuhause, Alias-Kompatibilität, Glas stirbt

**Files:**
- Modify: `web/tailwind.css` (Token-Blöcke Z.25–120, @theme Z.132–175, BASE Z.200–242, `.glass`-Regeln Z.372–390)
- Modify: `internal/adapter/webui/components/base.templ` (No-Flash-Script, Facets-SVG, Theme-Script)
- Modify: `internal/adapter/webui/components/appshell.templ` + `themetoggle.templ` (Toggle raus)
- Test: `internal/adapter/webui/components/base_test.go`

**Interfaces:**
- Produces (für ALLE späteren Tasks): Legacy-Utilities zeigen auf Lesesaal-Werte (`bg-canvas`→Papier, `bg-sunken`→Wash, `border-line`→Haarlinie, `text-blue`→Akzent, `text-green`→Live …) **und** neue Utilities `bg-paper, bg-panel, bg-wash, bg-sheet, border-hair, border-hair2, border-hairp, text-meta, text-live, text-live-bright, text-warn, bg-accent, text-accent`.
- Produces: Theme ist in L1 fix `light` (Toggle entfernt; Dunkel-Zwilling = L7).

- [ ] **Step 1: Failing Test** — in `base_test.go`:

```go
func TestBase_LightIsHome_NoFacetsNoToggle(t *testing.T) {
	var sb strings.Builder
	if err := components.Base("t", components.Empty()).Render(testCtx(t), &sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.Contains(out, `data-theme','light'`) && !strings.Contains(out, `data-theme", "light"`) {
		t.Fatalf("no-flash script does not force light:\n%s", out)
	}
	for _, gone := range []string{"kristall-facets", "toggleTheme", "flow-theme"} {
		if strings.Contains(out, gone) {
			t.Fatalf("kristall remnant %q still present", gone)
		}
	}
}
```

- [ ] **Step 2: Test laufen lassen** — Expected: FAIL (facets vorhanden).

- [ ] **Step 3: tailwind.css Token-Blöcke ersetzen** — die BEIDEN Blöcke `:root[data-theme="light"]` (Z.26–71) und `:root` (Z.74–120) durch EINEN Lesesaal-Block ersetzen (Spec §6; Triplets aus Hex):

```css
/* ── LESESAAL — Hell ist Zuhause (Spec §6/§7; Dunkel-Zwilling = L7) ── */
:root {
  color-scheme: light;
  --canvas:  250 250 247;   /* paper #FAFAF7 */
  --surface: 255 255 255;
  --sunken:  243 241 235;   /* wash  #F3F1EB */
  --line:    231 228 220;   /* hair  #E7E4DC */
  --line2:   236 234 228;
  --ink:      28  27  24;   /* #1C1B18 */
  --body:     61  59  54;
  --muted:   110 105  97;   /* meta  #6E6961 */
  --faint:   152 147 138;   /* #98938A */

  /* Lesesaal-Eigennamen */
  --paper:   250 250 247;
  --wash:    243 241 235;
  --sheet:   244 242 236;   /* #F4F2EC */
  --panel:   240 237 228;   /* #F0EDE4 */
  --hair:    231 228 220;
  --hair2:   217 213 202;   /* #D9D5CA */
  --hairp:   224 219 205;   /* #E0DBCD */
  --meta:    110 105  97;
  --accent:       43  91 246;   /* #2B5BF6 */
  --accent-deep:  29  70 216;   /* #1D46D8 */
  --accent-wash: 234 240 254;   /* #EAF0FE */
  --live:         15 138  70;   /* #0F8A46 */
  --live-bright:  28 193  97;   /* #1CC161 */
  --live-wash:   227 247 235;   /* #E3F7EB */
  --warn:        180  83   9;   /* #B45309 */
  --warn-wash:   251 240 220;   /* #FBF0DC */

  /* Legacy-Farbnamen → Lesesaal-Semantik (Seiten-Kompatibilität, L2–L4 räumt auf) */
  --blue:     43  91 246;
  --cyan:     14 152 136;   /* Petrol — trägt wtblock-running/now-line bis L4 */
  --green:    15 138  70;
  --purple:  122  63 228;
  --magenta: 209  67  36;
  --yellow:  180  83   9;
  --orange:  217 119   6;
  --red:     209  67  36;
  --teal:     14 152 136;
  --oncolor: 255 255 255;

  --grad-a:   43  91 246;   /* Gradients sterben: beide Enden = Akzent */
  --grad-b:   43  91 246;

  --glass:   240 237 228;   /* .glass wird opake Fläche (Übergangs-Alias) */
  --glass-a: 1;
  --glass-strong-a: 1;
  --glass-border-a: 0;

  --backdrop: linear-gradient(0deg, #FAFAF7, #FAFAF7);
  --facet-a: 0;

  --shadow:         28  27  24;
  --shadow-accent:  28  27  24;

  --code-bg:    15  20  38;   /* Code bleibt dunkel lesbar; Chroma-Pass = L3 */
  --code-fg:   201 212 245;
  --halo:       43  91 246;
  --halo-a:    0.10;
  --scrollthumb: 217 213 202;
}
```

- [ ] **Step 4: @theme ergänzen** — im bestehenden `@theme`-Block hinter `--color-danger` einfügen (Legacy-Zeilen bleiben):

```css
  --color-paper:       rgb(var(--paper));
  --color-wash:        rgb(var(--wash));
  --color-sheet:       rgb(var(--sheet));
  --color-panel:       rgb(var(--panel));
  --color-hair:        rgb(var(--hair));
  --color-hair2:       rgb(var(--hair2));
  --color-hairp:       rgb(var(--hairp));
  --color-meta:        rgb(var(--meta));
  --color-accent-deep: rgb(var(--accent-deep));
  --color-accent-wash: rgb(var(--accent-wash));
  --color-live:        rgb(var(--live));
  --color-live-bright: rgb(var(--live-bright));
  --color-live-wash:   rgb(var(--live-wash));
  --color-warn:        rgb(var(--warn));
  --color-warn-wash:   rgb(var(--warn-wash));
```
und `--color-accent: rgb(var(--blue));` auf `rgb(var(--accent))` umstellen. Die Shadow-Tokens beruhigen:

```css
  --shadow-soft: 0 1px 2px rgb(var(--shadow) / .05);
  --shadow-lift: 0 18px 50px -24px rgb(var(--shadow) / .28);
  --shadow-ring: 0 10px 40px -18px rgb(var(--shadow) / .22);
```

- [ ] **Step 5: BASE-Sektion Lesesaal-fest machen** — `html { background: var(--backdrop) fixed; … }` ersetzen durch `html { background: rgb(var(--paper)); min-height: 100%; overflow-x: clip; }`; direkt darunter `body { overflow-x: clip; }` ergänzen; die Regeln `.kristall-facets`, `.toggle-knob`/`.toggle-sun`/`.toggle-moon` (Z.205, 226–235) und die `body:has(#cockpit-rail) #timer-widget`-Demote-Regel (Z.386–390) ERSATZLOS löschen; Fokus-Ring auf `outline: 2px solid rgb(var(--accent)); outline-offset: 2px; border-radius: 2px;` stellen. `.glass`/`.glass-strong` ersetzen durch:

```css
  /* Übergangs-Alias bis L2–L4: ehemalige Glas-Karten liegen als ruhige Fläche */
  .glass, .glass-strong { background: rgb(var(--panel)); border: 1px solid rgb(var(--hairp)); }
```
Die `:root[data-theme="light"] .field`-Sonderregeln (Z.364–369) löschen und `.field` selbst auf `background: rgb(255 255 255 / .8); border: 1px solid rgb(var(--hair2));` stellen (Fokus-Zeile behalten, `--blue`→`--accent`). Alle übrigen `:root:not([data-theme="light"])`-Regeln in der Datei löschen (Dark folgt in L7 frisch).

- [ ] **Step 6: base.templ säubern** — (a) No-Flash-Script ersetzen durch:

```html
<script>document.documentElement.setAttribute('data-theme','light');</script>
```
(b) den kompletten `<svg class="kristall-facets" …></svg>`-Block löschen; (c) das Theme-Toggle-`<script>` (toggleTheme/storage/syncToggles, Z.76–100) komplett löschen; (d) `body`-Klasse `selection:bg-blue/15` → `selection:bg-accent-wash`.

- [ ] **Step 7: Toggle-Aufrufe entfernen** — in `appshell.templ` beide `@ThemeToggle()`-Zeilen löschen; `themetoggle.templ` + generiertes `themetoggle_templ.go` per `git rm` entfernen (Task 4 baut die Shell ohnehin um — hier nur toggle-frei machen, damit der Build steht).

- [ ] **Step 8: Bauen + kompletter Testlauf**

Run: `make generate && make web && go test ./... -race 2>&1 | tail -20`
Expected: PASS überall (Render-Tests, die „glass" o. Ä. asserten, auf die neuen Klassen anpassen — Assertions ändern, nie Verhalten wegtesten).

- [ ] **Step 9: Commit**

```bash
git add -A && git commit -m "feat(lesesaal): Token-Fundament — Hell als Zuhause, Alias-Kompatibilität, Glas/Facets/Toggle entfernt"
```

---

### Task 3: Zwei-Flächen-Primitives + Avatar/Chip-Farbsystem (Go-Helfer als Single Source)

**Files:**
- Create: `internal/adapter/webui/identity.go` + `internal/adapter/webui/identity_test.go`
- Create: `internal/adapter/webui/components/avatar.templ`
- Modify: `web/tailwind.css` (@layer components: `.panel`, `.typechip`, `.tc-*`, `.av-*`, `.eyebrow`)
- Modify: `internal/adapter/webui/components/styleguide.templ` (neue Lesesaal-Sektion)
- Test: `internal/adapter/webui/components/primitives_test.go`

**Interfaces:**
- Produces: `webui.Initials(name string) string` (1–2 Großbuchstaben) · `webui.AvatarTone(name string) string` (deterministisch `"av-a"…"av-f"`) · `webui.ShortName(name string) string` (letztes `/`-Segment, Fallback ganzer Name) · `components.Avatar(name string, sizeClass string) templ.Component` (getönte Initialen-Kachel; `sizeClass` z. B. `"av-28"`, `"av-36"`, `"av-96"`).
- Tasks 4–6 konsumieren exakt diese Namen.

- [ ] **Step 1: Failing Unit-Tests** — `identity_test.go`:

```go
package webui

import "testing"

func TestInitials(t *testing.T) {
	cases := map[string]string{
		"backstage":                        "BA",
		"RTL Extern":                       "RE",
		"flow":                             "FL",
		"kickstart-aws-infra":              "KI",
		"gitlab.com/dataalliance/x/y/cmdb": "CM",
		"":                                 "?",
	}
	for in, want := range cases {
		if got := Initials(ShortName(in)); got != want {
			t.Errorf("Initials(ShortName(%q)) = %q, want %q", in, got, want)
		}
	}
}

func TestShortName(t *testing.T) {
	cases := map[string]string{
		"gitlab.com/dataalliance/products/foolu/product/backstage": "backstage",
		"RTL Extern": "RTL Extern",
		"a/b/":       "b",
		"":           "",
	}
	for in, want := range cases {
		if got := ShortName(in); got != want {
			t.Errorf("ShortName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAvatarTone_DeterministicAndSpread(t *testing.T) {
	if AvatarTone("backstage") != AvatarTone("backstage") {
		t.Fatal("tone not deterministic")
	}
	seen := map[string]bool{}
	for _, n := range []string{"backstage", "flow", "RTL Extern", "cmdb", "infra", "skopeo", "tf-modules", "k8s-infra"} {
		seen[AvatarTone(n)] = true
	}
	if len(seen) < 3 {
		t.Fatalf("tones not spread: %v", seen)
	}
	for tone := range seen {
		if len(tone) != 4 || tone[:3] != "av-" || tone[3] < 'a' || tone[3] > 'f' {
			t.Fatalf("bad tone %q", tone)
		}
	}
}
```

- [ ] **Step 2: Laufen lassen** — Expected: FAIL (undefined: Initials).

- [ ] **Step 3: identity.go implementieren**

```go
package webui

import "strings"

// ShortName ist der Anzeigename eines Knotens: das letzte Pfadsegment eines
// Remote-Namens (Spec §5 Kurznamen-Regel; Kollisions-Dedup folgt in L2).
func ShortName(name string) string {
	name = strings.TrimRight(strings.TrimSpace(name), "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return name
}

// Initials liefert 1–2 Großbuchstaben für den Ersatz-Avatar: Anfangsbuchstaben
// der ersten beiden Wörter (Trenner: Space, "-", "_", "."), sonst die ersten
// beiden Runen. Leer → "?".
func Initials(name string) string {
	fields := strings.FieldsFunc(strings.TrimSpace(name), func(r rune) bool {
		return r == ' ' || r == '-' || r == '_' || r == '.'
	})
	switch {
	case len(fields) == 0:
		return "?"
	case len(fields) == 1:
		r := []rune(fields[0])
		if len(r) == 1 {
			return strings.ToUpper(string(r[0]))
		}
		return strings.ToUpper(string(r[0]) + string(r[1]))
	default:
		return strings.ToUpper(string([]rune(fields[0])[0]) + string([]rune(fields[1])[0]))
	}
}

// AvatarTone wählt deterministisch einen der sechs Lesesaal-Töne (Spec §7.2).
// Farbe pro Projekt lebt NUR hier — nirgendwo sonst.
func AvatarTone(name string) string {
	var h uint32
	for _, r := range name {
		h = h*31 + uint32(r)
	}
	return string([]byte{'a', 'v', '-', byte('a' + h%6)})
}
```

- [ ] **Step 4: Unit-Tests grün laufen lassen** — `go test ./internal/adapter/webui/ -run 'TestInitials|TestShortName|TestAvatarTone' -race` → PASS.

- [ ] **Step 5: CSS-Primitives** — in `web/tailwind.css` im ersten `@layer components` ergänzen:

```css
  /* Zwei-Flächen-Regel (Spec §4): Inhalt auf Papier, Instrumente auf Fläche */
  .panel { background: rgb(var(--panel)); border-radius: 14px; }

  .eyebrow { font-size: 11px; font-weight: 600; letter-spacing: .09em; text-transform: uppercase; color: rgb(var(--faint)); }

  /* Dokumenttyp-Chips (Spec §7.1) — feste semantische Zuordnung, nie pro Projekt */
  .typechip { font-family: "JetBrains Mono", ui-monospace, monospace; font-size: 11px; color: rgb(var(--meta)); border: 1px solid rgb(var(--hair2)); border-radius: 4px; padding: 1.5px 6px; background: rgb(var(--surface)); flex-shrink: 0; }
  .tc-b { color: #2554E8; background: #E8EEFE; border-color: #C9D8FC; }
  .tc-v { color: #7A3FE4; background: #F1EAFE; border-color: #DFD0FA; }
  .tc-t { color: #0B8A7B; background: #E2F5F2; border-color: #C6EAE5; }
  .tc-o { color: #B45309; background: #FDF0D9; border-color: #F5DFB3; }
  .tc-g { color: #15883E; background: #E5F6EA; border-color: #CBEBD5; }
  .tc-r { color: #D14324; background: #FDEAE4; border-color: #F8D5C9; }

  /* Ersatz-Avatare (Spec §7.2/§8): weiße Initialen auf sattem Grund */
  .ava { display: inline-grid; place-items: center; flex-shrink: 0; font-weight: 700; color: #fff; border-radius: 9px; }
  .av-28 { width: 28px; height: 28px; font-size: 11px; border-radius: 7px; }
  .av-36 { width: 36px; height: 36px; font-size: 13px; }
  .av-64 { width: 64px; height: 64px; font-size: 22px; border-radius: 15px; }
  .av-96 { width: 96px; height: 96px; font-size: 32px; border-radius: 22px; }
  .ava-agent { background: rgb(var(--surface)); border: 1.5px dashed rgb(var(--hair2)); color: rgb(var(--meta)); }
  .ava-logo { background: rgb(var(--surface)); border: 1px solid rgb(var(--hair2)); }
  .av-a { background: #3D6BF0; } .av-b { background: #8250DF; } .av-c { background: #0E9888; }
  .av-d { background: #D97706; } .av-e { background: #1FA254; } .av-f { background: #E25C3C; }
```

- [ ] **Step 6: Avatar-Komponente** — `components/avatar.templ`:

```go
package components

// Avatar rendert die Identität eines Namens: getönte Initialen-Kachel
// (Logo-Variante folgt in L2 über NodeLogo). sizeClass: av-28|av-36|av-64|av-96.
templ Avatar(initials string, tone string, sizeClass string) {
	<span class={ "ava", tone, sizeClass } aria-hidden="true">{ initials }</span>
}

// AgentAvatar ist die neutrale, gestrichelte Akteurs-Kachel für Agenten.
templ AgentAvatar(initials string, sizeClass string) {
	<span class={ "ava ava-agent", sizeClass } aria-hidden="true">{ initials }</span>
}
```
(Die Komponente nimmt fertige Strings — `webui.Initials`/`AvatarTone` werden vom Aufrufer gezogen; components bleibt domain-frei.)

- [ ] **Step 7: Render-Test** — in `components/primitives_test.go` ergänzen:

```go
func TestAvatar_RendersToneAndInitials(t *testing.T) {
	var sb strings.Builder
	if err := components.Avatar("BA", "av-c", "av-36").Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	for _, want := range []string{"av-c", "av-36", ">BA<", `aria-hidden="true"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("Avatar output missing %q:\n%s", want, out)
		}
	}
}
```

- [ ] **Step 8: Styleguide-Sektion** — in `styleguide.templ` eine Sektion „Lesesaal" anhängen, die zeigt: `.panel`-Block, alle sechs `.av-*` als `av-36`, alle sechs `.tc-*`-Chips, `.eyebrow`. (Bestehende Sektionen unangetastet — /ui ist die Sichtprobe.)

- [ ] **Step 9: Bauen + Tests + Commit**

Run: `make generate && make web && go test ./internal/adapter/webui/... -race`
Expected: PASS.

```bash
git add -A && git commit -m "feat(lesesaal): Zwei-Flächen-Primitives + Avatar/Typ-Farbsystem (Initials/Tone/ShortName als Single Source)"
```

---

### Task 4: Topbar-Shell — die Sidebar stirbt

**Files:**
- Modify: `internal/adapter/webui/components/appshell.templ` (Komplett-Umbau innen, Signatur bleibt)
- Modify: `internal/adapter/webui/components/sitenav.templ` (PrimaryNav → 3 Bereiche; `SiteNav`-templ löschen; neues `AreaFor`)
- Modify: `internal/adapter/httpserver/server.go` (Route `GET /ui/nav/tree` entfernen)
- Modify: `internal/adapter/httpserver/webui_nav.go` + `webui_nav_test.go` (Tree-Handler entfernen)
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go`
- Test: `internal/adapter/webui/components/shell_test.go`

**Interfaces:**
- Consumes: `Avatar` (Task 3), Token-Utilities (Task 2).
- Produces: Topbar mit Mount `<div id="timer-pill" hx-get="/ui/timer" …>` (Task 5 rendert hinein) und Mount `@PaletteMount()`-Platzhalter-Kommentar (Task 6 ersetzt ihn); `AreaFor(active string) string`; `AppShell`-Signatur UNVERÄNDERT `(active string, breadcrumb, subnav, content templ.Component)`.

- [ ] **Step 1: Failing Shell-Test** — `shell_test.go` um Kernasserts erweitern:

```go
func TestAppShell_TopbarNoSidebar(t *testing.T) {
	var sb strings.Builder
	err := components.AppShell("heute", nil, nil, components.Empty()).Render(testCtx(t), &sb)
	if err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if strings.Contains(out, "<aside") {
		t.Fatal("sidebar <aside> must be gone")
	}
	for _, want := range []string{`id="timer-pill"`, `href="/nodes"`, `href="/wissen"`, `href="/zeit"`, "data-palette-open"} {
		if !strings.Contains(out, want) {
			t.Fatalf("topbar missing %q:\n%s", want, out)
		}
	}
	// active "heute" gehört zum Bereich Zeit
	if !strings.Contains(out, `aria-current="page" href="/zeit"`) && !strings.Contains(out, `href="/zeit" aria-current="page"`) {
		t.Fatal("Zeit area not marked current for active=heute")
	}
	if strings.Contains(out, "/ui/nav/tree") {
		t.Fatal("nav tree mount must be gone")
	}
}
```

- [ ] **Step 2: Laufen lassen** — Expected: FAIL.

- [ ] **Step 3: sitenav.templ umbauen** — Inhalt ersetzen durch:

```go
package components

// NavItem is one navigation destination. LabelKey is an i18n key.
type NavItem struct {
	Key, Href, LabelKey string
}

// PrimaryNav sind die drei Bereiche der Lesesaal-Topbar (Spec §5.1).
func PrimaryNav() []NavItem {
	return []NavItem{
		{"projekte", "/nodes", "nav.projects"},
		{"docs", "/wissen", "nav.wissen"},
		{"zeit", "/zeit", "nav.zeit"},
	}
}

// UtilityNav ist das Avatar-Menü: Frei · Export · Einstellungen.
func UtilityNav() []NavItem {
	return []NavItem{
		{"frei", "/dayoffs", "nav.dayoffs"},
		{"export", "/export", "nav.export"},
		{"einstellungen", "/einstellungen", "nav.settings"},
	}
}

// AreaFor mappt den active-Key einer Seite auf ihren Topbar-Bereich.
// "" (z. B. home) markiert keinen Bereich — die Wortmarke führt zum Schreibtisch.
func AreaFor(active string) string {
	switch active {
	case "projekte":
		return "projekte"
	case "docs":
		return "docs"
	case "zeit", "heute", "woche", "historie", "stats", "frei", "export":
		return "zeit"
	}
	return ""
}
```
(`Glyph` fällt weg — Topbar-Bereiche sind reine Wortmarken. `IsSecondaryNavKey` und `SiteNav` ersatzlos löschen; Compiler zeigt verbliebene Nutzer.)

- [ ] **Step 4: appshell.templ neu** — kompletter neuer Inhalt (BrandMark bleibt erhalten, verliert nur den Gradient):

```go
package components

// BrandMark: schlichte Lesesaal-Wortbild-Raute in Akzent (Gradient tot, Spec §15).
templ BrandMark(sizeClass string) {
	<svg viewBox="0 0 24 24" aria-hidden="true" class={ sizeClass }>
		<g fill="none" stroke="rgb(var(--accent))" stroke-width="1.6" stroke-linejoin="round">
			<path d="M12 3l7 4v10l-7 4-7-4V7z"></path>
			<path d="M12 3v18M5 7l7 4 7-4" opacity=".5"></path>
		</g>
	</svg>
}

// AppShell ist die Lesesaal-Topbar-Shell (Spec §5): Wortmarke · drei Bereiche ·
// Suche (⌘K) · Timer-Pill · Avatar-Menü. Kein Seitenbaum, keine Bottom-Tabs.
// Signatur bleibt stabil — Seiten-Caller sind unberührt.
templ AppShell(active string, breadcrumb, subnav, content templ.Component) {
	<header class="sticky top-0 z-40 bg-paper/90 backdrop-blur-sm border-b border-hair">
		<div class="mx-auto w-full max-w-[1140px] px-4 sm:px-7 h-[58px] flex items-center gap-4 sm:gap-6">
			<a href="/" class="inline-flex items-center gap-2 shrink-0" aria-label={ T(ctx, "app.name") }>
				@BrandMark("h-5 w-5")
				<span class="font-display text-[19px] font-bold tracking-tight">{ T(ctx, "app.name") }<span class="text-accent">.</span></span>
			</a>
			<nav class="flex items-center gap-4 sm:gap-6 min-w-0" aria-label={ T(ctx, "nav.primary") }>
				for _, l := range PrimaryNav() {
					if l.Key == AreaFor(active) {
						<a aria-current="page" href={ templ.SafeURL(l.Href) } class="text-[14.5px] font-medium text-ink border-b-2 border-accent pt-[18px] pb-[16px]">{ T(ctx, l.LabelKey) }</a>
					} else {
						<a href={ templ.SafeURL(l.Href) } class="text-[14.5px] font-medium text-muted hover:text-ink border-b-2 border-transparent pt-[18px] pb-[16px]">{ T(ctx, l.LabelKey) }</a>
					}
				}
			</nav>
			<div class="flex-1"></div>
			<button type="button" data-palette-open aria-label={ T(ctx, "palette.open") }
				class="flex items-center gap-2 rounded-lg border border-hair2 bg-surface px-2.5 py-[7px] text-[13.5px] text-faint hover:border-faint hover:text-meta">
				<span aria-hidden="true">⌕</span>
				<span class="hidden sm:inline">{ T(ctx, "palette.hint") }</span>
				<kbd class="hidden md:inline font-mono text-[11px] border border-hair rounded px-1.5 bg-surface text-faint">⌘K</kbd>
			</button>
			<div
				id="timer-pill"
				hx-get="/ui/timer"
				hx-trigger="load, sse:session.started, sse:session.stopped, sse:session.updated, sse:session.deleted"
				hx-swap="innerHTML"
			></div>
			<button type="button" data-dialog-open="user-menu" aria-label={ T(ctx, "nav.menu") } class="shrink-0">
				<span class="ava av-e av-28" style="width:32px;height:32px;border-radius:8px" aria-hidden="true">{ T(ctx, "app.userInitials") }</span>
			</button>
		</div>
	</header>
	<main class="pb-16">
		<div class="mx-auto w-full max-w-[1140px] px-4 sm:px-7 pt-6 md:pt-8">
			if breadcrumb != nil {
				@breadcrumb
			}
			if subnav != nil {
				@subnav
			}
			@content
		</div>
	</main>
	// Avatar-Menü: Werkzeuge + Abmelden (ersetzt Sidebar-Sekundärnav + More-Drawer)
	<dialog id="user-menu" aria-modal="true" aria-labelledby="user-menu-title"
		class="fixed m-0 mt-[64px] ml-auto mr-4 w-[240px] rounded-[14px] border border-hair2 bg-surface text-ink p-0 shadow-lift backdrop:bg-ink/20">
		<div class="px-4 pt-3 pb-2 border-b border-hair">
			<span id="user-menu-title" class="eyebrow">{ T(ctx, "nav.menu") }</span>
		</div>
		<div class="py-1.5">
			for _, l := range UtilityNav() {
				<a href={ templ.SafeURL(l.Href) } data-dialog-close class="block px-4 py-2.5 text-[14px] font-medium text-body hover:bg-wash hover:text-ink">{ T(ctx, l.LabelKey) }</a>
			}
			<form action="/auth/logout" method="post" hx-boost="false" class="border-t border-hair mt-1.5 pt-1.5">
				<button class="w-full text-left px-4 py-2.5 text-[14px] font-medium text-meta hover:bg-wash hover:text-ink">{ T(ctx, "nav.logout") }</button>
			</form>
		</div>
	</dialog>
	<script src="/static/js/dialog.js" defer></script>
}
```

- [ ] **Step 5: Nav-Tree-Route entwirren** — in `server.go` die Zeile `mux.Handle("GET /ui/nav/tree", …)` löschen; in `webui_nav.go` NUR den Tree-Handler (und seine Tests in `webui_nav_test.go`) entfernen. Danach prüfen, ob `navtree.templ`/`node_tree_vm.go` noch Nutzer haben:

```bash
rg -n "NavTree|TreeRow|FillTreeHours|SubtreeHourTotals" internal/ -g '!*_templ.go' -g '!*_test.go'
```
Nur wenn es KEINE Treffer außerhalb von `navtree.templ`/`node_tree_vm.go`/`webui_nav.go` gibt: diese Dateien (+ zugehörige Tests + `navtree_templ.go`) per `git rm` löschen. Gibt es Treffer (z. B. /nodes-Seite), bleiben die Dateien — nur Route + Sidebar-Mount sind tot.

- [ ] **Step 6: i18n-Schlüssel** — in BEIDEN Katalogen ergänzen/prüfen:

```go
"palette.open":     "Suchen und springen",   // en: "Search and jump"
"palette.hint":     "Suchen …",              // en: "Search …"
"app.userInitials": "MS",                    // en: "MS" (bis echte User-Profile kommen)
```
`nav.more` wird nutzerlos → aus beiden Katalogen entfernen (rg-Check auf Restnutzung zuerst).

- [ ] **Step 7: Bauen + Tests reparieren**

Run: `make generate && make web && go test ./... -race 2>&1 | tail -30`
Expected: `shell_test` PASS; Tests, die Sidebar/Bottom-Tab/More-Drawer asserten (coverage_test, webauth_test-Snippets, nav-Tests), auf die Topbar-Realität anpassen. `webui_nav_test.go` schrumpft auf das, was bleibt.

- [ ] **Step 8: Commit**

```bash
git add -A && git commit -m "feat(lesesaal): Topbar-Shell ersetzt Sidebar — drei Bereiche, Suche, Timer-Pill-Mount, Avatar-Menü"
```

---

### Task 5: Timer-Pill — der eine Timer in der Topbar

**Files:**
- Modify: `internal/adapter/webui/timerwidget.templ` (TimerPill neu; TimerWidget wird Sheet-Body; TimerChip stirbt)
- Modify: `internal/adapter/httpserver/webui_timer.go` (`handleTimerWidget` rendert Pill; `handleTimerChip` löschen)
- Modify: `internal/adapter/httpserver/server.go` (Route `GET /ui/timer/chip` löschen)
- Test: `internal/adapter/webui/timerwidget_render_test.go`, `internal/adapter/httpserver/webui_timer_test.go`

**Interfaces:**
- Consumes: Mount `#timer-pill` (Task 4), `webui.ShortName` (Task 3), bestehende `TimerWidgetVM` + `BuildTimerWidgetVM` (unverändert), Dialog-Mechanik `data-dialog-open`.
- Produces: `TimerPill(vm TimerWidgetVM)` — wird von `handleTimerWidget` in den Mount gerendert. Start/Stop/Switch-Handler + deren Routen bleiben UNVERÄNDERT (Formulare posten weiter aus dem Sheet, SSE refresht den Mount).

- [ ] **Step 1: Failing Render-Test** — in `timerwidget_render_test.go`:

```go
func TestTimerPill_RunningShowsClockAndShortName(t *testing.T) {
	vm := TimerWidgetVM{Running: true, BaseSeconds: 754, NodeID: "n1",
		NodeName: "gitlab.com/dataalliance/products/foolu/product/backstage", NodeKind: domain.NodeKindRepo}
	out := render(t, TimerPill(vm)) // render-Helper wie in den Nachbartests
	for _, want := range []string{"data-timer", `data-base="754"`, ">backstage<", "timer-sheet"} {
		if !strings.Contains(out, want) {
			t.Fatalf("pill missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, vm.NodeName+"</a>") {
		t.Fatal("pill must show ShortName, not the full remote path")
	}
}

func TestTimerPill_IdleIsQuietStart(t *testing.T) {
	out := render(t, TimerPill(TimerWidgetVM{Running: false}))
	if !strings.Contains(out, "data-dialog-open=\"timer-sheet\"") {
		t.Fatal("idle pill must open the timer sheet")
	}
	if strings.Contains(out, "cta-glow") {
		t.Fatal("no glow CTAs in Lesesaal")
	}
}
```

- [ ] **Step 2: Laufen lassen** — Expected: FAIL (undefined: TimerPill).

- [ ] **Step 3: TimerPill implementieren** — in `timerwidget.templ` ERGÄNZEN und `TimerChip`+`timerChipBody` LÖSCHEN:

```go
// TimerPill ist DAS eine Timer-Instrument (Spec §10): läuft → Live-Punkt +
// tickende Mono-Uhr + Kurzname (Link zum Knoten); idle → ruhiger Start, der
// das Timer-Sheet (TimerWidget als Dialog-Body) öffnet.
templ TimerPill(vm TimerWidgetVM) {
	if vm.Running {
		<span class="inline-flex items-center gap-2 rounded-full border border-hair2 bg-surface pl-2.5 pr-3 py-[5px]">
			<span class="h-[7px] w-[7px] rounded-full bg-live-bright shadow-[0_0_0_3px_rgb(var(--live-wash))]" aria-hidden="true"></span>
			<span class="font-mono tnum text-[13px] font-semibold" data-timer data-timer-fmt="clock" data-base={ strconv.FormatInt(vm.BaseSeconds, 10) } role="timer">{ fmtClockHMS(vm.BaseSeconds) }</span>
			if vm.NodeName != "" {
				<a href={ templ.SafeURL("/nodes/" + vm.NodeID) } title={ vm.NodeName } class="hidden sm:inline max-w-[110px] truncate text-[12.5px] text-meta hover:text-accent">{ ShortName(vm.NodeName) }</a>
			}
			<button type="button" data-dialog-open="timer-sheet" aria-label={ components.T(ctx, "timer.title") } class="text-meta hover:text-ink text-[12px]">▾</button>
		</span>
	} else {
		<button type="button" data-dialog-open="timer-sheet"
			class="inline-flex items-center gap-1.5 rounded-full border border-hair2 bg-surface px-3 py-[5px] text-[13px] font-medium text-meta hover:text-ink hover:border-faint">
			<span aria-hidden="true">▶</span> { components.T(ctx, "timer.start") }
		</button>
	}
	@components.Dialog("timer-sheet", "timer.title", timerSheetBody(vm), false)
}

templ timerSheetBody(vm TimerWidgetVM) {
	@TimerWidget(vm)
}
```
Im verbleibenden `TimerWidget` die Karten-Hülle beruhigen: `class="rounded-2xl glass shadow-soft p-3 …"` → `class="panel p-4 text-[.9rem]"` (Zwei-Flächen-Regel: das Sheet ist ein Instrument).

- [ ] **Step 4: Handler + Route** — in `webui_timer.go`: `handleTimerWidget` rendert `webui.TimerPill(vm)` statt `webui.TimerWidget(vm)`; `handleTimerChip` komplett löschen. In `server.go` die Zeile `mux.Handle("GET /ui/timer/chip", …)` löschen. Die Start/Stop/Switch-Handler NICHT anfassen (sie rendern weiter `TimerWidget` für den Section-Swap im Sheet; der Mount refresht via SSE).

- [ ] **Step 5: Tests reparieren + laufen lassen**

Run: `go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -race 2>&1 | tail -20`
Expected: Pill-Tests PASS; Chip-Route-Tests in `webui_timer_test.go` löschen; Widget-Tests (Start/Stop/Switch-Flows) bleiben grün.

- [ ] **Step 6: Bauen + Commit**

```bash
make generate && make web
git add -A && git commit -m "feat(lesesaal): Timer-Pill in der Topbar — Chip-Route stirbt, Start/Stop/Switch unverändert"
```

---

### Task 6: ⌘K-Palette — fuzzy springen zu Knoten und Dokumenten

**Files:**
- Create: `internal/adapter/webui/palette.templ` + `internal/adapter/webui/palette_vm.go` + `internal/adapter/webui/palette_vm_test.go`
- Create: `internal/adapter/webui/static/js/palette.js`
- Create: `internal/adapter/httpserver/webui_palette.go` + `webui_palette_test.go`
- Modify: `internal/adapter/webui/components/appshell.templ` (Palette-Dialog + Script einhängen)
- Modify: `internal/adapter/httpserver/server.go` (Route)
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go`

**Interfaces:**
- Consumes: `fuzzymatch.Match(query, target string) (idx []int, score int, ok bool)` aus `internal/tui/ui/fuzzymatch` (domain-frei); `ports.NodeStore.List(ctx, ownerID)`; `ports.DocumentStore.ListPage(ctx, ownerID, nil, 200, 0)`; `ports.SessionStore.List(ctx, ownerID, since)` für MRU; `webui.ShortName/Initials/AvatarTone` + `components.Avatar`.
- Produces: `GET /ui/palette?q=` (webAuth) → `PaletteResults(vm)`-Fragment; `webui.BuildPaletteVM(nodes []domain.Node, recentNodeIDs []string, docs []domain.Document, q string) PaletteVM`.

- [ ] **Step 1: Failing VM-Test** — `palette_vm_test.go`:

```go
package webui

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestBuildPaletteVM_FuzzyAndOrder(t *testing.T) {
	nodes := []domain.Node{
		{ID: "n1", Name: "gitlab.com/dataalliance/products/foolu/product/backstage", Kind: domain.NodeKindRepo},
		{ID: "n2", Name: "RTL Extern", Kind: domain.NodeKindEngagement},
		{ID: "n3", Name: "github.com/serverkraken/flow", Kind: domain.NodeKindRepo},
	}
	docs := []domain.Document{
		{ID: "d1", Title: "Kompendium-Integration in flow", Path: "notes/kompendium"},
		{ID: "d2", Title: "Backstage Probleme", Path: "docs/group-processor-token-reuse"},
	}
	// fuzzy: "kompend" findet das Kompendium-Doc (Soenne-Gesetz)
	vm := BuildPaletteVM(nodes, nil, docs, "kompend")
	if len(vm.Docs) != 1 || vm.Docs[0].Title != "Kompendium-Integration in flow" {
		t.Fatalf("fuzzy recall failed: %+v", vm.Docs)
	}
	// leere Query: MRU-Knoten zuerst, dann Rest; Docs in gegebener (updated-desc) Reihenfolge
	vm = BuildPaletteVM(nodes, []string{"n3"}, docs, "")
	if vm.Nodes[0].ID != "n3" {
		t.Fatalf("MRU node not first: %+v", vm.Nodes)
	}
	// Kurzname + voller Pfad getrennt
	var bs PaletteNodeVM
	for _, n := range vm.Nodes {
		if n.ID == "n1" {
			bs = n
		}
	}
	if bs.Short != "backstage" || bs.Full != "gitlab.com/dataalliance/products/foolu/product/backstage" {
		t.Fatalf("short/full wrong: %+v", bs)
	}
}
```

- [ ] **Step 2: Laufen lassen** — Expected: FAIL (undefined: BuildPaletteVM).

- [ ] **Step 3: palette_vm.go implementieren**

```go
package webui

import (
	"sort"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/ui/fuzzymatch"
)

// PaletteNodeVM / PaletteDocVM sind je eine Sprungzeile der ⌘K-Palette.
type PaletteNodeVM struct {
	ID, Short, Full, Initials, Tone, Kind string
	Score                                 int
}

type PaletteDocVM struct {
	ID, Title, Path, Type string
	Score                 int
}

type PaletteVM struct {
	Query string
	Nodes []PaletteNodeVM
	Docs  []PaletteDocVM
}

const paletteMaxRows = 8

// BuildPaletteVM filtert Knoten + Dokumente fuzzy (Spec §5.4). Leere Query:
// recentNodeIDs (MRU) zuerst, dann alle Knoten alphabetisch; Docs kommen in
// der gelieferten (updated-desc) Reihenfolge. Mit Query gewinnt der Score.
func BuildPaletteVM(nodes []domain.Node, recentNodeIDs []string, docs []domain.Document, q string) PaletteVM {
	vm := PaletteVM{Query: q}
	recent := map[string]int{}
	for i, id := range recentNodeIDs {
		recent[id] = len(recentNodeIDs) - i // jünger = höher
	}
	for _, n := range nodes {
		short := ShortName(n.Name)
		row := PaletteNodeVM{ID: n.ID, Short: short, Full: n.Name,
			Initials: Initials(short), Tone: AvatarTone(n.Name), Kind: string(n.Kind)}
		if q == "" {
			row.Score = recent[n.ID]
			vm.Nodes = append(vm.Nodes, row)
			continue
		}
		if _, s, ok := fuzzymatch.Match(q, short); ok {
			row.Score = s + 1000 // Kurzname-Treffer schlagen Pfad-Treffer
		} else if _, s, ok := fuzzymatch.Match(q, n.Name); ok {
			row.Score = s
		} else {
			continue
		}
		vm.Nodes = append(vm.Nodes, row)
	}
	sort.SliceStable(vm.Nodes, func(i, j int) bool { return vm.Nodes[i].Score > vm.Nodes[j].Score })
	if len(vm.Nodes) > paletteMaxRows {
		vm.Nodes = vm.Nodes[:paletteMaxRows]
	}
	for i, d := range docs {
		row := PaletteDocVM{ID: d.ID, Title: d.Title, Path: d.Path, Type: string(d.Type)}
		if q == "" {
			row.Score = len(docs) - i
			vm.Docs = append(vm.Docs, row)
			continue
		}
		if _, s, ok := fuzzymatch.Match(q, d.Title); ok {
			row.Score = s + 1000
		} else if _, s, ok := fuzzymatch.Match(q, d.Path); ok {
			row.Score = s
		} else {
			continue
		}
		vm.Docs = append(vm.Docs, row)
	}
	sort.SliceStable(vm.Docs, func(i, j int) bool { return vm.Docs[i].Score > vm.Docs[j].Score })
	if len(vm.Docs) > paletteMaxRows {
		vm.Docs = vm.Docs[:paletteMaxRows]
	}
	return vm
}
```
(Feldnamen `domain.Document.Type/Title/Path` und `domain.Node.Kind` vor dem Tippen per `rg "type Document struct" -A12 internal/domain/` verifizieren und exakt übernehmen.)

- [ ] **Step 4: VM-Test grün** — `go test ./internal/adapter/webui/ -run TestBuildPaletteVM -race` → PASS.

- [ ] **Step 5: palette.templ**

```go
package webui

import "github.com/serverkraken/flow/internal/adapter/webui/components"

// PaletteDialog ist der ⌘K-Überbau (in der AppShell gemountet); die Zeilen
// kommen per htmx aus /ui/palette in #palette-results.
templ PaletteDialog() {
	<dialog id="palette" aria-modal="true" aria-label={ components.T(ctx, "palette.open") }
		class="fixed left-1/2 top-[16%] -translate-x-1/2 m-0 w-[min(600px,92vw)] rounded-[14px] border border-hair2 bg-surface p-0 shadow-lift backdrop:bg-ink/25">
		<input
			id="palette-input"
			type="text"
			name="q"
			autocomplete="off"
			placeholder={ components.T(ctx, "palette.placeholder") }
			class="w-full border-0 border-b border-hair bg-transparent px-5 py-4 text-[16px] text-ink outline-none placeholder:text-faint"
			hx-get="/ui/palette"
			hx-trigger="input changed delay:120ms, load"
			hx-target="#palette-results"
			hx-swap="innerHTML"
		/>
		<div id="palette-results" class="max-h-[320px] overflow-y-auto p-1.5"></div>
		<div class="flex gap-4 border-t border-hair px-4 py-2 text-[11.5px] text-faint">
			<span>↑↓ { components.T(ctx, "palette.select") }</span>
			<span>↵ { components.T(ctx, "palette.openRow") }</span>
			<span>esc { components.T(ctx, "common.close") }</span>
		</div>
	</dialog>
	<script src="/static/js/palette.js" defer></script>
}

// PaletteResults ist das htmx-Fragment: Knoten-Zeilen, dann Dokument-Zeilen.
templ PaletteResults(vm PaletteVM) {
	if len(vm.Nodes) == 0 && len(vm.Docs) == 0 {
		<p class="px-4 py-3 text-[13.5px] text-meta">{ components.T(ctx, "palette.empty") }</p>
	}
	for _, n := range vm.Nodes {
		<a href={ templ.SafeURL("/nodes/" + n.ID) } data-palette-row
			class="flex items-center gap-3 rounded-lg px-3 py-2 text-[14px] hover:bg-wash aria-selected:bg-wash" >
			@components.Avatar(n.Initials, n.Tone, "av-28")
			<span class="font-medium">{ n.Short }</span>
			<span class="ml-auto max-w-[46%] truncate font-mono text-[11px] text-faint">{ n.Full }</span>
		</a>
	}
	for _, d := range vm.Docs {
		<a href={ templ.SafeURL("/wissen/" + d.ID) } data-palette-row
			class="flex items-center gap-3 rounded-lg px-3 py-2 text-[14px] hover:bg-wash aria-selected:bg-wash">
			<span class="typechip">{ d.Type }</span>
			<span class="min-w-0 truncate">{ d.Title }</span>
			<span class="ml-auto max-w-[38%] truncate font-mono text-[11px] text-faint">{ d.Path }</span>
		</a>
	}
}
```
(Die Doc-Href `/wissen/{id}` an der real existierenden Wissen-Detailroute verifizieren: `rg -n '"GET /wissen' internal/adapter/httpserver/server.go` — exakt die dort registrierte Form verwenden.)

- [ ] **Step 6: static/js/palette.js**

```js
// ⌘K-Palette: öffnen/fokussieren, Pfeil-Navigation über [data-palette-row],
// Enter folgt der markierten Zeile, Esc schließt. Kein Framework.
(function () {
  var dlg, input, sel = -1;
  function rows() { return dlg ? Array.prototype.slice.call(dlg.querySelectorAll('[data-palette-row]')) : []; }
  function mark(n) {
    var r = rows();
    sel = Math.max(0, Math.min(n, r.length - 1));
    r.forEach(function (el, i) { el.setAttribute('aria-selected', i === sel ? 'true' : 'false'); });
    if (r[sel]) r[sel].scrollIntoView({ block: 'nearest' });
  }
  function open() {
    dlg = document.getElementById('palette');
    input = document.getElementById('palette-input');
    if (!dlg) return;
    dlg.showModal();
    input.value = '';
    input.focus();
    sel = -1;
  }
  document.addEventListener('keydown', function (e) {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      dlg && dlg.open ? dlg.close() : open();
      return;
    }
    if (!dlg || !dlg.open) return;
    if (e.key === 'ArrowDown') { e.preventDefault(); mark(sel + 1); }
    if (e.key === 'ArrowUp') { e.preventDefault(); mark(sel - 1); }
    if (e.key === 'Enter') {
      var r = rows();
      if (r[sel >= 0 ? sel : 0]) { e.preventDefault(); window.location.href = r[sel >= 0 ? sel : 0].href; }
    }
  });
  document.addEventListener('click', function (e) {
    var b = e.target.closest('[data-palette-open]');
    if (b) { e.preventDefault(); open(); }
    if (dlg && dlg.open && e.target === dlg) dlg.close(); // Klick auf Backdrop
  });
  document.body.addEventListener('htmx:afterSwap', function (e) {
    if (e.target && e.target.id === 'palette-results') mark(-1);
  });
})();
```

- [ ] **Step 7: Handler + Route + Mount** — `webui_palette.go`:

```go
package httpserver

import (
	"net/http"
	"sort"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
)

// handlePalette liefert die ⌘K-Sprungzeilen: Knoten (MRU aus den letzten 30
// Tagen Sessions) + jüngste Dokumente, fuzzy gefiltert über q.
func (s *Server) handlePalette(w http.ResponseWriter, r *http.Request) {
	owner := actorID(r.Context()) // exakt den Helper der Nachbar-Handler verwenden (rg "ownerID :=" webui_timer.go)
	q := r.URL.Query().Get("q")

	nodes, err := s.Nodes.List(r.Context(), owner)
	if err != nil {
		http.Error(w, "list nodes", http.StatusInternalServerError)
		return
	}
	sessions, err := s.Sessions.List(r.Context(), owner, s.Clock.Now().Add(-30*24*time.Hour))
	if err != nil {
		sessions = nil // MRU ist Komfort — degradiert still
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].StartAt.After(sessions[j].StartAt) })
	var recent []string
	seen := map[string]bool{}
	for _, ws := range sessions {
		if ws.NodeID != nil && !seen[*ws.NodeID] {
			seen[*ws.NodeID] = true
			recent = append(recent, *ws.NodeID)
		}
	}
	docs, _, err := s.Documents.ListPage(r.Context(), owner, nil, 200, 0)
	if err != nil {
		http.Error(w, "list documents", http.StatusInternalServerError)
		return
	}
	vm := webui.BuildPaletteVM(nodes, recent, docs, q)
	_ = webui.PaletteResults(vm).Render(r.Context(), w)
}
```
(Feld-/Helper-Namen `actorID`, `s.Sessions`, `ws.StartAt/NodeID`, `s.Clock` vor dem Tippen mit `rg` an `webui_timer.go`/`ports.go` abgleichen und exakt übernehmen — der Server-Struct hat für alles bereits Felder, **kein** neuer Konstruktor-Parameter, also KEIN `cmd/flow-server/main.go`-Change.)
Route in `server.go` neben den Timer-Routen: `mux.Handle("GET /ui/palette", s.webAuth(http.HandlerFunc(s.handlePalette)))`.
Mount: in `appshell.templ` direkt vor `<script src="/static/js/dialog.js" …>` die Zeile `@webuiPaletteMount()` — ACHTUNG Paketgrenze: `PaletteDialog` liegt im Paket `webui`, die Shell in `components`. Lösung: `PaletteDialog` nach `components/palette.templ` legen und die Zeilen-Templates (`PaletteResults`) im Paket `webui` lassen (VM-Typen wandern per Parameter: `PaletteResults` braucht nur `PaletteVM` aus `webui` — Import-Richtung `webui → components` ist die bestehende; also: Dialog-Hülle in `components` (statischer Rahmen, keine VM), Ergebnis-Fragment in `webui`. Der Handler rendert nur das Fragment.)

- [ ] **Step 8: Handler-Test** — `webui_palette_test.go` (Muster + Fakes der Nachbardatei `webui_timer_test.go` übernehmen):

```go
func TestPalette_FuzzyFindsNodeAndDoc(t *testing.T) {
	srv := newTestServerWithData(t) // Helper der Nachbartests: Fake-Stores + eingeloggte Session
	rec := doAuthedGet(t, srv, "/ui/palette?q=backst")
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(body, ">backstage<") {
		t.Fatalf("node row missing:\n%s", body)
	}
	if !strings.Contains(body, "Backstage Probleme") {
		t.Fatalf("doc row missing:\n%s", body)
	}
}

func TestPalette_RequiresAuth(t *testing.T) {
	srv := newTestServer(t)
	rec := doAnonGet(t, srv, "/ui/palette")
	if rec.Code == http.StatusOK {
		t.Fatal("palette must not be public")
	}
}
```
(Die konkreten Test-Helper-Namen aus `webui_timer_test.go` übernehmen — nicht neu erfinden.)

- [ ] **Step 9: i18n** — beide Kataloge:

```go
"palette.placeholder": "Springen zu Projekt oder Dokument …",  // en: "Jump to project or document …"
"palette.select":      "wählen",                                // en: "select"
"palette.openRow":     "öffnen",                                // en: "open"
"palette.empty":       "Kein Treffer.",                         // en: "No match."
```

- [ ] **Step 10: Bauen + alle Tests + Commit**

Run: `make generate && make web && go test ./... -race 2>&1 | tail -20`
Expected: PASS.

```bash
git add -A && git commit -m "feat(lesesaal): ⌘K-Palette — fuzzy Knoten+Dokumente, MRU zuerst, htmx-Fragment + vendored JS"
```

---

### Task 7: Wiring-Gate — Live-Smoke, Leichen-Sweep, volles CI

**Files:**
- Modify: nur was der Sweep findet (tote i18n-Keys, tote CSS-Klassen, Testreste)

**Interfaces:** — (Verifikations-Task; Pflicht nach [[feedback_plan_main_wiring_task]])

- [ ] **Step 1: Leichen-Sweep**

```bash
rg -n "ClashDisplay|Inter-Variable|kristall-facets|toggleTheme|ThemeToggle|TimerChip|/ui/timer/chip|/ui/nav/tree|SiteNav|IsSecondaryNavKey|nav.more|cta-glow|glass-strong" \
  internal/ web/ --glob '!*_templ.go'
```
Expected: 0 Treffer (Ausnahme: der `.glass`-Übergangs-Alias in tailwind.css und historische Kommentare). Jeden echten Treffer beseitigen.

- [ ] **Step 2: Guards + volles CI**

Run: `make ci`
Expected: GREEN — lint, verify-generate, verify-css, verify-no-popups, cover ≥75 %, build.

- [ ] **Step 3: Live-Smoke gegen den Dev-Stack** (Postgres+Dex laufen; sonst `make dev-up`)

```bash
make dev-run &   # Server auf https://localhost:8080 (self-signed)
sleep 2
TOKEN=$(make -s dev-token)
for p in / /nodes /wissen /zeit "/ui/palette?q=ko" /ui/timer; do
  code=$(curl -sk -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $TOKEN" "https://localhost:8080$p")
  echo "$p -> $code"
done
```
Expected: `/ui/palette?q=ko` und `/ui/timer` → 200 mit Bearer; die Seiten-Routen antworten 200 (oder 303 auf Login bei Cookie-Pfad — wie bisher). Danach Server stoppen.

- [ ] **Step 4: Browser-Sichtprobe für Soenne vorbereiten** — kurz notieren (im PR-/Abschlusstext): Topbar auf jeder Seite, ⌘K öffnet+filtert+springt, Timer-Pill tickt und Start/Stop über das Sheet funktioniert, /ui-Styleguide zeigt die Lesesaal-Sektion, kein horizontales Scrollen auf iPhone-Breite (375px, DevTools).

- [ ] **Step 5: Abschluss-Commit (falls der Sweep etwas fand)**

```bash
git add -A && git commit -m "chore(lesesaal): L1-Gate — Leichen-Sweep + Live-Smoke"
```

---

## Self-Review (ausgeführt beim Schreiben)

- **Spec-Deckung L1** (§17): Tokens ✓ (T2) · Fonts ✓ (T1) · Zwei-Flächen-Primitives ✓ (T3) · Topbar-Shell ✓ (T4) · Timer-Pill ✓ (T5) · ⌘K-Palette ✓ (T6) · Eindämmung html/body-clip ✓ (T2 Step 5) · Gate ✓ (T7). NICHT in L1 (bewusst, Spec §17): Seiten-Umbauten (L2–L4), Kurznamen-Dedup (L2), Logo-Groß-Render (L2), Dunkel-Zwilling (L7).
- **Platzhalter:** keine TBDs; wo Bestands-Helfernamen variieren können (Test-Helper, actorID, Wissen-Detailroute), steht der exakte `rg`-Verifikationsschritt daneben.
- **Typ-Konsistenz:** `TimerPill(vm TimerWidgetVM)` (T5) nutzt `ShortName` aus T3; `BuildPaletteVM`-Signatur in T6 Step 1/3 identisch; `Avatar(initials, tone, sizeClass)` überall dreistellig.
