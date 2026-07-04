# Kristall K2 — Cockpit Direction B (kind-differenziert) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Das Cockpit (`/nodes/{id}`) wird das approved „lebendige Projekt-Zuhause": **persistente Rail** (Identitäts-Karte mit 110-px-Logo-Hero + Auto-Crop, Timer-Karte in Mockup-Optik, Quick-Actions) + rechts **Pill-Tabs mit Übersicht als Default-Landing** (Rollup-Kacheln mit Vorwochen-Delta, Work/Privat-Split, Zusammensetzung ODER Fließt-nach-oben-Kette je Kind, Puls mit Ziel-Pills, Zuletzt-Wissen aus dem Subtree) — plus **Containment im Worktime-Tab**, **Session-Edit/Delete via `SessionDialog`** und der **Stop-Picker-Fix**.

**Architecture:** Kompositorisch auf Gelandetem: `NodeStats` (Slice 1, +`PrevWeek`), Logos (Slice 2, +Maße/Migration 0027 + `LogoShape`), Ziel-Pills/`BuildActivityRows` (Slice 3), K1-Primitives (`StatTileAccent`, Pill-`TabStrip` mit `Count`, `.glass`, `.nvdot*`). Keine neuen Ports — Subtree-Docs/-Activity/Kette/Zusammensetzung sind Handler-Kompositionen über bestehende Usecases. Die kanonische htmx-Regel bleibt: Tab-Fläche targetet `#cockpit-main`; die Rail ist der neue separate SSE-Container `#cockpit-rail` (Route `/nodes/{id}/head` bleibt, rendert künftig die Rail). `SessionDialog` (components) ist der EINE Session-Mechanismus (IA-Regel) — K2 baut ihn, K3 migriert Heute/Woche darauf.

**Tech Stack:** Go, templ + htmx + Tailwind v4, `golang.org/x/image/webp` (NEU, nur `DecodeConfig`), `make generate`/`make web`/`make ci`.

## Global Constraints
- Branch `cockpit-story`, Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`. Spec: `docs/superpowers/specs/2026-07-02-kristall-redesign-design.md` §4 (Containment-Tabelle!), §6, §2 (Entscheidungen 2/4/5/7). Mockup normativ: `assets/2026-07-01-cockpit-story/direction-b-APPROVED.html` (Rail/Feed-Bereiche Z.301–448).
- **MULTI-TENANT** (AGENTS.md §Grundsätze): owner-scoped; „single user"-Begründungen unzulässig.
- **Design-Änderbarkeits-Regel** (Soenne, K1-Dogfood): NUR Tokens/Primitives/benannte `@layer components`-Klassen; Arbitrary-One-offs (`text-[.8rem]` etc.), wo Token/Klasse existiert, sind Review-Findings. Wiederholte neue Muster → benannte Klasse.
- Kanonische htmx-Regel: alles, was die Tab-Fläche neu rendert, targetet `#cockpit-main` (durch bestehende Regression-Tests gepinnt — nicht brechen); Full-Page-Links/Forms `hx-boost="false"`.
- Containment (Spec §4): Rollup=Subtree (ist); **Worktime-Tab** Eng/Vorhaben=Subtree-Sessions MIT Node-Pill, Repo=eigene; **Übersicht-Wissen-Karte**=Subtree; Wissen-TAB bleibt eigene Docs (Umschalter = K4); Struktur-Tab unverändert (direkte Kinder).
- `make ci` grün pro Task (75 %, `*_templ.go` excl.); generierte Dateien VOR `make ci` stagen; `make web` + app.css bei neuen Klassen; **kein `make fmt`**; keine Emojis (▶ ■ ⇄ › Σ ● ◆ ok); keine Popups; i18n de+en (test-enforced); Events via `s.Emitter.Emit` + `s.sessionEventData`.
- Migration: nächste freie Nummer **0027**. Zeilennummern = Stand `6d9d6c2`; bei Drift Bezeichnern trauen.
- Podman-Fallback für pgstore-Tests wie gehabt.

---

## File Structure
**Create:** `internal/adapter/pgstore/migrations/0027_node_logos_dimensions.sql` · `internal/adapter/webui/logoshape.go` (+`logoshape_test.go`) · `internal/adapter/webui/components/sessiondialog.templ` (+Render-Test) · `internal/adapter/webui/cockpit_uebersicht.templ` + `cockpit_uebersicht_vm.go` (+Tests) · `internal/adapter/httpserver/webui_cockpit_uebersicht.go` (Übersicht-Builder) · `internal/adapter/httpserver/webui_cockpit_sessions.go` (Edit/Delete-Endpoints).
**Modify:** `internal/domain/nodelogo.go` (+Width/Height) · `internal/usecase/upload_node_logo.go` (Maße messen) + `get_node_logo.go` (Lazy-Backfill) · `internal/adapter/pgstore/nodelogos.go` (Spalten) · `internal/testutil/fakes.go` · `internal/domain/node_rollup.go` (+PrevWeek) + `internal/usecase/node_stats.go` · `internal/adapter/webui/cockpit_vm.go` (Tabs+VM-Felder+SessionRow-Felder) · `internal/adapter/webui/cockpit.templ` (Rail+Zwei-Spalten) · `internal/adapter/httpserver/webui_cockpit.go` (Builder/Handler) · `internal/adapter/httpserver/server.go` (2 Routen) · `internal/adapter/httpserver/webui_heute.go`:90-93 + `webui_home.go`:~138 (Stop-Picker-Fix) · `internal/i18n/catalog_de.go`/`catalog_en.go` · `go.mod` (+`golang.org/x/image`).

---

## Task 1: Logo-Maße (Migration 0027) + `LogoShape`-Auto-Crop-Entscheid

**Files:** Create Migration + `logoshape.go`(+Test). Modify `nodelogo.go`, `upload_node_logo.go`, `get_node_logo.go`, `pgstore/nodelogos.go`, `fakes.go` (FakeNodeLogoStore trägt Struct — nur kompilieren), `go.mod`.

**Interfaces:**
- Produces: `domain.NodeLogo.Width, Height int`; `usecase.ValidateNodeLogo(data) (mime string, w, h int, err error)` (ERWEITERT — Task 6 von Slice 2 rief 2-Werte-Form: alle Caller anpassen, `rg -n "ValidateNodeLogo" internal`); `webui.LogoShape(w, h int) string` → `"hex"` bei h>0 && ratio w/h ∈ [0.8,1.25], sonst `"tile"` (0-Maße → `"hex"`, Alt-Verhalten).

- [ ] **Step 1: Failing Tests** — `logoshape_test.go`:

```go
func TestLogoShape(t *testing.T) {
	cases := []struct{ w, h int; want string }{
		{100, 100, "hex"}, {120, 100, "hex"}, {80, 100, "hex"},
		{300, 100, "tile"}, {100, 300, "tile"}, {126, 100, "tile"},
		{0, 0, "hex"}, // Bestandslogos ohne Maße: bisheriges Hex-Verhalten
	}
	for _, c := range cases {
		if got := webui.LogoShape(c.w, c.h); got != c.want {
			t.Errorf("LogoShape(%d,%d)=%q want %q", c.w, c.h, got, c.want)
		}
	}
}
```

Usecase-Test (an `node_logo_test.go` anhängen): Upload des 1×1-PNG → gespeicherte `Width==1 && Height==1`; danach `GetNodeLogo` auf einen Fake-Eintrag mit `Width==0` und echten PNG-Bytes → Rückgabe trägt gemessene Maße UND der Store-Eintrag wurde aktualisiert (Lazy-Backfill).

- [ ] **Step 2: Run — fails.**

- [ ] **Step 3: Dependency + Domain + Migration**

```bash
go get golang.org/x/image@latest
```

`nodelogo.go`: `Width, Height int` (+Kommentar: „gemessen beim Upload via image.DecodeConfig; 0 = Altbestand, wird beim ersten Get lazy vermessen"). Migration `0027_node_logos_dimensions.sql`:

```sql
-- +goose Up
ALTER TABLE node_logos ADD COLUMN width  integer NOT NULL DEFAULT 0;
ALTER TABLE node_logos ADD COLUMN height integer NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE node_logos DROP COLUMN height;
ALTER TABLE node_logos DROP COLUMN width;
```

- [ ] **Step 4: Messen + Backfill** — `upload_node_logo.go`: `ValidateNodeLogo` erweitert:

```go
import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

// ValidateNodeLogo size-checks, content-sniffs and measures logo bytes.
// Width/height come from image.DecodeConfig (jpeg/png stdlib, webp via
// golang.org/x/image); a sniff-valid but unparseable image is rejected.
func ValidateNodeLogo(data []byte) (mime string, w, h int, err error) {
	if len(data) > MaxNodeLogoBytes {
		return "", 0, 0, ErrLogoTooLarge
	}
	mime = http.DetectContentType(data)
	switch mime {
	case "image/png", "image/jpeg", "image/webp":
	default:
		return "", 0, 0, ErrLogoBadType
	}
	cfg, _, derr := image.DecodeConfig(bytes.NewReader(data))
	if derr != nil {
		return "", 0, 0, ErrLogoBadType
	}
	return mime, cfg.Width, cfg.Height, nil
}
```
`Execute` speichert `Width: w, Height: h` im `domain.NodeLogo`. ALLE bisherigen `ValidateNodeLogo`-Caller auf die 4-Werte-Signatur heben (`webui_nodelogo.go` `readValidatedLogo` ignoriert Maße: `_, _, _, err :=` — VERIFY per rg).

`get_node_logo.go` Lazy-Backfill:

```go
func (uc GetNodeLogo) Execute(ctx context.Context, ownerID, nodeID string) (domain.NodeLogo, error) {
	l, err := uc.Logos.Get(ctx, ownerID, nodeID)
	if err != nil {
		return domain.NodeLogo{}, err
	}
	// Altbestand (vor Migration 0027 hochgeladen): Maße beim ersten Zugriff
	// nachmessen und persistieren, damit LogoShape entscheiden kann.
	if l.Width == 0 || l.Height == 0 {
		if cfg, _, derr := image.DecodeConfig(bytes.NewReader(l.Bytes)); derr == nil {
			l.Width, l.Height = cfg.Width, cfg.Height
			_ = uc.Logos.Put(ctx, l) // best-effort; serving darf nie daran scheitern
		}
	}
	return l, nil
}
```
(gleiche Imports; pgstore `nodelogos.go`: width/height in INSERT/UPDATE-SET/SELECT/Scan ergänzen.)

- [ ] **Step 5: `LogoShape`** — `logoshape.go`:

```go
package webui

// LogoShape decides the hero treatment for an uploaded logo: near-square
// images get the Kristall hexagon crop, wide/tall wordmarks render intact on
// a glass tile (contain). Unmeasured legacy rows (0×0) keep the hex crop.
func LogoShape(w, h int) string {
	if w <= 0 || h <= 0 {
		return "hex"
	}
	r := float64(w) / float64(h)
	if r >= 0.8 && r <= 1.25 {
		return "hex"
	}
	return "tile"
}
```

- [ ] **Step 6: Tests + CI + Commit**

```bash
go test ./internal/adapter/webui/ -run LogoShape && go test ./internal/usecase/ -run NodeLogo && make ci
git add -A ':!/.mcp.json'
git commit -m "feat(kristall): node logo dimensions (migration 0027, x/image webp) + LogoShape auto-crop rule"
```

---

## Task 2: `NodeRollup.PrevWeek` (Vorwochen-Delta)

**Files:** Modify `internal/domain/node_rollup.go`, `internal/usecase/node_stats.go`. Test: `node_stats_test.go` (anhängen).

**Interfaces:**
- Produces: `domain.NodeRollup.PrevWeek time.Duration` (Subtree-Summe der VOR-Kalenderwoche, Mo–So vor `weekStart`).

- [ ] **Step 1: Failing Test** (Muster `TestNodeStats_WorkPrivatSplit` in derselben Datei spiegeln — FakeStores + AddSession): drei Sessions — 2 h diese Woche, 3 h vorige Woche (Start = `weekStart.Add(-24h)`), 1 h vor zwei Wochen → `Week==2h && PrevWeek==3h`.
- [ ] **Step 2: Run — fails** (`PrevWeek` undefined).
- [ ] **Step 3:** `node_rollup.go`: Feld `PrevWeek time.Duration` (Kommentar „Vorwoche, für das Übersicht-Delta"). `node_stats.go`: im bestehenden Fenster-Block `prevWeekStart := weekStart.AddDate(0, 0, -7)` und in der Session-Schleife:

```go
		if !st.Before(prevWeekStart) && st.Before(weekStart) {
			r.PrevWeek += el
		}
```
- [ ] **Step 4: Run — passes** (alle bestehenden NodeStats-Tests grün). **Step 5:** `make ci` + Commit `feat(stats): NodeRollup.PrevWeek for the overview week delta`.

---

## Task 3: `SessionDialog` — der EINE Session-Mechanismus (Add/Edit)

**Files:** Create `internal/adapter/webui/components/sessiondialog.templ`; Modify i18n-Kataloge. Test: `components/primitives_test.go` anhängen (Render).

**Interfaces:**
- Consumes: `components.Dialog(id, titleKey string, body templ.Component)` — VERIFY exakte Signatur (`rg -n "templ Dialog" internal/adapter/webui/components/dialog.templ`) und das `data-dialog-open`-Muster; Assertions bleiben verbindlich.
- Produces:

```go
// SessionDialogVM drives the single shared add/edit session dialog (IA rule:
// one mechanism, contextual entry points prefill it).
type SessionDialogVM struct {
	DialogID string // element id, e.g. "session-dialog"
	Mode     string // "add" | "edit"
	Action   string // hx-post URL (add: POST /nodes/{id}/sessions; edit: POST /nodes/{id}/sessions/{sid}/edit)
	Target   string // hx-target selector, e.g. "#cockpit-main"
	Date     string // YYYY-MM-DD prefill
	From, To string // HH:MM
	Tag      string // space-joined
	Note     string
	Nodes    []domain.Node // optional booking picker (empty = hidden, node fixed by Action)
	NodeID   string        // preselect
}
```
templ `SessionDialog(vm SessionDialogVM)` — Kristall-Formular im `<dialog>`: date/from/to (`type="date"/"time"`, mobile-taugliche Größen wie Timer-Widget: `px-3 py-2.5 text-base md:px-2.5 md:py-1.5 md:text-[.8rem]` — genau diese, keine neuen One-offs), optionaler Node-`<select>`, tag/note, Submit (`components.Button` Primary, Label `session.dialog.save`) mit `hx-post={vm.Action} hx-target={vm.Target} hx-swap="innerHTML"`, Abbrechen schließt den Dialog (Muster der Dialog-Komponente). Formularname-Kontrakt = die BESTEHENDEN Nachbuchen-Feldnamen des Cockpit-Endpoints (`rg -n 'FormValue' internal/adapter/httpserver/webui_cockpit.go` — from/to/tag/note + date; exakt übernehmen, sonst bricht der Add-Pfad).

- [ ] **Step 1: i18n** (de/en): `session.dialog.addTitle` „Zeit nachbuchen"/"Add time" · `editTitle` „Sitzung bearbeiten"/"Edit session" · `save` „Speichern"/"Save" · `date` „Datum"/"Date" · `from` „Von"/"From" · `to` „Bis"/"To" · `tags` „Tags"/"Tags" · `note` „Notiz"/"Note" · `node` „Projekt"/"Project".
- [ ] **Step 2: Failing Render-Test** — add-Modus enthält `<dialog`, DialogID, `hx-post` auf Action, date-Prefill, KEIN select bei leeren Nodes; edit-Modus mit Nodes enthält select + NodeID-selected + editTitle.
- [ ] **Step 3: Implementieren** (Dialog-Komponente wrappen; Titel via Mode). **Step 4:** `make generate` + Tests grün. **Step 5:** `make web` (falls nötig) + `make ci` + Commit `feat(kristall): shared SessionDialog component (add/edit) — the single session mechanism`.

---

## Task 4: Rail + Tab-Gerüst (Zwei-Spalten, Übersicht-Default)

**Files:** Modify `cockpit_vm.go` (Tabs/VM), `cockpit.templ` (Umbau), `webui_cockpit.go` (Builder-Erweiterung: TodayHere/CountsWork/Logo/TabCounts), i18n. Tests: `cockpit_render_test.go` + `cockpit_vm_test.go` anpassen/ergänzen (bestehende `#cockpit-main`-Regressionstests MÜSSEN grün bleiben).

**Interfaces:**
- Consumes: `LogoShape` (T1), `SessionDialog` (T3), K1-`TabStrip`-Optik NICHT (Cockpit-Tabs sind eigene `pill-tabs`-Nav im templ — gleiche Klassen), `NodeTimer` (unverändert).
- Produces: `CockpitTabs` = `{"uebersicht","cockpit.tab.uebersicht"}` + bisherige 4; `NormalizeTab` default `"uebersicht"`; `NodeCockpit` NEU: `LogoShape string` (""|"hex"|"tile"), `TodayHere string` (heute auf DIESEM Node, fmtDurHM), `CountsWork bool` (effektiver Flag), `Contributors []string`, `TabCounts map[string]int`; templ `CockpitRail(d)` ersetzt `NodeHead` (Route `/nodes/{id}/head` liefert Rail; Container-ID im Seiten-Gerüst: `#cockpit-rail`).

- [ ] **Step 1: Failing Render-Tests** (an `cockpit_render_test.go`):
  - `TestCockpitLayout_TwoColumns`: `cockpitBody` rendert `id="cockpit-rail"` (SSE-Trigger enthält `sse:session.started` + `sse:node.updated`) UND `id="cockpit-main"`; Rail kommt im Markup VOR main; Wrapper trägt `lg:grid-cols-[340px_1fr]`.
  - `TestCockpitRail_IdentityHero`: LogoRef+LogoShape="tile" → `<img` mit `object-contain` OHNE `clip-path`; LogoShape="hex" → `clip-path`-Style; ohne Logo mit Icon → `<svg`; Hero-Box `h-[110px] w-[110px]`.
  - `TestCockpitRail_TimerStates`: TimerHere → Stop-Form + `data-timer`; TimerIdle → `cta-glow`-Start + „heute hier"-Zeile mit TodayHere + Work/Privat-Wort; TimerOtherBound → elsewhere-Banner mit OtherName + Wechseln; TimerNotBookable → Hinweistext.
  - `TestCockpitTabs_UebersichtDefault`: `NormalizeTab("")=="uebersicht"`; Tab-Nav rendert 5 `pill-tab`-Links, Übersicht `aria-current`, Count-Chip wenn `TabCounts["wissen"]>0`.
  - Bestehende Tests: `TestCockpitHex_*` entfallen ersatzlos NUR wenn `cockpitHex` entfällt — die Identitäts-Logik wandert in die Rail; Assertions (Logo>Icon>Glyph-Priorität!) in die neuen Rail-Tests ÜBERNEHMEN, nicht löschen.
- [ ] **Step 2: Run — fails.**
- [ ] **Step 3: VM** — `cockpit_vm.go`: Tabs+Default+Felder wie oben; `CockpitSessionRow` bleibt (T6 erweitert). i18n: `cockpit.tab.uebersicht` „Übersicht"/"Overview" · `cockpit.rail.todayHere` „heute hier"/"today here" · `cockpit.rail.countsWork` „zählt als Work"/"counts as Work" · `cockpit.rail.countsPrivat` „Privat — nur getrackt"/"Private — tracked only" · `cockpit.rail.inherited` „geerbt von"/"inherited from" · `cockpit.rail.contributors` „Beiträger"/"Contributors" · `cockpit.rail.children` „Unterknoten"/"Subnodes" · `cockpit.qa.book` „Nachbuchen"/"Add time" · `cockpit.qa.knowledge` „Neues Wissen"/"New knowledge" · `cockpit.qa.structure` „Struktur bearbeiten"/"Edit structure".
- [ ] **Step 4: templ-Umbau** — `cockpit.templ`:
  - `cockpitBody`: Wrapper `<div class="grid gap-4 lg:grid-cols-[340px_1fr] items-start">`; links `<div id="cockpit-rail" class="lg:sticky lg:top-6 flex flex-col gap-4" hx-get={"/nodes/"+d.N.ID+"/head"} hx-trigger="sse:session.started, sse:session.stopped, sse:session.updated, sse:node.updated, sse:node.moved" hx-swap="innerHTML">@CockpitRail(d)</div>`; rechts `<div id="cockpit-main" class="min-w-0">@CockpitTabsAndPanel(d)</div>`. (Responsive: einspaltig stapelt Rail zuerst — grid-Reihenfolge reicht.)
  - `CockpitRail(d)`: DREI Glas-Karten.
    (a) **Identität** (Mockup id-card): zentriert; Hero-Wrap 110px: `if d.N.LogoRef != ""` → `if d.LogoShape=="tile"` → `<img src=… class="h-[110px] w-[110px] rounded-2xl object-contain glass p-2">` mit Ring-Div dahinter; else Hex-`<img object-cover>` mit `style="clip-path:polygon(25% 0,75% 0,100% 50%,75% 100%,25% 100%,0 50%)"` + Hex-Ring; `else if d.N.Icon != ""` → Hex-Glas-Tile mit `@NodeIcon(d.N.Icon, "h-12 w-12")` im Knotenfarbton; else Glyph-Tile (Priorität wie gehabt). Kind-Badge-Ecke (`hero-badge`-artig: kleiner Glas-Kreis mit `NodeKindStyle(d.N.Kind).Glyph`). `<h1 class="font-display text-2xl font-bold">` Name; Badge-Zeile `@nodeKindBadge` + `@nodeStatusBadge` + „Bearbeiten"-Link (BESTEHT — aus c8c4dee, mitnehmen!); Beschreibung (`d.DescriptionHTML`, prose, max 3 Zeilen via `preview-clamp`); **id-meta**-Box (`bg-sunken/60 rounded-xl p-3 text-[.8rem]`-Zeilen): Satz (`d.Rate`, Eng: „Stundensatz", sonst + `cockpit.rail.inherited` Root-Name), Eng zusätzlich `cockpit.rail.children`: `d.TabCounts["struktur"]`, Repo: `upstream git`-Zeile wenn gesetzt (font-mono, `gitDisplay`); `Contributors` (wenn nicht leer, komma-joined).
    (b) **Timer** (Mockup timer-card): OtherBound → elsewhere-Banner (gelber Puls-Dot `animate-breathe`, „Timer läuft auf **OtherName**" + Mini-Uhr `data-mini-timer` + Wechseln-Form wie bisher); Idle → `cockpit.rail.todayHere`-Zeile (`d.TodayHere` + `·` + CountsWork? countsWork:countsPrivat) + big `cta-glow`-Start (`components.Button` Primary in `<div class="grid">`); Here → `data-timer data-timer-fmt="clock"` groß + Stop; Unbound/NotBookable wie bisherige Texte. Bestehende hx-post-Ziele (`/nodes/{id}/start|stop|switch`) UNVERÄNDERT, aber `hx-target="#cockpit-rail"`.
    (c) **Quick-Actions**: `grid grid-cols-2 gap-2` — Nachbuchen (`<button data-dialog-open="session-dialog">`), Neues Wissen (`<a href={"/wissen/neu?node="+d.N.ID} hx-boost="false">` — VERIFY Editor-Query-Param: `rg -n '"node"' internal/adapter/httpserver/webui_editor.go`), Struktur (Tab-Link wie Tab-Nav, `hx-get`/`hx-target="#cockpit-main"`, volle Breite `col-span-2`).
  - Seiten-Gerüst mountet EINMAL `@components.SessionDialog(sessionDialogAddVM(d))` (Helfer in `cockpit_uebersicht_vm.go` o. templ-nah: DialogID "session-dialog", Mode add, Action `/nodes/{id}/sessions`, Target `#cockpit-main`, Date=heute).
  - Tab-Nav: 5 Einträge aus `CockpitTabs`, Klassen `pill-tabs`/`pill-tab`/`pill-cnt` (K1), `hx-get /nodes/{id}/tab/{key}` + `hx-target="#cockpit-main"` + `hx-push-url` wie bisher; Count aus `d.TabCounts[key]`.
  - `cockpitPanelSSE("uebersicht")` = `"sse:session.started, sse:session.stopped, sse:session.updated, sse:session.deleted, sse:activity.logged, sse:document.updated, sse:node.updated"`.
- [ ] **Step 5: Builder** — `webui_cockpit.go` `nodeCockpitData`: nach dem Timer-Block ergänzen: `d.LogoShape` (wenn `n.LogoRef!=""`: `s.GetNodeLogo.Execute` → `webui.LogoShape(l.Width, l.Height)`; Fehler → "hex"); `d.TodayHere` (Sessions.List seit Tagesanfang? VERIFY wie `heuteDataFor` den Tag lädt — gleiche Quelle nutzen; Summe der Sessions mit `NodeID==n.ID` heute, `fmtDurHM`-Export); `d.CountsWork = domain.ResolveCountsTowardTarget(chain)` (chain wie beim Rate-Resolve); `d.TabCounts` = map{"wissen": len(node-docs — `s.ListDocuments.Execute(ctx, u.ID, &n.ID …)` VERIFY Signatur), "struktur": len(children — `s.ListNodeChildren`? VERIFY wie der Struktur-Tab sie lädt), "bindings": len(bindings — wie Bindings-Tab)}; Counts-Fehler → 0 (warn). `Contributors` füllt T5 (bleibt leer bis dahin — Zeile rendert konditional).
- [ ] **Step 6: Handler-Kompatibilität** — `renderNodeHead` liefert weiterhin die Route `/nodes/{id}/head`, rendert jetzt `webui.CockpitRail(d)`; Start/Stop/Switch-Handler rendern die Rail (Target-Wechsel in (b) beachten). `rg -n "cockpit-head" internal` → ALLE Reste auf `cockpit-rail` ziehen (SSE-Container-Umbenennung vollständig).
- [ ] **Step 7: generate/web/Tests/CI + Commit** `feat(kristall): cockpit two-column rail (identity hero + timer + quick actions) with Übersicht default tab`.

---

## Task 5: Übersicht-Feed (Kacheln · Work/Privat · Zusammensetzung|Kette · Puls · Wissen)

**Files:** Create `cockpit_uebersicht.templ` + `cockpit_uebersicht_vm.go` (+`_test.go`), `webui_cockpit_uebersicht.go`. Modify `webui_cockpit.go` (Tab-Switch „uebersicht" → Builder), `cockpit.templ` (`cockpitPanel` case), i18n.

**Interfaces:**
- Consumes: `d.Rollup` (+PrevWeek T2), `StatTileAccent` (K1), `BuildActivityRows` + `activityTargetPill` + `components.ActorGlyph` (Slice 3/M1), `.nvdot*` (K1), `NodeKindStyle`, `s.ListActivity.Execute(ctx, owner, classes, actorRef, limit, offset)`, `s.ListDocuments`, `s.NodeSubtree`? (VERIFY: wie kommt der Server an Subtree-IDs — `rg -n "Subtree" internal/adapter/httpserver internal/usecase/*.go` — es gibt den Store-Zugriff über `s.Stats`-Pfad; nimm den vorhandenen Usecase/Store-Weg, KEINEN neuen Port).
- Produces:

```go
// UebersichtVM is the overview landing's card data (kind-differentiated).
type UebersichtVM struct {
	Kind        domain.NodeKind
	// tiles
	TotalStr, WeekStr, WeekDelta, MonthStr, Earnings string
	// work/privat (week window)
	WorkPct int; WorkWeekStr, PrivatWeekStr, WorkMonthStr string
	HasSplit bool // false collapses the card (one side zero)
	// composition (Eng/Vorhaben) — direct children
	Comp []CompRow // {ID, Name string; Kind domain.NodeKind; Color string; DurStr string; Pct int; Live bool; LastAct string}
	// chain (Repo) — this → ancestors → total
	Chain []ChainRow // {Label string; Kind domain.NodeKind; DurStr string; Pct int; This bool; Sum bool}
	// pulse + knowledge
	Pulse []ActivityRowVM
	Docs  []UebersichtDoc // {ID, Title, Meta string}
	DocsTotal int
}
```
Builder `(s *Server) uebersichtData(ctx, u, d *webui.NodeCockpit) (webui.UebersichtVM, error)` — Regeln: Kacheln aus `d.Rollup` (WeekDelta = `+X`/`−X` vs PrevWeek, leer bei PrevWeek==0); Split: Woche `WorkWeek` vs `Week−WorkWeek`, `HasSplit = beide > 0`, Pct gerundet; Zusammensetzung: direkte Kinder (Quelle wie Struktur-Tab) → je Kind `s.Stats.NodeStats` (Subtree!), Pct = Kind-Total/Node-Total, Live = laufende Session-NodeID ∈ Kind-Subtree (Subtree-IDs des Kindes — über denselben Weg wie oben; bei Tiefe>1 reicht: running.NodeID==Kind.ID ODER in dessen Subtree via einmal geladener Gesamt-Subtree+Parent-Walk — implementiere den Parent-Walk über die schon geladene Subtree-Liste des COCKPIT-Knotens, kein Extra-Query), LastAct aus Puls-Daten (jüngster Eintrag mit NodeRef im Kind-Subtree, RelTime); Kette (Repo): Zeilen this(Node, `This:true`) → jeder Ancestor (leaf→root aus `d.Ancestors[1:]`, je NodeStats) → `Sum:true`-Zeile „Gesamt" = Owner-Gesamt (Σ über alle Root-Engagement-NodeStats — Roots aus der Parents-Quelle; N Roots klein), Pct = Zeile/Gesamt; Puls: `ListActivity(…, nil, nil, 50, 0)` → Filter NodeRef ∈ Cockpit-Subtree-IDs → Top 8 → `BuildActivityRows` (names/kinds via `s.nodeMaps`); `Contributors` (→ `d.Contributors`): distinct `ActorRef` der gefilterten Liste (max 4); Docs: `s.ListDocuments(owner, nil, nil)` → Filter `NodeID ∈ Subtree` (Repo: `== n.ID`) → sort UpdatedAt desc → Top 3, `DocsTotal` = Gesamtzahl, Meta = `ActorloseRelTime`? (nimm `fmtRelTime`-Muster aus activity_row.go — VERIFY Export, sonst kleiner Helfer).

- [ ] **Step 1: Failing VM-Tests** — Tabellen-Tests für: WeekDelta-Formatierung (+2h 05m / −0:30 / ""), HasSplit-Kollaps, Comp-Pct-Rundung, Chain-Pct (this 8 %, Sum 100 %), Puls-Filter (fremder NodeRef fliegt raus), Docs-Sort+Top3. (Pure-Helfer in `cockpit_uebersicht_vm.go` so schneiden, dass sie ohne Server testbar sind: `BuildUebersichtTiles(roll, rate)`, `BuildComp(children, statsByID, runningNodeID, subtreeParents, pulse)`, `BuildChain(node, ancestors, statsByID, ownerTotal)`, `FilterPulse(entries, subtreeIDs)`, `TopDocs(docs, subtreeIDs, now)` — exakte Signaturen im Test festschreiben.)
- [ ] **Step 2: Run — fails.** **Step 3: Helfer implementieren** (rein, ohne I/O). **Step 4: Builder + Handler-Verdrahtung** — `webui_cockpit.go`: `case "uebersicht"` im Tab-Datenpfad ruft `uebersichtData` (Fehler → PanelErr); `cockpitPanel` templ-case rendert `@CockpitUebersicht(d, vm)`.
- [ ] **Step 5: templ** — `cockpit_uebersicht.templ` mit Mockup-Klassen (alles Bestand: `StatTileAccent`, `.glass`, `.nvdot*`, `pill`-Muster, `activityTargetPill`, `ActorGlyph`; NEU als benannte Klassen in tailwind.css NUR: `.split-bar { height:14px; border-radius:999px; overflow:hidden; display:flex; background:rgb(var(--sunken)); border:1px solid rgb(var(--line2)); }` + `.split-work { background:linear-gradient(90deg,rgb(var(--green)),rgb(var(--cyan))); }` + `.split-priv { background:linear-gradient(90deg,rgb(var(--purple)),rgb(var(--purple))/.7); }` + `.chain-bar { height:6px; border-radius:999px; background:rgb(var(--sunken)); overflow:hidden; }` — Design-Änderbarkeits-Regel!). Aufbau: `grid gap-4`: Kachel-Zeile `grid grid-cols-2 xl:grid-cols-4 gap-3` (Σ-Kachel: value mit `bg-gradient-to-r from-purple to-cyan bg-clip-text text-transparent`; Verdienst hue yellow); `grid gap-4 xl:grid-cols-2`: Work/Privat-Karte (Legende, `.split-bar` mit width-%-Styles, Note `cockpit.wp.note`, Totals) + Zusammensetzung/Kette-Karte (Comp-Rows: nvdot je Kind-Kind, Name link → `/nodes/{id}`, DurStr, `.chain-bar` mit Pct, Live-Dot `animate-breathe bg-green` wenn Live, LastAct `text-faint`; Chain analog mit `This`-Highlight `nv-row-active`-artig + Sum-Zeile Σ); Puls-Karte (`card-h` mit LIVE-Badge `animate-breathe`, Zeilen: ActorGlyph, ActorRef fett, Verb, `activityTargetPill`, RelTime; AGENT-Tag wenn ActorKind=="agent": kleines `border-purple/40 text-purple`-Chip „AI-AGENT"); Wissen-Karte (3 `kitem`-Zeilen: ◆-Tile, Titel→`/wissen/{id}`, Meta; Footer „alle N ›" → Wissen-Tab-Link `hx-target="#cockpit-main"`). i18n: `cockpit.ov.total` „Subtree Σ"/"Subtree Σ" · `ov.week` „Diese Woche"/"This week" · `ov.month` „Diesen Monat"/"This month" · `ov.earnings` „Verdienst"/"Earnings" · `ov.inclChildren` „eigen + alle Unterknoten"/"own + all subnodes" · `wp.title` „Work vs. Privat"/"Work vs. private" · `wp.note` „Work zählt aufs Tages-Soll · Privat wird nur getrackt"/"Work counts toward the daily target · private is only tracked" · `wp.workMonth` „Work Monat"/"Work month" · `comp.title` „Woraus besteht das?"/"What is this made of?" · `comp.lastAct` „zuletzt aktiv"/"last active" · `chain.title` „Fließt nach oben"/"Flows upward" · `chain.here` „hier"/"here" · `chain.total` „Gesamt"/"Total" · `pulse.title` „Puls — was gerade los ist"/"Pulse — what's happening" · `pulse.live` „LIVE"/"LIVE" · `pulse.agent` „AI-AGENT"/"AI-AGENT" · `ov.docs` „Zuletzt geändertes Wissen"/"Recently changed knowledge" · `ov.docsAll` „alle %d ›"/"all %d ›" (Format-Key: VERIFY wie i18n Platzhalter handhabt — sonst zwei Keys).
- [ ] **Step 6: Render-Tests** (output-asserting): Kacheln mit Delta; Split-Widths; Repo rendert Kette (kein Comp), Engagement rendert Comp (keine Kette); Puls-Zeile mit Pill+AGENT-Tag; Docs-Karte. **Step 7:** generate/web/ci + Commit `feat(kristall): cockpit Übersicht landing — tiles, work/privat split, composition/chain, live pulse, knowledge`.

---

## Task 6: Containment im Worktime-Tab + Session-Edit/Delete + Stop-Picker-Fix

**Files:** Create `webui_cockpit_sessions.go` (+Tests in bestehender Cockpit-Testdatei). Modify `cockpit_vm.go` (`CockpitSessionRow` +`ID, NodeID, NodeName string; NodeKind domain.NodeKind` + `BuildCockpitSessionRows`-Signatur +nodes-Lookups), `cockpit.templ` (worktime-Panel: Pill+Aktionen+Edit-Dialog), `webui_cockpit.go` (worktime-Tab-Daten: Subtree), `server.go` (2 Routen), `webui_heute.go`:90-93 + `webui_home.go`:~138 (Picker), i18n (`cockpit.session.edit` „Bearbeiten"/"Edit" · `cockpit.session.delete` „Löschen"/"Delete" · `cockpit.session.deleteConfirm` „Sitzung löschen?"/"Delete session?").

**Interfaces:**
- Produces: Routen `POST /nodes/{id}/sessions/{sid}/edit` + `POST /nodes/{id}/sessions/{sid}/delete` (webAuth; thin auf `s.EditSession`/`s.DeleteSession` — VERIFY deren Input-Formen in `webui_worktime.go` handleWebEdit/handleWebDelete und exakt spiegeln, inkl. Fehlerpfade als PanelErr-Re-Render), Emit `session.updated`/`session.deleted` via `sessionEventData`, Response = `renderNodePanel(worktime)` (Target `#cockpit-main` — kanonische Regel!).

- [ ] **Step 1: Failing Handler-Tests** — (a) Worktime-Tab eines Engagements listet die Session eines Enkel-Repos MIT dessen Node-Pill (`nvdot`-Glyph/NodeName im Row-Markup) — Repo-Cockpit listet NUR eigene; (b) Edit-Endpoint ändert Zeiten (Store-Assert) + rendert Panel; (c) Delete-Endpoint entfernt + `session.deleted`-Event trägt id; (d) Picker: `heuteDataFor`/`homeDataFor` Buchungs-Select enthält jetzt ein Repo+Vorhaben (IsBookable) und der laufende Node ist `selected` (VERIFY bestehende Picker-Tests + Markup: `rg -n "projectId" internal/adapter/webui/heute.templ internal/adapter/webui/home.templ`).
- [ ] **Step 2: Run — fails.**
- [ ] **Step 3: Rows + Tab-Daten** — `BuildCockpitSessionRows(sessions, now, names map[string]string, kinds map[string]domain.NodeKind)`; Row-Felder füllen; `webui_cockpit.go` worktime-case: Eng/Vorhaben → Subtree-IDs (gleiche Quelle wie T5) + Sessions.List gefiltert; Repo → wie bisher eigene. templ: je Row rechts `activityTargetPill`-artige Node-Pill (NUR wenn `d.N.Kind != domain.KindRepo`), Edit-Button (öffnet `SessionDialog` edit — pro Row `data-dialog-open` auf EINEN Dialog, dessen Felder per hx-get? NEIN, einfach: Edit-Button = `hx-get /nodes/{id}/tab/worktime?edit={sid}` → Panel rendert den Edit-Dialog offen mit Prefill [`<dialog open>`-Attribut auf der Dialog-Komponente via VM-Flag — VERIFY ob Dialog-Komponente ein open-Flag hat, sonst 3-Zeilen-Erweiterung dort]), Delete-Button = Form mit `components.ConfirmDialog`-Muster (VERIFY bestehende Nutzung: `rg -n "ConfirmDialog" internal/adapter/webui`).
- [ ] **Step 4: Endpoints** — `webui_cockpit_sessions.go` (Muster handleWebNodeAddSession daneben; Zeit-Parsing wie handleWebEdit); Routen in server.go bei den Cockpit-Routen.
- [ ] **Step 5: Stop-Picker-Fix** — beide Sites: Filter `if !domain.IsBookable(p.Kind) || p.Status != domain.NodeActive { continue }` (Status-Verhalten wie Bestand — VERIFY, nicht verschärfen), Kommentar „bookable = Engagement/Vorhaben/Repo (Spec #1-Fix)"; Vorselektion: running-NodeID als `Selected`-Feld durch die bestehende VM (VERIFY Feldweg — heute.templ/home.templ Select-Option-Schleife) — laufender Node `selected`.
- [ ] **Step 6: Run — passes; alle bestehenden Cockpit-/Heute-/Home-Tests grün** (Regression `#cockpit-main`-Regel!). **Step 7:** generate/web/ci + Commit `feat(kristall): worktime-tab containment + session edit/delete via SessionDialog + IsBookable stop picker`.

---

## Task 7: Wiring-Audit + Voll-CI + Live-Gate + Holistic Review

- [ ] **Step 1: Audit**
```bash
rg -n "cockpit-rail|cockpit-head" internal --glob '!*_templ.go'   # nur -rail
rg -n "sessions/\{sid\}/(edit|delete)" internal/adapter/httpserver/server.go
rg -n "IsBookable" internal/adapter/httpserver/webui_heute.go internal/adapter/httpserver/webui_home.go
rg -c "uebersicht" internal/adapter/webui/cockpit_vm.go
```
- [ ] **Step 2: Voll-CI** — `make generate && make web && make ci` (echter Exit-Code!), Coverage notieren.
- [ ] **Step 3: Live-Gate (Dev-Stack, scripted Login):**
  1. Migration 0027 applied; **RTL-Extern-Logo** (breit) → Rail-Hero rendert `object-contain` OHNE clip-path (tile); ein quadratisches Test-Logo → Hex-Crop; Icon-/Glyph-Fallback.
  2. `/nodes/{id}` landet auf **Übersicht**; Kacheln inkl. Vorwochen-Delta; Work/Privat-Balken stimmt gegen `/api/v1/nodes/{id}/stats`.
  3. Engagement-Cockpit: Zusammensetzungs-Karte (Kinder, Anteile, Live-Dot beim laufenden Kind, zuletzt aktiv); Repo-Cockpit: Kette this→…→Gesamt (Pct plausibel).
  4. Puls: Timer-Start auf Enkel-Repo erscheint im Engagement-Puls mit Ziel-Pill; AGENT-Zeile (via MCP-Doc-Edit oder seeded) trägt Hexagon-Avatar+Tag. Beiträger in der Identitäts-Karte.
  5. Worktime-Tab: Engagement listet Subtree-Sessions mit Node-Pills; Edit ändert Zeit (Dialog), Delete mit Confirm; Repo nur eigene. Rail-Timer: alle 5 Zustände (idle „heute hier … zählt als Work", here, otherBound-Banner+Wechseln, unbound, notBookable) — Uhren ticken (Rail `data-timer-fmt="clock"` + Seiten-Uhren unabhängig).
  6. Heute+Home-Picker enthalten Repos/Vorhaben, laufender Node vorselektiert; Stop auf Repo bucht korrekt.
  7. Tabs: alle 5 laden in `#cockpit-main` (kein Strip-Nesting — Regressionstests + Sichtprüfung), Counts stimmen, `?tab=`-Deep-Link, SSE-Reload je Tab.
  8. Mobile: Rail stapelt über Tabs, Dialog-Formulare komfortabel (T3-Größen).
- [ ] **Step 4: Browser-Dogfood (Soenne)** — Story-Gefühl der Übersicht („was ist das, was passiert gerade"), RTL-Logo-Hero, Kind-Unterschiede Eng vs. Repo.
- [ ] **Step 5: Holistic Review (Opus)** — BASE = K2-Plan-Commit. Fokus: Containment-Korrektheit (Subtree-Filter owner-scoped, Repo==self), Kette/Comp-Prozente (Division durch 0), Puls-Filter (kein Cross-Tenant/Cross-Subtree-Leak), LogoShape-Blast-Radius (ValidateNodeLogo-Signaturwechsel ALLE Caller), Dialog-Mechanik (ein Dialog, Edit-Prefill, keine Popups), htmx-Regel (`#cockpit-main`-Pins grün), Design-Änderbarkeits-Regel (keine neuen One-offs), i18n-Parität, x/image-Dep sauber.
- [ ] **Step 6: Follow-up-Commit nur bei Findings.**

---

## Self-Review (plan author)
**Spec-Coverage:** §6 Rail (Identität+Hero-Auto-Crop §2.4/T1+T4, Timer-Karte T4, QA T4) ✓; Übersicht-Feed komplett T5 (Kacheln+Delta T2, Split Slice-1-Daten, Comp|Kette kind-differenziert §4, Puls Slice-3-Daten, Wissen-Subtree) ✓; Tabs+Übersicht-Default+Counts T4 ✓; Containment Worktime-Tab + Edit/Delete + `SessionDialog` T6 (Dialog T3) ✓; Stop-Picker-Fix T6 ✓; SSE-Split `#cockpit-rail`/`#cockpit-main` T4 ✓; Wissen-TAB-Umschalter bewusst K4 (Constraint dokumentiert) ✓.
**Placeholder-Scan:** VERIFY-Punkte benennen konkrete rg-Kommandos + Fallbacks (Dialog-open-Flag, Editor-Param, Subtree-Quelle, EditSession-Inputs, Picker-VM-Weg, i18n-Format-Keys); Tests tragen echte Assertions; keine „TBD". ✓
**Typ-Konsistenz:** `LogoShape(w,h)→"hex"|"tile"` (T1) ↔ `NodeCockpit.LogoShape` (T4); `ValidateNodeLogo` 4-Werte überall; `PrevWeek` (T2) ↔ `WeekDelta` (T5); `SessionDialogVM`-Felder (T3) ↔ Mounts (T4/T6); `UebersichtVM`/Helfer-Signaturen in T5-Tests festgeschrieben; `CockpitSessionRow`-Erweiterung (T6) mit neuer Build-Signatur; Routen-Set konsistent. ✓
**Reuse:** StatTileAccent/pill-tabs/nvdot/activityTargetPill/ActorGlyph/NodeIcon/ConfirmDialog/Dialog; NodeTimer unverändert; keine neuen Ports; genau 4 neue benannte CSS-Klassen (split-bar/-work/-priv, chain-bar) — Änderbarkeits-Regel eingehalten.
