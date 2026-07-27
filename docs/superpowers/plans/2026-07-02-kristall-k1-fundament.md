# Kristall K1 — Fundament (Dark-Default, Canvas, Primitives, Sidebar, Timer-Widget) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Das Kristall-Design wird app-weit sichtbar: **Dark wird Default** (Light bleibt abgeleitet), Canvas/Facets ziehen aufs approved Mockup, die Kern-Primitives (Button-CTA, StatTile, TabStrip) bekommen die Mockup-Optik, die **Sidebar** wird zur schwebenden Glas-Karte mit form-codiertem Baum (● ◆ ⬡, Fade, Stunden-Badges), und das **globale Timer-Widget** landet in der Shell (Desktop-Karte + Mobile-Chip) — der erste Schritt der IA-Konsolidierung.

**Architecture:** Die Kristall-Token-Schicht existiert bereits (M1-Reframe: `web/tailwind.css` mit Light/Dark-Twilight, `.glass`, `.kristall-facets`, No-Flash-Theme in `components.Base`) — K1 flippt den Default, hebt Backdrop/Facets/Primitives auf Mockup-Norm und macht `ColorHex` token-reaktiv (`rgb(var(--x))` statt festem Hex). Sidebar/Widget folgen dem etablierten htmx-Fragment-Muster (`/ui/nav/tree` → neu `/ui/timer`); das Widget wird per leerem Container in `AppShell` gemountet (kein Import-Zyklus components→webui). Session-Mutationen laufen über die bestehenden Usecases + `s.sessionEventData` (Slice 3).

**Tech Stack:** Go, templ + htmx + Tailwind v4 (Token-CSS in `web/tailwind.css`), SSE, `make generate`/`make web`/`make ci`.

## Global Constraints
- Branch: `cockpit-story`, Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`. Spec: `docs/superpowers/specs/2026-07-02-kristall-redesign-design.md` §5 (+§0 Grundsätze, §3.1 Widget). Mockup normativ: `docs/superpowers/specs/assets/2026-07-01-cockpit-story/direction-b-APPROVED.html`.
- **MULTI-TENANT** (AGENTS.md §Grundsätze): alle neuen Queries owner-scoped; „ist nur ein User"-Begründungen unzulässig (auch in Kommentaren/Reports).
- **K1 entfernt NICHTS**: Home-/Heute-Start/Stop-Formulare bleiben bis K3 (dokumentierter Übergangszustand). Das Widget kommt additiv dazu.
- `make ci` grün pro Task (Gate 75 %, `*_templ.go` ausgeschlossen, echte output-asserting Tests). Nach `.templ`: `make generate`, generierte Dateien VOR `make ci` stagen. Neue Tailwind-Klassen/CSS: `make web` + `internal/adapter/webui/static/app.css` committen (`verify-css` ist Schiedsrichter; Tailwind scannt auch Doc-Kommentare). **Kein `make fmt`.** Keine Emojis. Keine Browser-Popups (Dialog-Komponente). i18n de+en Parität (test-enforced). Events via `s.Emitter.Emit` mit `Data: s.sessionEventData(...)`.
- Zeilennummern = Stand `3a71716`; bei Drift den Code-Auszügen/Bezeichnern vertrauen.
- Falls pgstore-Docker-Tests den Daemon nicht erreichen: `DOCKER_HOST` auf den podman-Socket, einmal retry.

---

## File Structure
**Create:**
- `internal/adapter/webui/timerwidget.templ` + `timerwidget_vm.go` — Widget (Karte + Chip-Variante) + VM.
- `internal/adapter/httpserver/webui_timer.go` — Fragment-/Aktions-Handler.
- Tests: `internal/adapter/httpserver/webui_timer_test.go`, `internal/adapter/webui/timerwidget_render_test.go` (intern), NavTree-Assertions in bestehender Testdatei.

**Modify:**
- `web/tailwind.css` — Token-Flip (`:root`=dark, `[data-theme=light]`=light), Backdrop-Radials, `.glass` blur 16, `--shadow-lift`, neue `@layer components`-Klassen (`.pill-tabs*`, `.rtile-ac`, `.nvdot*`, `.fade-label`, `.cta-glow`).
- `internal/adapter/webui/components/base.templ` — No-Flash-Default → dark; Facets-SVG → Mockup-Polygone (token-getönt).
- `internal/adapter/webui/components/button.templ` — Primär → Gradient-CTA.
- `internal/adapter/webui/components/stattile.templ` — rtile-Optik (+ optionaler Akzent).
- `internal/adapter/webui/components/subnav.templ` — Pill-Tabs + optionaler Count.
- `internal/adapter/webui/components/appshell.templ` — Sidebar als schwebende Glas-Karte; Widget-Mounts (Desktop + Mobile-Chip + Dialog).
- `internal/adapter/webui/nodestyle.go` — `ColorHex` → `rgb(var(--x))` (+ Drift-Guard-Test anpassen; Slice-3-Assertion `#7dcfff` → `rgb(var(--cyan))`).
- `internal/adapter/webui/navtree.templ` + `node_tree_vm.go` (`TreeRow.Hours`) — Baum-Rework.
- `internal/adapter/httpserver/webui_nav.go` — Stunden-Aggregation.
- `internal/adapter/httpserver/server.go` — Routen `GET /ui/timer`, `POST /ui/timer/{start|stop|switch}`.
- `internal/i18n/catalog_de.go` / `catalog_en.go` — `timer.*`-Keys.
- `internal/adapter/webui/components/styleguide.templ` — neue Primitives-Showcases.

---

## Task 1: Dark-Default-Flip + token-reaktives `ColorHex`

**Files:** Modify `web/tailwind.css` (Z.25–113), `internal/adapter/webui/components/base.templ` (Z.24–34), `internal/adapter/webui/nodestyle.go`, `internal/adapter/webui/nodestyle_test.go`, `internal/adapter/webui/cockpit_render_test.go` (Slice-3-Assertion).

**Interfaces:**
- Produces: `:root` = Kristall-Dark, `[data-theme="light"]` = Light-Ableitung; `webui.ColorHex(name) string` liefert `"rgb(var(--<name>))"` (`""` für unset/unknown — Name unverändert, Semantik „CSS-Farb-Ausdruck").

- [ ] **Step 1: Failing Test** — `nodestyle_test.go`: `TestColorHexCoversWholePalette` umschreiben:

```go
// Drift guard: every domain palette color MUST map to a token-reactive CSS
// color expression, else a node could carry a color the WebUI renders blank —
// and inline hexes would not flip with the theme.
func TestColorHexCoversWholePalette(t *testing.T) {
	for _, name := range domain.NodeColors {
		got := webui.ColorHex(name)
		want := "rgb(var(--" + name + "))"
		if got != want {
			t.Errorf("color %q → %q, want %q", name, got, want)
		}
	}
	if webui.ColorHex("") != "" {
		t.Error("empty color → empty expression")
	}
	if webui.ColorHex("chartreuse") != "" {
		t.Error("unknown color → empty expression (not a guess)")
	}
}
```

- [ ] **Step 2: Run — fails.** `go test ./internal/adapter/webui/ -run TestColorHexCoversWholePalette` → FAIL (liefert `#…`).

- [ ] **Step 3: `ColorHex` token-reaktiv** — `nodestyle.go`: die `colorHex`-Map ersetzen durch:

```go
// ColorHex returns a token-reactive CSS color expression for a whitelisted
// color name ("" for unset/unknown). rgb(var(--x)) flips with the theme —
// a fixed hex would freeze node swatches in one palette.
func ColorHex(name string) string {
	for _, n := range domain.NodeColors {
		if n == name {
			return "rgb(var(--" + name + "))"
		}
	}
	return ""
}
```
(Die alte Map löschen. Der Funktionsname bleibt — alle Caller nutzen den Rückgabewert nur in `style`-Attributen, wo CSS-Var-Ausdrücke gleichwertig funktionieren.)

- [ ] **Step 4: Slice-3-Assertion anpassen** — `cockpit_render_test.go`, `TestNodeGlyphSwatch_LogoIconGlyphPriority`: `strings.Contains(icon, "#7dcfff")` → `strings.Contains(icon, "rgb(var(--cyan))")` (Kommentar: Tint ist token-reaktiv).

- [ ] **Step 5: Token-Flip** — `web/tailwind.css`: Die beiden Blöcke tauschen ihre Selektoren — der bisherige `:root { … }`-LIGHT-Block (Z.26–68) wird wortgleich zu `:root[data-theme="light"] { … }`; der bisherige `:root[data-theme="dark"]`-Block (Z.71–113) wird wortgleich zu `:root { … }` (Kommentare mitziehen: `/* ── DARK (default) — … ── */`, `/* ── LIGHT — abgeleitet ── */`). Die Regeln in Z.220–223 + 270 + 299 + 317 (`:root[data-theme="dark"] …`) bleiben funktional: sie treffen künftig NUR noch, wenn das Attribut explizit `dark` ist — deshalb setzt Base das Attribut IMMER (Step 6). ZUSÄTZLICH direkt unter dem Toggle-Block (Z.224) ergänzen, damit Attribut-lose Erstrender identisch dark sind:

```css
/* data-theme is always set by the no-flash script; these keep no-JS/first-paint
   consistent with the dark default. */
:root:not([data-theme]) .toggle-sun { display:none; }
:root:not([data-theme]) .toggle-moon { display:inline; }
```

- [ ] **Step 6: No-Flash-Default** — `base.templ` Z.24–34: Script-Body ersetzen:

```js
(function () {
	try {
		var saved = localStorage.getItem('flow-theme');
		document.documentElement.setAttribute('data-theme', saved || 'dark');
	} catch (e) {
		document.documentElement.setAttribute('data-theme', 'dark');
	}
})();
```
(Gespeicherte Wahl wird weiter respektiert; `prefers-color-scheme` entfällt bewusst — Entscheidung §2.3 der Spec: Kristall-Dark ist die Identität.)

- [ ] **Step 7: Dark-spezifische `wtblock`-Regeln generalisieren** — Z.270/299/317: Da dark jetzt Default (auch ohne Attribut) ist, die drei `:root[data-theme="dark"] .wtblock…`-Selektoren erweitern zu `:root:not([data-theme="light"]) .wtblock…` (gleiche Deklarationen). VERIFY per Browser-los: `rg -n 'data-theme="dark"' web/tailwind.css` → nur noch Toggle-Knob/Sun-Moon-Regeln (Z.220–223) dürfen auf explizit-dark matchen (der Knob-Slide ist rein kosmetisch; zusätzlich `:root:not([data-theme="light"]) .toggle-knob { transform: translateX(20px); }` ergänzen, damit der Knob im Default-Dark rechts steht).

- [ ] **Step 8: Build + Tests + CI**

Run: `make web && make generate && go test ./internal/adapter/webui/ -run "ColorHex|Priority" && make ci`
Expected: grün; app.css-Delta = Selektoren-Tausch (groß, aber mechanisch).

- [ ] **Step 9: Commit**

```bash
git add -A ':!/.mcp.json'
git commit -m "feat(kristall): dark is the default theme; ColorHex becomes token-reactive rgb(var(--x))"
```

---

## Task 2: Canvas-Upgrade — Mockup-Backdrop + Facets

**Files:** Modify `web/tailwind.css` (`--backdrop` beider Blöcke, `--facet-a`), `internal/adapter/webui/components/base.templ` (Facets-SVG Z.37–42). Test: `internal/adapter/webui/components/base_test.go` (anhängen; Datei existiert).

- [ ] **Step 1: Failing Render-Test** — `base_test.go` (Harness der Datei spiegeln — sie rendert `Base` bereits):

```go
func TestBase_KristallFacets(t *testing.T) {
	// mockup-normative facet layer: token-tinted polygons + soft radial pools
	body := renderBaseToString(t) // adapt: reuse the file's existing render helper
	for _, want := range []string{`class="kristall-facets"`, `fill-opacity=".022"`, `stroke-opacity=".04"`, `url(#kfacet-glow)`} {
		if !strings.Contains(body, want) {
			t.Errorf("facets layer missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Run — fails.** (`fill-opacity=".022"` existiert nicht.)

- [ ] **Step 3: Backdrop-Radials** — `web/tailwind.css`: im (jetzt) `:root`-Dark-Block `--backdrop` ersetzen:

```css
  --backdrop:
    radial-gradient(900px 620px at 8% -8%, rgba(196,181,253,.16), transparent 60%),
    radial-gradient(840px 640px at 104% 2%, rgba(103,232,249,.13), transparent 55%),
    radial-gradient(760px 720px at 90% 112%, rgba(110,231,183,.08), transparent 60%),
    linear-gradient(150deg,#1c1838,#2c1c42 55%,#102530);
```

im `[data-theme="light"]`-Block:

```css
  --backdrop:
    radial-gradient(900px 620px at 8% -8%, rgba(139,92,246,.10), transparent 60%),
    radial-gradient(840px 640px at 104% 2%, rgba(11,165,214,.08), transparent 55%),
    linear-gradient(150deg,#f4f2fb,#eef0fb 55%,#eaf4f6);
```

`--facet-a`: dark → `.8`, light → `.3`.

- [ ] **Step 4: Facets-SVG (Mockup-Polygone, token-getönt)** — `base.templ` Z.37–42 ersetzen:

```templ
			<svg class="kristall-facets" viewBox="0 0 1180 820" preserveAspectRatio="xMidYMid slice" aria-hidden="true">
				<defs>
					<radialGradient id="kfacet-glow" cx="50%" cy="50%" r="50%">
						<stop offset="0%" stop-color="rgb(var(--cyan))" stop-opacity=".08"></stop>
						<stop offset="100%" stop-color="rgb(var(--cyan))" stop-opacity="0"></stop>
					</radialGradient>
				</defs>
				<g fill="rgb(var(--ink))" fill-opacity=".022" stroke="rgb(var(--ink))" stroke-opacity=".04" stroke-width="1">
					<polygon points="0,90 260,0 360,220 60,320"></polygon>
					<polygon points="360,220 260,0 600,80 500,270"></polygon>
					<polygon points="880,0 1180,60 1100,290 840,200"></polygon>
					<polygon points="1100,290 1180,60 1180,420 960,470"></polygon>
					<polygon points="90,560 350,510 430,780 120,820"></polygon>
					<polygon points="740,820 690,540 1020,590 1080,820"></polygon>
				</g>
				<circle cx="220" cy="140" r="240" fill="url(#kfacet-glow)"></circle>
				<circle cx="1000" cy="700" r="260" fill="url(#kfacet-glow)"></circle>
			</svg>
```
(`rgb(var(--ink))` tönt die Facetten in beiden Themes richtig: fast-weiß auf dark, tiefes Navy auf light — jeweils mit Mockup-Opazitäten.)

- [ ] **Step 5: Glass auf Mockup-Norm** — `web/tailwind.css` Z.347–348: `.glass` blur `10px→16px`, `.glass-strong` `12px→18px`; `--shadow-lift` (Z.163) ersetzen durch die Mockup-Schatten-Komposition:

```css
  --shadow-lift: 0 22px 55px -22px rgb(var(--shadow) / .60), 0 8px 24px -14px rgb(var(--shadow) / .45), 0 18px 44px -16px rgb(var(--shadow-accent) / .18);
```

- [ ] **Step 6: generate + web + Tests + CI + Commit**

```bash
make generate && make web && go test ./internal/adapter/webui/components/ -run TestBase && make ci
git add -A ':!/.mcp.json'
git commit -m "feat(kristall): mockup backdrop radials + token-tinted facet layer + glass blur 16"
```

---

## Task 3: Primitives — Gradient-CTA, rtile-StatTile, Pill-TabStrip

**Files:** Modify `components/button.templ` (btnClass Z.13–27), `components/stattile.templ`, `components/subnav.templ`, `web/tailwind.css` (neue Klassen), `components/styleguide.templ` (Showcases anhängen). Tests: `components/primitives_test.go` (anhängen — Datei existiert, Muster spiegeln).

**Interfaces:**
- Produces: `Button` API unverändert (nur Optik); `StatTile(labelKey, value, hue string)` unverändert + NEU `StatTileAccent(labelKey, value, sub, hue string)` (Akzent-Unterkante + Subzeile); `Tab{Key, Href, LabelKey string; Count int}` (neues Feld, 0 = kein Chip) + `TabStrip` Pill-Optik.

- [ ] **Step 1: Failing Tests** — `primitives_test.go` anhängen (Render-Muster der Datei spiegeln):

```go
func TestButtonPrimary_KristallCTA(t *testing.T) {
	body := renderComp(t, Button(BtnPrimary, "Timer starten", "▶", nil)) // adapt helper name
	for _, want := range []string{"from-green", "to-cyan", "text-oncolor", "cta-glow"} {
		if !strings.Contains(body, want) {
			t.Errorf("primary CTA missing %q: %s", want, body)
		}
	}
}

func TestStatTileAccent_RendersAccentBarAndSub(t *testing.T) {
	body := renderComp(t, StatTileAccent("stats.week", "18h 20m", "+2h 05m", "purple"))
	for _, want := range []string{"rtile-ac", "--ac:var(--purple)", "18h 20m", "+2h 05m", "glass"} {
		if !strings.Contains(body, want) {
			t.Errorf("accent tile missing %q: %s", want, body)
		}
	}
}

func TestTabStrip_PillsAndCount(t *testing.T) {
	tabs := []Tab{{Key: "a", Href: "/a", LabelKey: "nav.projects", Count: 12}, {Key: "b", Href: "/b", LabelKey: "nav.home"}}
	body := renderComp(t, TabStrip(tabs, "a"))
	if !strings.Contains(body, "pill-tabs") || !strings.Contains(body, `aria-current="page"`) {
		t.Errorf("pill container/active missing: %s", body)
	}
	if !strings.Contains(body, ">12<") {
		t.Errorf("count chip missing: %s", body)
	}
}
```

- [ ] **Step 2: Run — fails.** `go test ./internal/adapter/webui/components/ -run "CTA|AccentBar|PillsAndCount"` → FAIL.

- [ ] **Step 3: CSS-Klassen** — `web/tailwind.css`, in den ersten `@layer components`-Block (nach `.card-corner`, Z.350) anhängen:

```css
  /* Kristall CTA glow (mockup .big-start) + hover lift */
  .cta-glow { box-shadow: 0 12px 30px -10px rgb(var(--cyan) / .55); transition: transform .14s ease, box-shadow .14s ease; }
  .cta-glow:hover { transform: translateY(-1px); box-shadow: 0 16px 36px -10px rgb(var(--cyan) / .65); }

  /* rollup-tile accent underline (mockup .rtile::after); hue via --ac */
  .rtile-ac { position: relative; }
  .rtile-ac::after { content:""; position:absolute; left:0; right:0; bottom:0; height:3px; background: linear-gradient(90deg, rgb(var(--ac, var(--blue))), transparent); }

  /* pill tab strip (mockup .tabs) */
  .pill-tabs { display:flex; gap:4px; background: rgb(var(--sunken) / .75); border:1px solid rgb(var(--line2)); border-radius:14px; padding:5px; overflow-x:auto; }
  .pill-tab { display:flex; align-items:center; gap:8px; padding:9px 15px; border-radius:10px; color: rgb(var(--muted)); font-weight:600; font-size:13px; white-space:nowrap; transition: color .13s ease, background .13s ease; }
  .pill-tab:hover { color: rgb(var(--ink)); }
  .pill-tab[aria-current="page"] { background: rgb(var(--glass) / var(--glass-strong-a)); color: rgb(var(--ink)); box-shadow: inset 0 0 0 1px rgb(var(--glass) / var(--glass-border-a)); }
  .pill-cnt { font-size:10.5px; color: rgb(var(--faint)); background: rgb(var(--glass) / .07); padding:1px 7px; border-radius:999px; }
  .pill-tab[aria-current="page"] .pill-cnt { color: rgb(var(--cyan)); }
```

- [ ] **Step 4: Button-CTA** — `button.templ` `btnClass` BtnPrimary-Zweig:

```go
	case BtnPrimary:
		return base + "bg-gradient-to-r from-green to-cyan text-oncolor font-bold cta-glow"
```
(Übrige Varianten unverändert.)

- [ ] **Step 5: StatTile** — `stattile.templ` ersetzen (API-kompatibel + Accent-Variante):

```templ
package components

func valueHue(hue string) string {
	switch hue {
	case "blue", "cyan", "green", "purple", "magenta", "yellow", "orange", "red", "teal":
		return "text-" + hue
	default:
		return "text-ink"
	}
}

// StatTile shows an eyebrow label (i18n key) over a large tnum value on glass.
templ StatTile(labelKey, value, hue string) {
	<div class="relative overflow-hidden rounded-2xl glass shadow-soft py-3 px-4 text-center">
		<div class="eyebrow uppercase text-[.62rem] font-semibold text-faint">{ T(ctx, labelKey) }</div>
		<div class={ "mt-1 font-display text-[1.35rem] font-semibold tnum", valueHue(hue) }>{ value }</div>
	</div>
}

// StatTileAccent is the Kristall rollup tile (mockup .rtile): left-aligned,
// hue accent underline via --ac, optional sub line. sub may be "".
templ StatTileAccent(labelKey, value, sub, hue string) {
	<div class="relative overflow-hidden rounded-[18px] glass shadow-soft rtile-ac px-4 pt-4 pb-4" style={ "--ac:var(--" + hue + ")" }>
		<div class="eyebrow uppercase text-[.62rem] font-bold text-faint">{ T(ctx, labelKey) }</div>
		<div class={ "mt-1.5 font-display text-[1.45rem] font-semibold leading-tight tnum", valueHue(hue) }>{ value }</div>
		if sub != "" {
			<div class="mt-0.5 text-[.68rem] text-muted">{ sub }</div>
		}
	</div>
}
```

- [ ] **Step 6: TabStrip** — `subnav.templ` ersetzen:

```templ
package components

// Tab is one sub-navigation entry. Count > 0 renders a count chip (Kristall
// pill-tabs, mockup .tab .cnt).
type Tab struct {
	Key, Href, LabelKey string
	Count               int
}

// TabStrip renders the Kristall pill tab strip; the active tab is glass-raised
// and aria-current.
templ TabStrip(tabs []Tab, active string) {
	<div class="mb-6">
		<nav class="pill-tabs scroll-thin" aria-label={ T(ctx, "nav.primary") }>
			for _, t := range tabs {
				if t.Key == active {
					<a href={ templ.SafeURL(t.Href) } aria-current="page" class="pill-tab">
						{ T(ctx, t.LabelKey) }
						if t.Count > 0 {
							<span class="pill-cnt tnum">{ fmt.Sprint(t.Count) }</span>
						}
					</a>
				} else {
					<a href={ templ.SafeURL(t.Href) } class="pill-tab">
						{ T(ctx, t.LabelKey) }
						if t.Count > 0 {
							<span class="pill-cnt tnum">{ fmt.Sprint(t.Count) }</span>
						}
					</a>
				}
			}
		</nav>
	</div>
}
```
(`import "fmt"` im templ-Header ergänzen, falls die Datei keinen hat — templ erlaubt Go-Imports oben.)

- [ ] **Step 7: Styleguide-Showcase** — `styleguide.templ`: im Buttons-Abschnitt nichts nötig (rendert `Button` bereits); NEU einen Abschnitt „Kristall-Kacheln + Tabs" anhängen, der `StatTileAccent("stats.week", "18h 20m", "+2h 05m ggü. Vorwoche", "purple")` und `TabStrip([]Tab{{Key:"a",Href:"#",LabelKey:"nav.home",Count:12},{Key:"b",Href:"#",LabelKey:"nav.projects"}}, "a")` zeigt (Muster der bestehenden Sektionen spiegeln).

- [ ] **Step 8: generate + web + Tests + CI + Commit**

```bash
make generate && make web && go test ./internal/adapter/webui/components/ && make ci
git add -A ':!/.mcp.json'
git commit -m "feat(kristall): gradient CTA button + rtile stat tiles + pill tab strip"
```

---

## Task 4: Sidebar — schwebende Glas-Karte + form-codierter Baum (ohne Stunden)

**Files:** Modify `components/appshell.templ` (aside Z.25, main Z.52), `internal/adapter/webui/navtree.templ`, `web/tailwind.css` (`.nvdot*`, `.fade-label`, aktive Zeile). Tests: `internal/adapter/webui/node_tree_vm_internal_test.go` (Render-Assertions anhängen; NavTree ist exportiert → alternativ externes Testfile, Muster prüfen).

**Interfaces:**
- Consumes: `TreeRow{Node domain.Node; Level int}` (+`Hours string` kommt in Task 5 — hier NICHT rendern).
- Produces: NavTree-Markup mit `.nvdot .nvdot-eng|-vor|-repo` + `--nc`-Tönung + `.fade-label`; Sidebar-Panel als Glas-Karte.

- [ ] **Step 1: Failing Render-Test**

```go
func TestNavTree_FormCodedDots(t *testing.T) {
	ctx := context.Background()
	rows := []TreeRow{
		{Node: domain.Node{ID: "e1", Name: "Kundenarbeit", Kind: domain.KindEngagement, Color: "magenta"}, Level: 0},
		{Node: domain.Node{ID: "v1", Name: "Plattform-Umbau", Kind: domain.KindVorhaben, Color: "purple"}, Level: 1},
		{Node: domain.Node{ID: "r1", Name: "flow", Kind: domain.KindRepo, Color: "blue"}, Level: 2},
	}
	body := renderToBufNT(t, ctx, NavTree(rows)) // adapt/reuse render helper
	for _, want := range []string{"nvdot-eng", "nvdot-vor", "nvdot-repo", "--nc:var(--magenta)", "fade-label", `title="flow"`} {
		if !strings.Contains(body, want) {
			t.Errorf("nav tree missing %q: %s", want, body)
		}
	}
}
```

- [ ] **Step 2: Run — fails.**

- [ ] **Step 3: CSS** — `web/tailwind.css` (`@layer components` anhängen):

```css
  /* sidebar tree: form-coded kind dots (mockup .node .dot), hue via --nc */
  .nvdot { width:9px; height:9px; flex:0 0 auto; background: rgb(var(--nc, var(--blue))); box-shadow: 0 0 8px -1px rgb(var(--nc, var(--blue))); }
  .nvdot-eng { border-radius:50%; }
  .nvdot-vor { transform: rotate(45deg); border-radius:1.5px; }
  .nvdot-repo { clip-path: polygon(25% 0,75% 0,100% 50%,75% 100%,25% 100%,0 50%); box-shadow:none; }
  /* soft fade truncation (mockup .node .lb) — replaces the hard ellipsis */
  .fade-label { white-space:nowrap; overflow:hidden; -webkit-mask-image: linear-gradient(90deg,#000 80%,transparent); mask-image: linear-gradient(90deg,#000 80%,transparent); }
  .nv-row-active { background: linear-gradient(90deg, rgb(var(--blue) / .20), rgb(var(--blue) / .03)); box-shadow: inset 0 0 0 1px rgb(var(--blue) / .4); }
  .nv-row-active .fade-label { -webkit-mask-image:none; mask-image:none; color: rgb(var(--ink)); font-weight:700; }
```

- [ ] **Step 4: NavTree-Markup** — `navtree.templ` ersetzen:

```templ
package webui

import "github.com/serverkraken/flow/internal/domain"

// navDotClass maps a node kind to its form-coded dot (● Engagement, ◆ Vorhaben
// as rotated square, ⬡ Repo as hexagon clip).
func navDotClass(k domain.NodeKind) string {
	switch k {
	case domain.KindEngagement:
		return "nvdot nvdot-eng"
	case domain.KindVorhaben:
		return "nvdot nvdot-vor"
	default:
		return "nvdot nvdot-repo"
	}
}

// NavTree renders the sidebar project-tree spine: form-coded kind dots in the
// node's color, soft fade truncation (full name via title tooltip), depth
// indentation. Each node links to its cockpit. A tiny script highlights the
// row matching the current location (fragment is htmx-loaded; the server does
// not know the active page).
templ NavTree(rows []TreeRow) {
	if len(rows) == 0 {
		<p class="px-2 py-1 text-[.78rem] text-faint">—</p>
	} else {
		<ul class="space-y-0.5" id="navtree-list">
			for _, row := range rows {
				<li>
					<a
						href={ templ.SafeURL("/nodes/" + row.Node.ID) }
						style={ nodeIndentStyle(row.Level) + nvHue(row.Node.Color) }
						title={ row.Node.Name }
						class="flex items-center gap-2 rounded-lg px-2 py-1 text-[.8rem] text-body hover:bg-glass/10 transition-colors"
					>
						<span class={ navDotClass(row.Node.Kind) } aria-hidden="true"></span>
						<span class="fade-label min-w-0 flex-1">{ row.Node.Name }</span>
					</a>
				</li>
			}
		</ul>
		<script>
			(function () {
				var list = document.getElementById('navtree-list');
				if (!list) { return; }
				list.querySelectorAll('a[href]').forEach(function (a) {
					if (a.getAttribute('href') === window.location.pathname) { a.classList.add('nv-row-active'); }
				});
			})();
		</script>
	}
}
```

Und in `node_tree_vm.go` (oder direkt in navtree.templ als Go-Func) den Hue-Helper:

```go
// nvHue appends the node-color custom property for the tree dot ("" when unset).
func nvHue(color string) string {
	if ColorHex(color) == "" {
		return ""
	}
	return ";--nc:var(--" + color + ")"
}
```
> VERIFY: `nodeIndentStyle(level)` liefert einen `style`-String OHNE abschließendes Semikolon? Lesen und die Konkatenation entsprechend setzen (`"…" + ";--nc:…"` bzw. anpassen).
> HINWEIS htmx: das `<script>` im Fragment wird von htmx bei Innerswap ausgeführt (htmx evaluiert Skripte in geswapptem Content) — genau dafür ist es im Fragment platziert, nicht in base.

- [ ] **Step 5: AppShell-Panel** — `appshell.templ` Z.25 (Desktop-aside) Klassen ersetzen:

```templ
	<aside class="hidden md:flex md:flex-col fixed left-4 inset-y-4 w-[248px] rounded-[20px] glass-strong shadow-lift z-40 overflow-hidden">
```
und Z.52 (main): `md:pl-[248px]` → `md:pl-[280px]` (248 Panel + 16 Abstand + 16 Luft). Mobile-Chrome unverändert (dieser Task).

- [ ] **Step 6: generate + web + Tests + CI + Commit**

```bash
make generate && make web && go test ./internal/adapter/webui/ -run NavTree && make ci
git add -A ':!/.mcp.json'
git commit -m "feat(kristall): floating glass sidebar + form-coded nav tree with fade labels"
```

---

## Task 5: Stunden-Badges im Baum (owner-scoped Aggregation)

**Files:** Modify `internal/adapter/webui/node_tree_vm.go` (`TreeRow.Hours` + `FillTreeHours`), `internal/adapter/httpserver/webui_nav.go`, `internal/adapter/webui/navtree.templ` (Badge rendern). Tests: `node_tree_vm`-Testdatei + `webui_nav_test.go` (existiert).

**Interfaces:**
- Consumes: den Session-List-Usecase, den die REST-Liste nutzt (VERIFY: `rg -n "ListSessions" internal/adapter/httpserver/worktime.go internal/adapter/httpserver/server.go` — exakt dieses Feld/`Execute`-Signatur verwenden; Sessions tragen `NodeID *string`, `Elapsed(now)`).
- Produces: `TreeRow.Hours string` (`"41h"`; leer unter 1 h); `webui.FillTreeHours(rows []TreeRow, totals map[string]time.Duration)`; `webui.SubtreeHourTotals(nodes []domain.Node, sessions []domain.Session, now time.Time) map[string]time.Duration`.

- [ ] **Step 1: Failing Unit-Test** (an die node_tree_vm-Tests anhängen):

```go
func TestSubtreeHourTotals_RollsUpAncestors(t *testing.T) {
	now := time.Date(2026, 7, 2, 18, 0, 0, 0, time.Local)
	p := func(s string) *string { return &s }
	nodes := []domain.Node{
		{ID: "e1", Kind: domain.KindEngagement},
		{ID: "v1", ParentID: p("e1"), Kind: domain.KindVorhaben},
		{ID: "r1", ParentID: p("v1"), Kind: domain.KindRepo},
	}
	mk := func(node string, h int) domain.Session {
		return domain.Session{ID: node + "-s", NodeID: p(node), Start: now.Add(time.Duration(-h) * time.Hour), Stop: &now}
	}
	sessions := []domain.Session{mk("r1", 2), mk("v1", 1), {ID: "unbooked", Start: now.Add(-time.Hour), Stop: &now}}
	got := webui.SubtreeHourTotals(nodes, sessions, now)
	if got["r1"] != 2*time.Hour || got["v1"] != 3*time.Hour || got["e1"] != 3*time.Hour {
		t.Errorf("totals = %v", got)
	}

	rows := []webui.TreeRow{{Node: nodes[0]}, {Node: nodes[1]}, {Node: nodes[2]}}
	webui.FillTreeHours(rows, got)
	if rows[0].Hours != "3h" || rows[2].Hours != "2h" {
		t.Errorf("hours = %q / %q", rows[0].Hours, rows[2].Hours)
	}
	short := []webui.TreeRow{{Node: domain.Node{ID: "x"}}}
	webui.FillTreeHours(short, map[string]time.Duration{"x": 30 * time.Minute})
	if short[0].Hours != "" {
		t.Errorf("sub-1h must render empty, got %q", short[0].Hours)
	}
}
```
> VERIFY Session-Feldnamen (`Stop *time.Time`, `Elapsed(now)`) gegen `internal/domain` — Testkonstruktion ggf. an die echten Felder anpassen; die Assertions (Rollup über Eltern, unbooked ignoriert, <1h leer) sind verbindlich.

- [ ] **Step 2: Run — fails.**

- [ ] **Step 3: Implementierung** — `node_tree_vm.go` anhängen:

```go
// SubtreeHourTotals aggregates each node's SUBTREE worktime in ONE pass over
// the owner's sessions (owner-scoped by construction — callers pass one
// owner's nodes+sessions): every session adds its elapsed time to its node
// and all ancestors. Unbooked sessions are skipped.
func SubtreeHourTotals(nodes []domain.Node, sessions []domain.Session, now time.Time) map[string]time.Duration {
	parent := make(map[string]*string, len(nodes))
	for _, n := range nodes {
		parent[n.ID] = n.ParentID
	}
	totals := make(map[string]time.Duration, len(nodes))
	for _, s := range sessions {
		if s.NodeID == nil {
			continue
		}
		el := s.Elapsed(now)
		if el <= 0 {
			continue
		}
		id := *s.NodeID
		for {
			p, ok := parent[id]
			if !ok {
				break // node not visible (archived/foreign) — stop the walk
			}
			totals[id] += el
			if p == nil {
				break
			}
			id = *p
		}
	}
	return totals
}

// FillTreeHours stamps rows with a compact subtree-hours badge ("41h");
// under one hour the badge stays empty to keep the tree calm.
func FillTreeHours(rows []TreeRow, totals map[string]time.Duration) {
	for i := range rows {
		if h := int(totals[rows[i].Node.ID].Hours()); h >= 1 {
			rows[i].Hours = fmt.Sprintf("%dh", h)
		}
	}
}
```
`TreeRow` um `Hours string` erweitern. (`fmt`/`time` importieren.)

- [ ] **Step 4: Handler** — `webui_nav.go` `handleNavTreeFragment`: nach `rows := webui.BuildTree(visible)` einschieben (Usecase-Feld aus dem VERIFY oben verwenden; Fehler nur warnen — Badges sind Zierde, der Baum muss immer rendern):

```go
	if sessions, serr := s.ListSessions.Execute(r.Context(), u.ID, time.Time{}); serr == nil {
		webui.FillTreeHours(rows, webui.SubtreeHourTotals(visible, sessions, s.Clock.Now()))
	} else {
		slog.WarnContext(r.Context(), "nav tree: hour badges skipped", "err", serr)
	}
```

- [ ] **Step 5: Badge rendern** — `navtree.templ`, in der Zeile nach dem `fade-label`-Span:

```templ
						if row.Hours != "" {
							<span class="shrink-0 text-[.68rem] text-faint tnum" aria-hidden="true">{ row.Hours }</span>
						}
```
NavTree-Render-Test um `"3h"`-Assertion ergänzen (Row mit gesetztem Hours).

- [ ] **Step 6: Handler-Test** — `webui_nav_test.go`: bestehenden Fragment-Test erweitern/spiegeln — geseedeter Node + Session → Fragment enthält das Badge; Store-Fehlerfall → Fragment rendert trotzdem (ohne Badge).

- [ ] **Step 7: generate + Tests + CI + Commit**

```bash
make generate && go test ./internal/adapter/webui/ ./internal/adapter/httpserver/ -run "SubtreeHour|NavTree" && make ci
git add -A ':!/.mcp.json'
git commit -m "feat(kristall): subtree hour badges in the nav tree (single owner-scoped pass)"
```

---

## Task 6: Globales Timer-Widget (Shell)

**Files:** Create `internal/adapter/webui/timerwidget_vm.go`, `internal/adapter/webui/timerwidget.templ`, `internal/adapter/httpserver/webui_timer.go`, `internal/adapter/httpserver/webui_timer_test.go`, `internal/adapter/webui/timerwidget_render_test.go`. Modify `components/appshell.templ` (Mounts), `server.go` (Routen), `catalog_de.go`/`catalog_en.go`.

**Interfaces:**
- Consumes: `s.GetRunningSession.Execute(ctx, ownerID) (domain.Session, bool, error)`, `s.StartSession.Execute(ctx, ownerID, nodeID *string, tags []string, note string)`, `s.StopSession.Execute(ctx, ownerID, sessionID string, nodeID *string)`, `s.CreateNode.Execute` (Quick-Create, Muster `handleWebStop` webui.go:27–33), `s.ListNodes.Execute` (+`domain.IsBookable`), `s.sessionEventData` (Slice 3), `s.Clock.Now()`. VERIFY die exakten Signaturen per `rg -n "StartSession|StopSession|GetRunningSession" internal/adapter/httpserver/webui.go webui_cockpit.go`.
- Produces: `webui.TimerWidgetVM{Running bool; Unbound bool; SessionID, NodeID, NodeName, NodeColor string; NodeKind domain.NodeKind; BaseSeconds int64; Bookable []domain.Node; Err string}`; templ `webui.TimerWidget(vm TimerWidgetVM)` (Desktop-Karte) + `webui.TimerChip(vm TimerWidgetVM)` (Mobile-Chip + Dialog); Routen `GET /ui/timer`, `GET /ui/timer/chip`, `POST /ui/timer/start`, `POST /ui/timer/stop`, `POST /ui/timer/switch`.

- [ ] **Step 1: i18n-Keys** (beide Kataloge, neuer `timer.*`-Block; de / en):

```
"timer.idle":       "Kein Timer läuft"            / "No timer running"
"timer.start":      "Timer starten"               / "Start timer"
"timer.stop":       "Stoppen"                     / "Stop"
"timer.switch":     "Wechseln"                    / "Switch"
"timer.runningOn":  "läuft auf"                   / "running on"
"timer.unbound":    "ohne Projekt"                / "no project yet"
"timer.choose":     "Projekt wählen…"             / "Choose a project…"
"timer.newProject": "…oder neues Projekt"         / "…or a new project"
"timer.needNode":   "Zum Stoppen Projekt wählen"  / "Choose a project to stop"
"timer.title":      "Timer"                       / "Timer"
```

- [ ] **Step 2: Failing Handler-Test** — `webui_timer_test.go` (Harness der bestehenden webui-Handler-Tests spiegeln; Emitter-Capture wie in Slice-3-Tests):

```go
func TestTimerWidget_Lifecycle(t *testing.T) {
	// seed: bookable node n1 "flow" (KindRepo, color cyan)
	// 1) GET /ui/timer (idle) → contains timer.start label, node select with n1, newProject field
	// 2) POST /ui/timer/start {projectId: n1} → 200, fragment shows running state:
	//    data-timer element, node pill "flow", stop button; captured session.started
	//    event Data carries node/name/kind (sessionEventData)
	// 3) POST /ui/timer/switch {projectId: n2} → stops+starts; two events captured
	// 4) POST /ui/timer/stop {} on a BOUND session → 200, idle fragment again
	// 5) unbound flow: start without projectId → running-unbound fragment shows
	//    timer.needNode select; POST stop WITHOUT projectId → fragment re-renders
	//    with vm.Err (timer.needNode), session STILL running; POST stop with
	//    projectId → idle.
	// Assertions verbindlich; Helfer an die reale Harness anpassen.
}
```

- [ ] **Step 3: Run — fails** (Routen fehlen).

- [ ] **Step 4: VM + Builder** — `timerwidget_vm.go`:

```go
package webui

import "github.com/serverkraken/flow/internal/domain"

// TimerWidgetVM drives the global shell timer widget (desktop card + mobile
// chip). Exactly ONE session can run per owner; the widget is that session's
// single global home (IA rule: eine globale Aktion, ein globales Zuhause).
type TimerWidgetVM struct {
	Running     bool
	Unbound     bool // running without a node → stop requires choosing one
	SessionID   string
	NodeID      string
	NodeName    string
	NodeColor   string
	NodeKind    domain.NodeKind
	BaseSeconds int64
	Bookable    []domain.Node
	Err         string // i18n-resolved message rendered inline (never a popup)
}
```

- [ ] **Step 5: templ** — `timerwidget.templ` (Karte + Chip; beide SSE-frei — der MOUNT-Container triggert):

```templ
package webui

import (
	"strconv"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

// TimerWidget is the desktop sidebar card: idle → start form with node picker
// (+ quick-create); running → live clock + node pill + stop/switch; unbound →
// stop demands a node. All actions post to /ui/timer/* and swap the mount.
templ TimerWidget(vm TimerWidgetVM) {
	<section class="rounded-2xl glass shadow-soft p-3 text-[.85rem]">
		<div class="eyebrow uppercase text-[.62rem] font-bold text-faint mb-2">{ components.T(ctx, "timer.title") }</div>
		if vm.Err != "" {
			<p class="mb-2 rounded-lg bg-red/10 px-2.5 py-1.5 text-[.78rem] text-red" role="alert">{ vm.Err }</p>
		}
		if !vm.Running {
			<form hx-post="/ui/timer/start" hx-target="#timer-widget" hx-swap="innerHTML" class="space-y-2">
				@timerNodeSelect(vm.Bookable, "")
				<input name="newProject" placeholder={ components.T(ctx, "timer.newProject") } class="w-full rounded-lg border border-line bg-sunken/60 px-2.5 py-1.5 text-[.8rem]"/>
				@components.Button(components.BtnPrimary, components.T(ctx, "timer.start"), "▶", templ.Attributes{"type": "submit", "class": "w-full"})
			</form>
		} else {
			<div class="flex items-center gap-2 min-w-0">
				if vm.NodeName != "" {
					{{ k := NodeKindStyle(vm.NodeKind) }}
					<a href={ templ.SafeURL("/nodes/" + vm.NodeID) } class={ "inline-flex min-w-0 items-center gap-1.5 rounded-md border px-2 py-0.5 text-[.72rem] font-medium", kindToneClass(k.Tone) }>
						<span aria-hidden="true">{ k.Glyph }</span>
						<span class="fade-label">{ vm.NodeName }</span>
					</a>
				} else {
					<span class="text-[.72rem] text-yellow">{ components.T(ctx, "timer.unbound") }</span>
				}
				<span class="ml-auto shrink-0 font-mono tnum text-[1.05rem] text-ink" data-timer data-base={ strconv.FormatInt(vm.BaseSeconds, 10) } role="timer">{ fmtSecsClock(vm.BaseSeconds) }</span>
			</div>
			<form hx-post="/ui/timer/stop" hx-target="#timer-widget" hx-swap="innerHTML" class="mt-2 space-y-2">
				if vm.Unbound {
					@timerNodeSelect(vm.Bookable, components.T(ctx, "timer.needNode"))
				}
				@components.Button(components.BtnDanger, components.T(ctx, "timer.stop"), "■", templ.Attributes{"type": "submit", "class": "w-full"})
			</form>
			<details class="mt-1.5">
				<summary class="cursor-pointer list-none text-[.72rem] text-muted hover:text-ink">{ components.T(ctx, "timer.switch") } ›</summary>
				<form hx-post="/ui/timer/switch" hx-target="#timer-widget" hx-swap="innerHTML" class="mt-1.5 space-y-2">
					@timerNodeSelect(vm.Bookable, "")
					@components.Button(components.BtnSecondary, components.T(ctx, "timer.switch"), "⇄", templ.Attributes{"type": "submit", "class": "w-full"})
				</form>
			</details>
		}
	</section>
}

// timerNodeSelect is the shared bookable-node picker. label may be "".
templ timerNodeSelect(bookable []Node, label string) {
	if label != "" {
		<label class="block text-[.72rem] text-yellow">{ label }</label>
	}
	<select name="projectId" class="w-full rounded-lg border border-line bg-sunken/60 px-2.5 py-1.5 text-[.8rem]">
		<option value="">{ components.T(ctx, "timer.choose") }</option>
		for _, n := range bookable {
			<option value={ n.ID }>{ n.Name }</option>
		}
	</select>
}

// TimerChip is the mobile top-bar mount: a compact live chip that opens the
// full widget in a Kristall dialog (no browser popups).
templ TimerChip(vm TimerWidgetVM) {
	<button type="button" data-dialog-open="timer-sheet" class="inline-flex items-center gap-1.5 rounded-full glass px-3 py-1 text-[.78rem]">
		if vm.Running {
			<span class="h-2 w-2 rounded-full bg-green animate-breathe" aria-hidden="true"></span>
			<span class="font-mono tnum" data-mini-timer>{ fmtSecsClock(vm.BaseSeconds) }</span>
		} else {
			<span class="text-muted">▶ { components.T(ctx, "timer.title") }</span>
		}
	</button>
	@components.Dialog("timer-sheet", components.T(ctx, "timer.title"), timerChipBody(vm))
}

templ timerChipBody(vm TimerWidgetVM) {
	@TimerWidget(vm)
}
```
> VERIFY vor Implementierung: (a) `fmtSecsClock` existiert in webui (Cockpit nutzt es — sonst Helfer aus cockpit_vm spiegeln); (b) `components.Dialog`-Signatur (`rg -n "templ Dialog" internal/adapter/webui/components/dialog.templ`) und den Aufruf exakt anpassen (Muster „more-menu" in appshell); (c) `Node` in `timerNodeSelect` = `domain.Node` — Import/Typ anpassen; (d) `components.Button` akzeptiert `class`-Attr via `templ.Attributes`? Wenn nicht: Button in einen `w-full`-Wrapper-`<div>` setzen. Assertions der Tests bleiben verbindlich.

- [ ] **Step 6: Handler** — `webui_timer.go`:

```go
package httpserver

import (
	"net/http"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/i18n"
	"github.com/serverkraken/flow/internal/usecase"
)

// timerWidgetVM assembles the shell timer state for one owner.
func (s *Server) timerWidgetVM(r *http.Request, u domain.User, errMsg string) webui.TimerWidgetVM {
	vm := webui.TimerWidgetVM{Err: errMsg}
	all, _ := s.ListNodes.Execute(r.Context(), u.ID)
	for _, n := range all {
		if n.Status == domain.NodeActive && domain.IsBookable(n.Kind) {
			vm.Bookable = append(vm.Bookable, n)
		}
	}
	rs, ok, err := s.GetRunningSession.Execute(r.Context(), u.ID)
	if err != nil || !ok {
		return vm
	}
	vm.Running = true
	vm.SessionID = rs.ID
	vm.BaseSeconds = int64(rs.Elapsed(s.Clock.Now()) / time.Second)
	if rs.NodeID == nil {
		vm.Unbound = true
		return vm
	}
	vm.NodeID = *rs.NodeID
	for _, n := range all {
		if n.ID == vm.NodeID {
			vm.NodeName, vm.NodeColor, vm.NodeKind = n.Name, n.Color, n.Kind
			break
		}
	}
	return vm
}

func (s *Server) renderTimerWidget(w http.ResponseWriter, r *http.Request, u domain.User, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.TimerWidget(s.timerWidgetVM(r, u, errMsg)).Render(r.Context(), w)
}

func (s *Server) handleTimerWidget(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	s.renderTimerWidget(w, r, u, "")
}

func (s *Server) handleTimerChip(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = webui.TimerChip(s.timerWidgetVM(r, u, "")).Render(r.Context(), w)
}

// timerNodeFromForm resolves projectId / newProject (quick-create, mirrors
// handleWebStop) into a node id pointer. nil = unbound.
func (s *Server) timerNodeFromForm(r *http.Request, u domain.User) *string {
	_ = r.ParseForm()
	nodeID := r.FormValue("projectId")
	if name := r.FormValue("newProject"); name != "" {
		if p, err := s.CreateNode.Execute(r.Context(), u.ID, usecase.CreateNodeInput{Name: name, Kind: domain.KindEngagement}); err == nil {
			nodeID = p.ID
			s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeCreated, UserID: u.ID, Data: map[string]any{"id": p.ID, "name": p.Name}})
		}
	}
	if nodeID == "" {
		return nil
	}
	return &nodeID
}

func (s *Server) handleTimerStart(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	nodeID := s.timerNodeFromForm(r, u)
	sess, err := s.StartSession.Execute(r.Context(), u.ID, nodeID, nil, "")
	if err != nil {
		s.renderTimerWidget(w, r, u, i18n.T(r.Context(), "timer.idle"))
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStarted, UserID: u.ID,
		Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)})
	s.renderTimerWidget(w, r, u, "")
}

func (s *Server) handleTimerStop(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	rs, ok, gerr := s.GetRunningSession.Execute(r.Context(), u.ID)
	if gerr != nil || !ok {
		s.renderTimerWidget(w, r, u, "")
		return
	}
	nodeID := s.timerNodeFromForm(r, u)
	if nodeID == nil {
		nodeID = rs.NodeID // bound session: stop books to its own node
	}
	sess, err := s.StopSession.Execute(r.Context(), u.ID, rs.ID, nodeID)
	if err != nil {
		s.renderTimerWidget(w, r, u, i18n.T(r.Context(), "timer.needNode"))
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStopped, UserID: u.ID,
		Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)})
	s.renderTimerWidget(w, r, u, "")
}

func (s *Server) handleTimerSwitch(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	target := s.timerNodeFromForm(r, u)
	if target == nil {
		s.renderTimerWidget(w, r, u, i18n.T(r.Context(), "timer.choose"))
		return
	}
	if rs, ok, gerr := s.GetRunningSession.Execute(r.Context(), u.ID); gerr == nil && ok {
		stopNode := rs.NodeID
		if stopNode == nil {
			stopNode = target // unbound running: book it to the switch target
		}
		if sess, err := s.StopSession.Execute(r.Context(), u.ID, rs.ID, stopNode); err == nil {
			s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStopped, UserID: u.ID,
				Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)})
		}
	}
	sess, err := s.StartSession.Execute(r.Context(), u.ID, target, nil, "")
	if err != nil {
		s.renderTimerWidget(w, r, u, i18n.T(r.Context(), "timer.choose"))
		return
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStarted, UserID: u.ID,
		Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)})
	s.renderTimerWidget(w, r, u, "")
}
```
> Fehlertexte: bewusst i18n-Kurzmeldungen (kein err.Error() ins UI); Start-Fehlerfall (already running) rendert schlicht den echten Zustand neu — der Fragment-GET zeigt dann die laufende Session. VERIFY Signaturen wie oben; `domain.Session.Elapsed` existiert (Cockpit nutzt es).

- [ ] **Step 7: Routen + Mounts** — `server.go` (bei den anderen `/ui/…`-Routen):

```go
	mux.Handle("GET /ui/timer", s.webAuth(http.HandlerFunc(s.handleTimerWidget)))
	mux.Handle("GET /ui/timer/chip", s.webAuth(http.HandlerFunc(s.handleTimerChip)))
	mux.Handle("POST /ui/timer/start", s.webAuth(http.HandlerFunc(s.handleTimerStart)))
	mux.Handle("POST /ui/timer/stop", s.webAuth(http.HandlerFunc(s.handleTimerStop)))
	mux.Handle("POST /ui/timer/switch", s.webAuth(http.HandlerFunc(s.handleTimerSwitch)))
```

`appshell.templ`: Desktop — zwischen Brand-Block und `@SiteNav(active)` einfügen:

```templ
		<div
			id="timer-widget"
			class="px-3 pb-1"
			hx-get="/ui/timer"
			hx-trigger="load, sse:session.started, sse:session.stopped, sse:session.updated, sse:session.deleted"
			hx-swap="innerHTML"
		></div>
```

Mobile-Top-Bar (Z.40–48, neben `@ThemeToggle()`):

```templ
			<div
				id="timer-chip"
				hx-get="/ui/timer/chip"
				hx-trigger="load, sse:session.started, sse:session.stopped, sse:session.updated, sse:session.deleted"
				hx-swap="innerHTML"
			></div>
```

- [ ] **Step 8: Render-Tests** — `timerwidget_render_test.go` (Paket `webui`, `renderToBuf`-Muster): idle enthält `hx-post="/ui/timer/start"` + newProject-Feld; running enthält `data-timer` + Stop; unbound enthält `timer.needNode`-Select; Chip (running) enthält `data-mini-timer` + `data-dialog-open="timer-sheet"`.

- [ ] **Step 9: generate + web + Tests + CI + Commit**

```bash
make generate && make web && go test ./internal/adapter/webui/ ./internal/adapter/httpserver/ -run Timer && make ci
git add -A ':!/.mcp.json'
git commit -m "feat(kristall): global shell timer widget (desktop card + mobile chip) — IA canonical home"
```

---

## Task 7: Wiring-Audit + Voll-CI + Live-Done-Gate + Holistic Review

- [ ] **Step 1: Wiring-Audit**

```bash
rg -n "ui/timer" internal/adapter/httpserver/server.go internal/adapter/webui/components/appshell.templ
rg -c "nvdot|fade-label|pill-tabs|cta-glow|rtile-ac" web/tailwind.css
rg -n "data-theme', saved || 'dark'" internal/adapter/webui/components/base.templ
```
Expected: 5 Routen + 2 Mounts; alle 5 CSS-Klassenfamilien; Dark-Default.

- [ ] **Step 2: Voll-CI** — `make generate && make web && make ci` → grün, Coverage notieren.

- [ ] **Step 3: Live-Done-Gate (Dev-Stack, scripted Dex-Login wie gehabt):**
  1. `GET /` ohne gespeichertes Theme → `data-theme="dark"`, Twilight-Backdrop + Facets im HTML; Toggle → light persistiert, Reload bleibt light.
  2. Sidebar: schwebende Glas-Karte; Baum zeigt Form-Punkte je Kind in Knotenfarbe, langer Name faded (kein `…`), `title`-Tooltip; Stunden-Badges an Knoten mit ≥1 h Subtree-Zeit; aktive Route hervorgehoben (JS).
  3. Widget idle → Start mit Projekt → Widget zeigt Pill+Uhr (tickt); Home/Heute-Fragmente zeigen die Session ebenfalls (SSE, Doppel-Reload idempotent — Risiko §12.5 der Spec explizit beobachten); Stop im Widget → idle; unbound-Start → Stop verlangt Projekt (Inline-Fehler, kein Popup); Switch wechselt in einem Zug; `newProject` legt Engagement an und startet darauf.
  4. Mobile-Viewport (Dev-Tools): Chip in der Top-Bar tickt, öffnet Dialog mit Widget, Aktionen funktionieren.
  5. `/ui`-Styleguide: CTA-Button, StatTileAccent, Pill-Tabs sichtbar korrekt in dark UND light.
  6. Activity-Log: Widget-Start/Stop erzeugen Einträge MIT Ziel-Pill (sessionEventData-Pfad).
- [ ] **Step 4: Browser-Dogfood (Soenne)** — Gesamteindruck „App fühlt sich Kristall an", Sidebar-Baum, Widget-Alltagstauglichkeit; Light-Ableitung akzeptabel (Feinschliff-Notizen → K5).
- [ ] **Step 5: Holistic Review (Opus)** — BASE = Task-1-Vorgänger-Commit (beim Start notieren). Fokus: Theme-Flip-Vollständigkeit (kein Selektor hängt mehr fälschlich an explizit-dark; no-JS-Erstrender), ColorHex-Blast-Radius (alle style-Attr-Caller), Widget-Nebenläufigkeit (Doppel-Submit, SSE-Doppel-Render, unbound-Stop-Pfad), Owner-Scoping der neuen Queries (§0 Grundsätze — Multi-Tenant-Kalibrierung!), Aggregations-Korrektheit (Ancestor-Walk, archivierte Eltern), i18n-Parität, A11y (role=timer, aria-labels, prefers-reduced-motion).
- [ ] **Step 6: Follow-up-Commit nur bei Findings.**

---

## Self-Review (plan author)
**Spec-Coverage (§5 K1):** Dark-Default → T1; Canvas/Facets → T2; Komponenten-Restyle (Kern-Primitives; vollflächige Seiten-Adoption ist per Spec K3/K4) → T3; Sidebar-Rework (Glas-Panel, Form-Punkte, Fade, Badges) → T4+T5; Timer-Widget (§3.1) → T6; `/ui` + Gates → T3 Step 7 + T7. Nicht-Entfernen der Alt-Formulare als Global Constraint (K3). ✓
**Placeholder-Scan:** Alle VERIFY-Punkte benennen konkrete Suchbefehle + verbindliche Fallbacks (Dialog-Signatur, Button-class-Attr, Session-Felder, ListSessions-Feld); Tests tragen echte Assertions. Kein TBD. ✓
**Typ-Konsistenz:** `ColorHex → rgb(var(--x))` (T1) wird von T4 `nvHue` + T6-Pill konsumiert; `TreeRow.Hours` (T5) rendert in T4-Markup erst ab T5-Step-5 (T4 rendert ohne Hours — explizit vermerkt); `Tab.Count int`; `TimerWidgetVM`-Felder == templ-Nutzung == Handler-Builder; Routen-Set konsistent (widget/chip/start/stop/switch). `fmtSecsClock`/`NodeKindStyle`/`kindToneClass` existieren in webui (Cockpit/Slice 3). ✓
**Reuse:** Widget nutzt bestehende Usecases + `sessionEventData` + `[data-timer]`-Rebind + Dialog-Komponente + htmx-Fragment-Muster (`/ui/nav/tree`); keine neuen Ports; Aggregation kompositorisch im Handler. Mockup-Werte 1:1 (Backdrop, Facets, Pill-Tabs, rtile, Dots, Fade).
