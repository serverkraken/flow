# Lesesaal L3 — Lese-Ebene (Dokument · Eindämmung · Mermaid · Verweise-Rail · Wissen-Regale + Suche) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Die **Lese-Ebene** auf Lesesaal-Sprache bringen. Drei Flächen sterben aus der Kristall-Ära und werden neu gesetzt: die **Dokument-Seite** (`/wissen/{id}`) wird Pfad-Rückgrat + Provenance-Zeile (Akteur Mensch/Agent · Zeit · Pfad · Lesezeit · Bearbeiten/Anpinnen) + Lesespalte (max. 680px, echte Doc-Typografie) + funktionale Meta-Spalte (`docrail`: ToC · Verweise beide Richtungen), **Markdown** wird eingedämmt (Fremd-Inhalt, §11) und **Mermaid** als gesetzte Figur gerendert, die **Wissen-Seite** (`/wissen`) wird Suche-als-Haupttür + **Regale nach Typ** + Zuletzt-aktualisiert. Flankierend: **Provenance-Stamp** (Akteur bei jeder Doc-Mutation, Migration + Domain-Feld) und eine **CSP-/Sanitizer-Härtung** (Agent-Markdown ist untrusted). Der Editor zieht auf Lesesaal-Tokens um (kein Redesign).

**Architecture:** Server-rendered wie gehabt (templ + htmx + Tailwind, kein SPA, kein Node). Markdown bleibt **server-seitig** (goldmark → bluemonday, `RenderDocument`); Mermaid ist **progressive enhancement**: der Server rendert den Codefence als lesbare, gesetzte `<figure>` mit `<pre class="mermaid">` + collapsibler Quelle, und **nur wenn ein Dokument Mermaid enthält** wird vendored `mermaid.min.js` + ein winziges idempotentes `mermaid-init.js` lazy nachgeladen (Doc ohne Diagramm bleibt scriptfrei; Doc ohne JS bleibt lesbar). Neue Anzeige-Logik lebt in reinen, unit-getesteten Go-Buildern (`webui`-Paket, domain-frei) und in der goldmark-Pipeline; die templ-Komponenten nehmen fertige Strings. Provenance nutzt das **bestehende `actor`-Modell** (`actor.FromContext(ctx)` → `{Kind: human|agent, Ref}`, gesetzt in `ctxWithUser`/`middleware.go:28` für API **und** WebUI) — die Doc-Write-Usecases stempeln beim Schreiben. Der Server-Struct hat für Dokumente/Suche/Wissen bereits alle Usecase-Felder (`GetDocument`, `ListDocuments`, `SearchDocuments`, `BacklinksDocument`, `SetPinned`, `CreateDocument`, `UpdateDocument`, `UpsertDocumentByPath`) — **kein `cmd/flow-server/main.go`-Change** außer der Provenance-Migration (die läuft über goose automatisch) und ggf. neuen Routen (statische Pfade in `server.go`).

**Tech Stack:** Go 1.x · templ · Tailwind v4.1.5 (CLI, `make web`) · htmx (vendored, SSE-Extension) · goldmark + bluemonday (Server-Markdown) + chroma (Highlighting) · **mermaid.min.js (neu vendored, gepinnt, MIT, LICENSES.md)** · Schibsted Grotesk + JetBrains Mono (L1). Provenance: goose-Migration `0028` + `actor`-Paket (Bestand).

**Spec:** `docs/superpowers/specs/2026-07-04-lesesaal-webui-redesign-design.md` (§9 Zeilen Dokument+Wissen · §10 Provenance + Kontext-Instrument-Randnote · §11 Lese-Ebene komplett + Eindämmungs-Systemregel · §13 A11y · §16 Punkte 7/8/9 · §17 L3-Definition). **Normatives Mockup:** `docs/superpowers/specs/assets/2026-07-03-lesesaal/lesesaal.html` (v2.4 — bei Zweifel gewinnt das Mockup; **Dokument = Z. 675–804**, **Wissen = Z. 806–843**, **CSS = Z. 20–322**, davon Lese-Ebene Z. 179–228).

---

## Global Constraints

- Branch **`lesesaal-l3`** (frisch off `rebuild` `f4269dd`, ausgecheckt); Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`. **Committe NIE als Planner** — der Orchestrator committet nach Soennes Plan-Review; die Implementer-Dispatches committen am Task-Ende.
- **NIE `make fmt`** ausführen. **NIE `git stash`** in Dispatches. Nach jedem Task: `git log --oneline -3` prüfen, dass HEAD vorangegangen ist (+ `git diff --stat HEAD~1`) — Subagent-Commits können den Branch-Ref verfehlen (Memory).
- `make ci` muss am Task-Ende grün sein (Gate 75 %, aktuell 85,6 %; `*_templ.go` ausgeschlossen; **pgstore-Tests brauchen den Podman-Socket** — `DOCKER_HOST` auf den Podman-Socket setzen; die Provenance-Migration/-Store-Änderung (Task 3) fällt sonst durch verify-generate-freie Docker-Tests).
- Nach JEDER `.templ`-Änderung: `make generate` und die `*_templ.go` mitcommitten. Nach JEDER `web/tailwind.css`-Änderung: `make web` und `internal/adapter/webui/static/app.css` mitcommitten (verify-css ist ein Drift-Diff).
- i18n: jede neue Nutzertext-Zeile in **beiden** Katalogen (`internal/i18n/catalog_de.go` + `catalog_en.go`); de+en-Parität ist test-enforced. Keine hartkodierten Anzeige-Strings.
- Keine Emojis (monospace-Glyphen ● ◆ ⬡ ▶ ■ ⧉ ⌕ + SVG erlaubt), **keine Browser-Popups** (`verify-no-popups` — Copy/Anpinnen ohne `alert/confirm/prompt`; Löschen über `components.ConfirmDialog`).
- **owner-scoped** bleibt überall unangetastet (jede Store-Query trägt `u.ID`; „ist nur ein User" ist keine Begründung, AGENTS.md §Grundsätze). Jede neue Fläche bekommt einen **Owner-Scope-Negativtest**, wo sie fremde Owner-Daten laden könnte (Dokument-View, Wissen-Regale, Suche).
- **Farb-Gesetz (Spec §7):** Farbe pro Projekt existiert NUR im Avatar. **Dokumenttyp-Farben sind fest & semantisch** (`.tc-b/-v/-t/-o/-g/-r`, L2 `DocTypeChipClass`/`DocTypeLabel`) — auf Typ-Chips und Regal-Keys, überall gleich. **Akteure:** Mensch = getönte Initialen; **Agent = gestrichelter neutraler Rahmen** (`.ava.agent`, Bestand `.ava-agent`/`components.AgentAvatar` verifizieren). Keine Kristall-Formcodierung (◆▲●) auf L3-Flächen.
- **Eindämmung (Spec §11 — Markdown ist Fremd-Inhalt):** `html,body{overflow-x:clip}` steht seit L1; Prose `overflow-wrap:break-word`; **jeder breite Block scrollt im eigenen Rahmen** (`pre`, `.tblwrap`, Diagramm-`.frame`); Diagramme behalten mobil ihre Naturgröße und scrollen; Flex-Kinder mit Textinhalt `min-width:0`; der Mono-Pfad in der Provenance-Zeile bricht (`overflow-wrap:anywhere`, Mockup Z.320). Die Seite pannt **nie** horizontal — im Gate 375px prüfen.
- **Design nur über Tokens/Primitives/benannte Klassen** (Gate-Punkt): wo das Mockup harte Maße vorgibt, eine **benannte Klasse** in `web/tailwind.css` anlegen (Task 1) statt Arbitrary-`[px]` zu streuen. Tokens `var(--panel)`/`--sheet`/`--hairp`/`--warn-wash` stehen seit L1.
- **SSE-Regel:** jede Doc-Mutation emittiert ihr Event (`document.created/updated/deleted`, Bestand) und der konsumierende Container ist im Task benannt (`#document-fragment`, `#content` Wissen). Anpinnen/Provenance-Änderung emittiert `document.updated`.
- Tailwind-v4-Fallen (Memory): kein `<alpha-value>` in `@theme`; niemals `*/` in CSS-Kommentaren; `@source not`-Zeilen (`docs/`, `.claude/`) nicht anfassen.
- **rg-Verifikation vor jeder Bestandsnutzung (Prozess-Pflicht):** JEDES als „Bestand" referenzierte Symbol (Template, Helfer, Handler, VM-Feld, Komponente, Usecase-Feld, Test-Helper — z. B. `RenderDocument`, `getDocPolicy`, `DocumentVM`, `DocumentFragment`, `WissenCategories`, `BuildWissenOverview`, `docRowFromDocument`, `wissenResults`, `SearchDocuments.Execute`, `nodeMaps`, `docCols`, `actor.FromContext`, `WikilinkTargets`, `ResolveWikilink`, `components.Backlinks`, `components.Toc`, `DocTypeChipClass`, `AppShell`, `renderToBuf`, `testCtx`, `i18n.WithLocale`) vor dem Tippen per `rg -n "<Name>" internal/ -g '!*_templ.go'` gegen den echten Code prüfen. **Bestand gewinnt** — Signaturen/Feldnamen exakt übernehmen, nichts erfinden.

## Mermaid + CSP — Vorgabe (ENTSCHIEDEN, codex-second-opinion 2026-07-06; NICHT erneut konsultieren)

**Weiche „C-lite" (verbindlich):** vendored `mermaid.min.js` unter `static/vendor/` (Version **pinnen**, LICENSES.md), **lazy nur laden, wenn das gerenderte Dokument tatsächlich Mermaid enthält**; `startOnLoad:false` + explizites idempotentes `mermaid.run()` über eine Renderer-Funktion (initial + `htmx:afterSwap`/`htmx.onLoad`, verarbeitete Container markieren, kein Doppel-Render); **`securityLevel:"strict"`** (Minimum; keine klickbaren Diagramm-Links — gesetzte-Figur-Doktrin), `htmlLabels:false`, `secure`-Defaults behalten (Diagramm-Direktiven dürfen nichts herunterstufen), **NIE loose/antiscript**; Input-Größe capen (Browser-DoS). **Progressive Enhancement:** Codefence bleibt ohne JS lesbar; bei Parse-Fehler sichtbar + kleiner Error-State; bei Erfolg Diagramm als Figur mit Abb.-Nr. + Caption, Quelle zugänglich (`<details>`, collapsed). **KEIN Server-SVG-Cache** (verfrüht bei 3 Docs).

**Flankierend (CSP „skipping is not defensible"):** CSP-Header (`default-src 'self'; script-src 'self' 'nonce-…'; script-src-attr 'none'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; connect-src 'self'`) — erst **Report-Only** testen (Mermaid-SVG-Inline-Styles, SSE `connect-src`, templ `style=`-Attribute), dann enforce. htmx-Härtung `htmx.config.allowEval=false`, `allowScriptTags=false`, `selfRequestsOnly=true`. **Markdown-Sanitizing gegen rohes HTML/`hx-*`-Attribute:** goldmark läuft OHNE `html.WithUnsafe` (rohes HTML wird escaped) + bluemonday `getDocPolicy` (strippt `<script>`, `javascript:`, unbekannte Attribute) — diese Boundary ist **Bestand** und wird in Task 2 verifiziert/erweitert (nur die für Figuren nötigen Elemente werden zugelassen, keine `hx-*`/`on*`/`data-*` für Agent-Content). CSP-Härtung darf SSE/Palette/Dialoge/bestehende Seiten NICHT brechen — eigener Task (9) mit Smoke über alle Flächen.

## Agent-Besetzung & Dispatch-Protokoll (übernommen aus L1/L2, Auditor-Zeilen auf Dokument/Wissen angepasst)

Rollen als Projekt-Agents in `.claude/agents/` (Modell + Effort im Frontmatter fest). Orchestrator-Session `/effort high`. Dispatches nennen das Modell NIE implizit (Memory: nie Fable erben).

| Task | Agent (`subagent_type`) | Modell · Effort |
|---|---|---|
| 1 L3-CSS-Klassen | `lesesaal-implementer` | Sonnet · medium |
| 2 Markdown-Pipeline (Mermaid-Figur + Sanitizer + Lesezeit) | `lesesaal-implementer-deep` | Sonnet · high |
| 3 Provenance-Stamp (Migration + Domain + Store + Usecases) | `lesesaal-implementer-deep` | Sonnet · high |
| 4 mermaid.min.js vendoren + Init-JS + LICENSES | `lesesaal-implementer` | Sonnet · medium |
| 5 Dokument-Seite (Spine + Prov + Prose + ToC-Rail) | `lesesaal-implementer-deep` | Sonnet · high |
| 6 Verweise-Rail (von hier/hierher) | `lesesaal-implementer` | Sonnet · medium |
| 7 Wissen-Seite (Regale nach Typ + Bigsearch + Zuletzt) | `lesesaal-implementer-deep` | Sonnet · high |
| 8 Editor auf Lesesaal-Tokens | `lesesaal-implementer` | Sonnet · medium |
| 9 CSP + htmx-Härtung + Sanitizer-Smoke | `lesesaal-implementer-deep` | Sonnet · high |
| 10 Wiring-Gate | `lesesaal-implementer` | Sonnet · medium |
| jedes Task-Review | `lesesaal-task-reviewer` | Haiku · high |
| Slice-Ende: Whole-Branch | `lesesaal-final-reviewer` | Opus · xhigh |
| Slice-Ende: Design-Treue | `lesesaal-mockup-auditor` | Sonnet · medium |

**Protokoll pro Task:**
1. Dispatch Implementer mit: wörtlichem Task-Text + Global-Constraints-Block + Mermaid/CSP-Vorgabe + „Branch `lesesaal-l3`, Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild`". Ein Task pro Dispatch.
2. Orchestrator verifiziert danach selbst: `git log --oneline -3` (HEAD vorangegangen?) + `git diff --stat HEAD~1`.
3. Dispatch `lesesaal-task-reviewer` mit Task-Text + Commit-Range. `Rejected`/Critical → Fix-Dispatch an denselben Implementer; Minor darf der Orchestrator selbst fixen.
4. Ledger `.superpowers/sdd/progress.md` fortschreiben (Commits, Verdikt, ci-Stand).

**Protokoll Slice-Ende (feste Reihenfolge):**
1. `make ci` grün.
2. **Rest-Sweep** (mechanisch): `gemini-bigcontext` (agy) über `git diff --name-only rebuild..HEAD`; Fallback `code-searcher` (agy/gemini-CLI ggf. tot). Dispatch-Text unten.
3. `lesesaal-final-reviewer` (Range `rebuild..HEAD`) → Findings fixen.
4. `lesesaal-mockup-auditor` → Abweichungen fixen (Referenzzeilen: Dokument Mockup Z.675–804, Wissen Z.806–843, CSS Z.179–322).
5. **Soenne-Live-Gate** (Browser, nicht delegierbar) — inkl. Mermaid-Doc, Wissen-Suche („kompend" findet „Kompendium"), 960px- und 375px-Sichtprobe (kein horizontales Pannen; breite Tabelle/`pre`/Diagramm scrollen im eigenen Rahmen).
6. Nachlauf: Auto-Memory + flow-Mirror des Ledgers/Plans (`flow_update_doc`).

**Dispatch-Text Rest-Sweep (`<RANGE>` = `rebuild..HEAD`):**
> Lies vollständig: alle Dateien aus `git diff --name-only <RANGE>` plus `web/tailwind.css`, `internal/adapter/webui/static/app.css`, `internal/adapter/webui/static/LICENSES.md`. Finde ausschließlich: (a) **Kristall-Reste auf L3-Flächen** (document*.templ/wissen*.templ/editor*.templ/markdownprose.templ/toc.templ/backlinks.templ) — `glass`, `shadow-soft`, `shadow-lift`, `font-display`, `bg-gradient-to-r`, `from-green`/`to-cyan`, `kindToneClass`, `KindGlyph`, `rounded-2xl`/`rounded-3xl` als Karten-Optik, Formcodierungs-Glyphen ◆/▲/●; (b) **Arbitrary-Tailwind-Werte** (`text-[#`, `bg-[#`, `rounded-[`, `shadow-[`, `text-[.`, `text-[1`, `w-[`, `h-[`) auf L3-Flächen, wo eine benannte Lesesaal-Klasse existiert; (c) **verwaiste i18n-Keys** (definiert, nirgends per `T(`/`Tn(` referenziert) und **tote Kategorie-Reste** (`WissenCategories`, `wissen.daily.description`, `/wissen/system`, `wissenCategoryNav`, `wissenOverviewCards`) falls die Regale-nach-Typ-Umstellung sie ersetzt hat; (d) **rohes `hx-`/`on`/`data-` in Sanitizer-Whitelist** (`getDocPolicy`) das Agent-Markdown aktives Verhalten erlauben würde. Ausgabe: gruppierte Liste `Datei:Zeile — Befund`, KEINE Fixes, KEINE Stilurteile.

**Hinweis Memory-Bank:** keine `CLAUDE-*.md` im Repo → `memory-bank-synchronizer` wird übersprungen; Nachlauf ist Orchestrator-Arbeit (Auto-Memory + flow-Mirror).

---

### Task 1: Lesesaal-L3-Komponentenklassen — Lese-Ebene + Wissen als benannte Klassen

**Files:**
- Modify: `web/tailwind.css` (`@layer components`, hinter den L1/L2-Primitives)
- Modify: `internal/adapter/webui/components/styleguide.templ` (Lesesaal-L3-Sektion)
- Test: `internal/adapter/webui/components/styleguide_test.go` (Muster der L2-Sektion; sonst Render-Smoke)

**Interfaces / Produces (für Tasks 2/5/6/7):** benannte Klassen exakt aus dem Mockup-CSS (Z.179–322). **Vor dem Tippen prüfen, was L1/L2 schon hat** (`.prose`, `.callout`, `.frame`, `.spine`, `.pagehead`, `.sect`, `.typechip`, `.tc-*` existieren teils — `rg -n "\.prose\b|\.callout\b|\.frame\b|\.read\b|\.bigsearch\b|\.docrail\b|\.prov\b|\.shelf\b|\.lede\b|\.tblwrap\b" web/tailwind.css`). **Nur Fehlendes ergänzen, Bestehendes an das Mockup angleichen — Bestand gewinnt bei Konflikt nur, wenn es dem Mockup schon entspricht.** Neu/anzugleichen:
- **Lesespalte:** `.read` (Grid `minmax(0,1fr) 250px`, Gap 64, `padding-bottom:80px`), `.prose` (max-width 680px, 15.5/1.72, `overflow-wrap:break-word`, `min-width:0`; `.prose>*+*` Abstand; `.prose h2` mit **Haarlinie oben** `border-top:1px solid rgb(var(--hair))`, `.prose h3`, `.prose strong`, `.prose .lede` Blockquote-Lede, `.prose code`/`pre` auf `--sheet`, `.tblwrap` (`overflow-x:auto`) + `table th` Versalien-Köpfe/`td`), `.wl`/`.wikilink` (akzentblau, `underline dotted`, `.wl:hover` accent-wash — Bestand-Klasse `.wikilink` und Mockup-`.wl` **beide** bedienen, siehe Task 6).
- **Figuren:** `figure`, `figure .frame` (Rahmen + `overflow-x:auto`), `.frame svg{display:block}`, `figcaption` (+ `figcaption b`), `.mermaid-figure`, **`.mermaid-src`/`details`/`summary`** (Task-2-Ausgabe hat einen `<details class="mermaid-src">`-Quelltext-Toggle — eigenes Lesesaal-Styling für `.mermaid-src`/`summary`, sonst rendert das native Disclosure-Dreieck stilbruch; agy-Finding #5), + **Mermaid-Error-State** (`.mermaid-error` faint/warn, Codefence-Quelle sichtbar). `.attach` wird **NICHT** angelegt (Artefakte = L6, Offene Entsch. #4).
- **Callout:** `.callout`/`.callout .m` an das Mockup (warn-wash, Mono-`!`) angleichen — der bestehende Renderer (`markdown_callout.go`) emittiert `.callout callout-<kind>` + `.callout-title`/`.callout-glyph`; die CSS an beide Strukturen anpassen (nicht den Renderer in Task 1 ändern; Renderer-Angleich = Task 2 falls nötig).
- **Doc-Meta-Spalte:** `.docrail` (+ `.docrail .blk` Panel, `.docrail .blk>.eyebrow`, `.docrail .krow` `--hairp`), `.toc a`/`.toc a.here`/`.toc .n` (die Task-1-JS `toc.js` schreibt Links — Klassen so, dass die vorhandenen `toc-h2`/`toc-h3`-Klassen der JS greifen ODER die JS in Task 5/6 angepasst wird; siehe Task 5 Step ToC).
- **Provenance:** `.prov` (flex-wrap, Gap 8, meta), `.prov .mono` (12px, `overflow-wrap:anywhere` <620px), `.prov .dotsep`.
- **Wissen:** `.bigsearch` (1.5px Rahmen, Radius 12, `input{flex:1;min-width:0}` — die `min-width:0`-Lehre aus §11), **`.bigsearch .ico`** (Lupe-Glyph — Mockup nennt sie `.glass`, kollidiert mit der Kristall-`.glass`-Utility + Task-10-Sweep → **umbenennen**, Codex #13), `.shelf .t`/`.shelf .mono`, und die generische Zeile `.row` (Mockup Z.98–107: `.row .grow`/`.t`/`.s`/`.path`/`.right .v`/`.right .k`) — falls L2 nur `.projrow` hat, `.row` **zusätzlich** anlegen ODER die Wissen-Zeilen auf `.projrow` mappen (Task 7 entscheidet die Zeilen-Klasse; Task 1 liefert beide, damit Task 7 frei wählt).
- **Token statt `#fff` (Codex #20, L7-Vorsorge):** wo das Mockup `background:#fff` hartkodiert (`figure .frame`, `.bigsearch`, `.ava.logo`), stattdessen `rgb(var(--surface))`/`rgb(var(--paper))` setzen — billig, spart den Dunkel-Zwilling-Refactor in L7. (base.templ fixiert heute `data-theme=light`, Dunkel erst L7 — daher kein L3-Blocker, nur Hygiene.)
- **Responsive (Mockup Z.284–321) — vollständig:** `@media (max-width:960px)`: `.read{grid-template-columns:1fr;gap:0}`, `.docrail{margin-top:40px;padding-top:0}`, `.prose{max-width:100%}`. `@media (max-width:620px)`: `.row .right .k{display:none}`, `.bigsearch{padding:12px 14px;gap:10px}` + `.bigsearch kbd{display:none}` + `.bigsearch input{font-size:15px}`, `.prose{font-size:15px}`, `.frame svg{max-width:none}` + `figure .frame{padding:14px}`, `.prov .mono{overflow-wrap:anywhere}`. (`.sect-h{flex-wrap:wrap}` existiert aus L2 — prüfen.)
- **Hinweis Mockup-Bug:** `.prose pre code{color:var(--sheet-ink)}` (Mockup Z.189) referenziert ein NICHT definiertes Token `--sheet-ink`. Chroma-Spans (chroma.css) setzen die Code-Farben ohnehin → diese Zeile **weglassen** oder auf `rgb(var(--ink))` setzen. Kein neues Token einführen (Minor, im Task vermerkt).

**Zustände dieser Fläche:** /ui-Styleguide zeigt leer (—-Zeilen), lang (86-Zeichen-`.prov .mono` bricht via `overflow-wrap:anywhere`, breite Tabelle scrollt im `.tblwrap`), mobil 375px (`.read` einspaltig, `.docrail` stackt, `.bigsearch` kompakt) — Sichtprobe im Gate.

- [ ] **Step 1: Mockup-CSS verifizieren + Bestand prüfen**
```bash
sed -n '179,228p' docs/superpowers/specs/assets/2026-07-03-lesesaal/lesesaal.html   # Dokument-Ansicht + prose
sed -n '243,262p' docs/superpowers/specs/assets/2026-07-03-lesesaal/lesesaal.html   # bigsearch + shelf
sed -n '284,321p' docs/superpowers/specs/assets/2026-07-03-lesesaal/lesesaal.html   # responsive
rg -n "\.prose\b|\.callout\b|\.frame\b|\.read\b|\.bigsearch\b|\.docrail\b|\.prov\b|\.shelf\b|\.lede\b|\.tblwrap\b|\.row\b|\.wikilink\b|\.wl\b" web/tailwind.css
```

- [ ] **Step 2: Failing Test** — in `styleguide_test.go` ergänzen (Muster der L2-Sektion; die Komponente heißt **`StyleguidePage()`** (styleguide.templ:6, Codex #14) — nicht `Styleguide()`; Test-Ctx vorher `rg "templ StyleguidePage|func testCtx" internal/adapter/webui/components/` verifizieren):
```go
func TestStyleguide_HasLesesaalL3Section(t *testing.T) {
	var sb strings.Builder
	if err := components.StyleguidePage().Render(testCtx(t), &sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	for _, want := range []string{"read", "prose", "docrail", "bigsearch", "prov"} {
		if !strings.Contains(out, want) {
			t.Fatalf("Styleguide misses Lesesaal-L3 demo of %q", want)
		}
	}
}
```

- [ ] **Step 3: Test laufen lassen** — Expected: FAIL.

- [ ] **Step 4: Klassen in `web/tailwind.css` ergänzen/angleichen** — wörtlich aus dem Mockup (Z.179–228, 243–262, 284–321), `#hex` → `rgb(var(--token))` wo ein Token existiert. Bestehende `.prose`/`.callout`/`.frame`/`.spine` an das Mockup angleichen statt duplizieren. `@source not`-Zeilen nicht anfassen.

- [ ] **Step 5: Styleguide-Sektion** — in `styleguide.templ` eine Sektion „Lesesaal L3": ein `.read`-Grid mit `.prose` (h2 mit Haarlinie, `.lede`, `.tblwrap`+Tabelle, `pre`, `.callout`, ein `figure .frame` mit Placeholder-SVG, eine `.mermaid-error`-Demo) + eine `.docrail` (ToC-Block + Verweise-`.krow`) + eine `.prov`-Zeile + eine `.bigsearch` + zwei `.shelf`-`.row`s. Bestehende Sektionen unangetastet.

- [ ] **Step 6: Bauen + Tests + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... -race
git add -A && git commit -m "feat(lesesaal): L3-Layout-Klassen (Lese-Ebene/prose/figure/docrail/prov/bigsearch/shelf) als benannte Klassen"
```
Expected: PASS; `git status` zeigt geänderte `app.css`.

---

### Task 2: Markdown-Pipeline — Mermaid als gesetzte Figur (AST-Transformer!) + Sanitizer-Boundary + Lesezeit

> **KRITISCHE ARCHITEKTUR-KORREKTUR (Codex-Finding #1/#2, verifiziert an goldmark v1.8.2):** goldmarks `renderer.NodeRenderer`-Dispatch ist **exklusiv pro `ast.NodeKind`** — die niedrigste Prioritätszahl gewinnt den Slot und delegiert NICHT zurück. `goldmark-highlighting` registriert `ast.KindFencedCodeBlock` bei Priorität 200. Ein eigener Renderer auf `KindFencedCodeBlock` (egal welche Priorität) würde **alle** Codeblöcke stehlen → **stille Regression: jeder ` ```go`/` ```bash`-Block verliert Chroma-Highlighting**. DESHALB: **NICHT** einen FencedCodeBlock-Renderer bauen. Stattdessen das **bewährte Callout-Muster** (`markdown_callout.go`): ein **AST-Transformer** ersetzt Mermaid-`*ast.FencedCodeBlock`-Knoten durch einen **eigenen Knotentyp** (`kindMermaid`), und ein Renderer wird für `kindMermaid` registriert — Chromas FencedCodeBlock-Renderer bleibt für alle anderen Sprachen unangetastet.

**Files:**
- Create: `internal/adapter/webui/mermaid.go` + Tests in `internal/adapter/webui/markdown_test.go` (kein Monolith; Muster `markdown_callout.go`)
- Modify: `internal/adapter/webui/wikilink.go` (`RenderDocument`-Signatur + `getDocPolicy` erweitern)
- Modify: `internal/adapter/httpserver/webui_document.go`, `internal/adapter/httpserver/webui_cockpit.go` (:42), `internal/adapter/httpserver/webui_editor.go` (:30, :194) — **alle 4 `RenderDocument`-Caller** an die neue Signatur (rg-Step unten)
- Create: `internal/adapter/webui/readingtime.go` + `internal/adapter/webui/readingtime_test.go`
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go` (Figuren-Caption-Keys)

**Interfaces:**
- **Signatur-Änderung (löst Codex #3 i18n + #4 HasMermaid-Divergenz + #7 Mehrfach-Caller in EINEM Zug):** `RenderDocument(ctx context.Context, src string, resolve WikilinkResolver) (template.HTML, DocMeta)` mit `type DocMeta struct { HasMermaid bool }`. `ctx` erlaubt die i18n-lokalisierte Caption; `DocMeta.HasMermaid` ist die **einzige Quelle** für „lädt Assets?" (kein separater String-Scan `HasMermaid` mehr — der divergierte vom Fence-Parser). Alle 4 Caller anpassen: Doc-Handler nutzt `html, meta := RenderDocument(r.Context(), doc.Body, resolve)`; Cockpit/Editor nutzen `html, _ := …` (Mermaid rendert dort als lesbare Quelle; Diagramm nur wo `mermaid-init.js` läuft — siehe Task 4/5, Offene Entsch. #9).
- **Mermaid-Transformer + -Renderer** (Muster `calloutTransformer`/`calloutHTMLRenderer`): `mermaidTransformer` walkt das Doc, findet `*ast.FencedCodeBlock` mit Info-String `mermaid` (Sprache via `fcb.Language(reader.Source())`) und ersetzt jeden durch einen `mermaidNode{Source, N int}` (Zähler beim Transformieren vergeben). `mermaidHTMLRenderer` (registriert für `kindMermaid`) schreibt je Knoten:
  ```html
  <figure class="mermaid-figure">
    <div class="frame"><pre class="mermaid">…QUELLE (html.EscapeString) …</pre></div>
    <figcaption><b>Abb. N</b> · <span class="mermaid-cap">…RenderedFrom…</span></figcaption>
    <details class="mermaid-src"><summary>Quelle</summary><pre><code>…QUELLE…</code></pre></details>
  </figure>
  ```
  Progressive: ohne JS ist die Quelle im `<pre class="mermaid">` lesbar; `mermaid-init.js` (Task 4) ersetzt sie durch das SVG. **Caption-Lokalisierung ohne per-Node-ctx-Zugriff:** `RenderDocument` löst die zwei Label-Strings **einmal** vor dem Convert auf (`figLabel := components.T(ctx, "document.figure.label")`, `renderedFrom := components.T(ctx, "document.figure.mermaid")`) und gibt sie dem `mermaidHTMLRenderer` als Felder mit; der Renderer interpoliert `figLabel + " " + N + " · " + renderedFrom`. (Exportierten i18n-Helfer `components.T`/`i18n.T` vorher per `rg "func T(" internal/adapter/webui/components/ internal/i18n/` verifizieren.) `DocMeta.HasMermaid` = wurde ≥1 `mermaidNode` erzeugt.
- `webui.ReadingTime(body string) int` — Wortzahl / 220, aufgerundet, **mind. 1**; Frontmatter vorher via `domain.ParseFrontmatter` strippen (Muster aus `RenderMarkdown`). i18n-frei (das templ setzt „N min Lesezeit").
- `getDocPolicy()` (bluemonday, `wikilink.go`) — **minimal** die für Figuren nötigen Elemente/Attribute zulassen: `figure`, `figcaption`, `details`, `summary`, `b`, und `class` auf `figure`/`figcaption`/`details` (`pre`/`code`/`span`+`class` stehen schon). **KEINE** `data-*`, `hx-*`, `on*`, `style`, `id`-Freigabe für Agent-Content über das Bestehende hinaus. **Verifizieren + dokumentieren:** goldmark läuft OHNE `html.WithUnsafe` → rohes HTML im Agent-Markdown wird escaped; bluemonday strippt Rest. Diese Boundary ist der Kern der §11-Fremd-Inhalt-Regel und wird in Task 9 gesmoked.

**Zustände dieser Fläche:** leer (kein Mermaid → Pipeline unverändert, `DocMeta.HasMermaid=false`, Chroma-Highlighting **unangetastet**), lang (Mermaid-Quelle 200+ Zeilen → `.frame` scrollt; Input-Cap client-seitig Task 4), **Fehlerpfad** (kaputter Mermaid-Code → Server rendert trotzdem die Figur mit lesbarer Quelle; Client-Init Task 4 zeigt `.mermaid-error`), mobil (Diagramm behält Naturgröße, `.frame` scrollt — Task 1 `.frame svg{max-width:none}` <620px), **Nicht-Doc-Kontext** (Cockpit-Node-Beschreibung/Editor-Preview: Mermaid-Fence → lesbare `<pre class="mermaid">`-Quelle, kein Diagramm ohne mermaid-init — bewusst, Offene Entsch. #9).

- [ ] **Step 0: rg-Verifikation (Bestand gewinnt)**
```bash
rg -n "func RenderDocument|getDocPolicy|calloutTransformer|calloutHTMLRenderer|highlightingExtension" internal/adapter/webui/ -g '!*_templ.go'
rg -n "RenderDocument\(" internal/adapter/ -g '!*_test.go'          # alle 4 Caller
rg -n "func T\(" internal/adapter/webui/components/ internal/i18n/    # i18n-Helfer
rg -n "TestRenderDocument_CodeHighlightUsesClasses" internal/adapter/webui/markdown_test.go  # der Regressions-Wächter
```

- [ ] **Step 1: Failing Tests** — `markdown_test.go` (DE-Locale-ctx für die „Abb."-Assertion, Muster `activity_row_test.go:51`) + `readingtime_test.go`:
```go
func TestRenderDocument_MermaidBecomesFigure(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	src := "# H\n\n```mermaid\ngraph TD; A-->B\n```\n"
	html, meta := RenderDocument(ctx, src, func(string) (string, string, bool) { return "", "", false })
	out := string(html)
	for _, want := range []string{`class="mermaid-figure"`, `<pre class="mermaid">`, "graph TD", "Abb. 1", "<details"} {
		if !strings.Contains(out, want) { t.Fatalf("mermaid figure misses %q:\n%s", want, out) }
	}
	if !meta.HasMermaid { t.Fatal("meta.HasMermaid must be true") }
	if strings.Contains(out, "<script") || strings.Contains(out, "hx-") { t.Fatalf("sanitizer leaked active markup:\n%s", out) }
}

func TestRenderDocument_TwoMermaidNumbered(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	src := "```mermaid\nA\n```\n\ntext\n\n```mermaid\nB\n```\n"
	out := string(mustHTML(RenderDocument(ctx, src, nilResolve)))
	if !strings.Contains(out, "Abb. 1") || !strings.Contains(out, "Abb. 2") { t.Fatalf("figures must number sequentially:\n%s", out) }
}

// REGRESSIONS-WÄCHTER (Codex #2): normale Codeblöcke behalten Chroma-Highlighting.
func TestRenderDocument_MermaidDoesNotBreakHighlighting(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	src := "```go\nfmt.Println(\"x\")\n```\n\n```mermaid\nA-->B\n```\n"
	out := string(mustHTML(RenderDocument(ctx, src, nilResolve)))
	if !strings.Contains(out, "class=\"mermaid-figure\"") { t.Fatal("mermaid block missing") }
	if !strings.Contains(out, "chroma") && !strings.Contains(out, "class=\"k") { // chroma-Klassen (ClassPrefix "")
		t.Fatalf("go block lost chroma highlighting — mermaid renderer stole FencedCodeBlock:\n%s", out)
	}
}

func TestReadingTime(t *testing.T) {
	if got := ReadingTime(strings.Repeat("wort ", 440)); got != 2 { t.Fatalf("ReadingTime = %d, want 2", got) }
	if ReadingTime("kurz") < 1 { t.Fatal("ReadingTime must be >= 1") }
}

func TestRenderDocument_RawHTMLNeutralized(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	src := `<div hx-get="/x" onclick="alert(1)">x</div>` + "\n\n<script>alert(1)</script>"
	out := string(mustHTML(RenderDocument(ctx, src, nilResolve)))
	if strings.Contains(out, "hx-get") || strings.Contains(out, "onclick") || strings.Contains(out, "<script") {
		t.Fatalf("agent raw HTML must be neutralized:\n%s", out)
	}
}
```
(`mustHTML`/`nilResolve` als kleine Test-Helper; die reale Chroma-Klasse in der Assertion gegen `TestRenderDocument_CodeHighlightUsesClasses` abgleichen — dort steht, welche Klasse chroma mit `ClassPrefix("")` emittiert.)

- [ ] **Step 2: Laufen lassen** — FAIL.

- [ ] **Step 3: `mermaid.go` implementieren** — `mermaidTransformer` (Priorität so, dass er vor dem Rendern läuft; Muster `calloutTransformer{}` bei Priorität 0) ersetzt Mermaid-FencedCodeBlocks durch `mermaidNode`; `mermaidHTMLRenderer{figLabel, renderedFrom string}` für `kindMermaid`. In `RenderDocument` (`wikilink.go`): Signatur auf `(ctx, src, resolve)` + `DocMeta`-Rückgabe; Transformer via `parser.WithASTTransformers` (neben `calloutTransformer`) einhängen, Renderer via `renderer.WithNodeRenderers` (neben wikilink/callout). `DocMeta.HasMermaid` aus dem Transformer-Zähler.

- [ ] **Step 4: Alle 4 Caller + i18n + `getDocPolicy` + `readingtime.go`** — Caller (webui_document.go:37, webui_cockpit.go:42, webui_editor.go:30/194) auf neue Signatur; i18n-Keys `document.figure.label` („Abb." / „Fig.") + `document.figure.mermaid` („gerendert aus ```mermaid" / „rendered from ```mermaid") in BEIDE Kataloge; `getDocPolicy` um Figuren-Elemente; `readingtime.go`.

- [ ] **Step 5: VOLLE Suite (Regressions-Wächter inklusive) + Commit**
```bash
make generate && go test ./internal/adapter/webui/... -race 2>&1 | tail -20   # NICHT nur -run Mermaid — CodeHighlight muss mitlaufen
go test ./internal/adapter/httpserver/... -race 2>&1 | tail -10               # Caller-Anpassung grün
git add -A && git commit -m "feat(lesesaal): Mermaid als gesetzte Figur via AST-Transformer (Chroma unangetastet), nummeriert+i18n; RenderDocument(ctx)→DocMeta; Lesezeit-Helfer; Sanitizer-Whitelist"
```

---

### Task 3: Provenance-Stamp — Akteur bei jeder Doc-Mutation (Migration + Domain + Store + Usecases)

> **Bedingt (Offene Entsch. #1): ENTSCHEIDUNG = Actor-Stamp** (Empfehlung; kein Heuristik-Ersatz möglich, weil `Type.HumanOwned()` die *Editier-Berechtigung* meint, NICHT die *Autorschaft* — 81 % der Docs sind agent-verfasst, viele davon project-typisiert). Wählt Soenne die Alternative (kein Akteur, nur „aktualisiert <Zeit>"), entfällt dieser Task und Task 5 rendert die Prov-Zeile ohne Avatar/Ref.

**Files:**
- Create: `internal/adapter/pgstore/migrations/0028_documents_updated_by.sql` (goose Up/Down — Pflicht-Annotationen, Memory)
- Modify: `internal/domain/document.go` (zwei nullable Felder)
- Modify: `internal/adapter/pgstore/documents.go` (`docCols`/`prefixedDocCols` + Scan + INSERT/UPDATE/Upsert)
- Modify: `internal/usecase/create_document.go`, `internal/usecase/update_document.go`, `internal/usecase/upsert_document_by_path.go` (Stamp aus `actor.FromContext(ctx)`)
- Test: `internal/usecase/create_document_test.go` / `update_document_test.go` / `upsert_document_by_path_test.go`, `internal/adapter/pgstore/documents_test.go`

**Interfaces:**
- Domain: `Document` bekommt `UpdatedByKind string` + `UpdatedByRef string` (leer = unbekannt/pre-L3). Kein `created_by` in L3 (Prov-Zeile zeigt „aktualisiert von …"; created_by wäre Extension — bewusst weggelassen, Offene Entsch. #1).
- Migration `0028`: `ALTER TABLE documents ADD COLUMN updated_by_kind text, ADD COLUMN updated_by_ref text;` (nullable, keine Defaults — Bestandsdaten bleiben NULL → Prov-Zeile rendert neutral). Down = `DROP COLUMN`.
- Stamp: in den drei Write-Usecases `a := actor.FromContext(ctx)` lesen und `doc.UpdatedByKind = string(a.Kind); doc.UpdatedByRef = a.Ref` VOR dem Store-Write setzen. **Verifikationsstep zuerst:** `rg -n "actor.FromContext|actor.WithContext" internal/adapter/httpserver/middleware.go internal/adapter/httpserver/webauth.go` — bestätigen, dass `ctxWithUser` (auth + webAuth) den Actor setzt (verifiziert: middleware.go:28); MCP/Agent setzt `X-Flow-Actor` → `Kind: agent, Ref: <client>`. `SetPinned`/`SetArchived` restampen **NICHT** (Pin/Archiv ist keine Autoränderung).
- pgstore: **ZWEI Spaltenkonstanten** erweitern (Codex-Finding #8 — die Suche nutzt NICHT `docCols`): `docCols` (Get/List/Page/Archived) **und** `prefixedDocCols` (Volltext-`Search` documents.go:328 + `SemanticSearch` :443) je um `updated_by_kind, updated_by_ref` bzw. `d.updated_by_kind, d.updated_by_ref`. **DREI Scan-Helfer** um zwei `sql.NullString`/`*string` in exakt gleicher Spaltenreihenfolge erweitern: `scanDocument`, `scanSearchHit` (documents.go:381), `scanSemanticHit` (documents.go:491). INSERT + `UPDATE documents SET …` (documents.go:174) + Upsert-`INSERT … ON CONFLICT` (documents.go:234) um die zwei Spalten. **Ohne diesen Schritt trägt `domain.SearchHit` keine Provenance → Task 7s Wissen-Zeilen (Meta = „Pfad · Akteur" aus `UpdatedByRef`) wären in Suchergebnissen leer.** **Owner-scoped** unverändert.

**Zustände:** leer/Bestand (NULL → `UpdatedByRef=""` → Task-5-Prov-Zeile ohne Akteur, nur Zeit+Pfad), Mensch (`kind=human`, getönter Avatar + DisplayName), Agent (`kind=agent`, gestrichelter Avatar + Ref), Fehlerpfad (Actor fehlt im ctx → `FromContext` liefert Default `{Kind:Human}` → kein Crash; im Test abgedeckt).

- [ ] **Step 1: Failing Usecase-Test** — z. B. `create_document_test.go`: mit `ctx := actor.WithContext(context.Background(), actor.Actor{Kind: actor.Agent, Ref: "claude-code"})` ein Doc anlegen und asserten `doc.UpdatedByKind == "agent" && doc.UpdatedByRef == "claude-code"`. (Test-Store-Muster + `CreateDocumentInput` vorher per `rg` verifizieren.)
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3: Migration `0028` schreiben** (goose Up/Down mit `-- +goose Up/Down`), Domain-Felder ergänzen, pgstore `docCols`/Scan/INSERT/UPDATE/Upsert erweitern, Stamp in den drei Usecases.
- [ ] **Step 4: Store-Roundtrip-Test (inkl. Suche!)** — `documents_test.go` (testcontainers, `DOCKER_HOST` Podman): Doc mit `UpdatedByKind/Ref` schreiben, per `Get` **und per `Search`** (`prefixedDocCols`-Pfad, Codex #8) zurücklesen, Felder in beiden prüfen; ein Bestands-Doc ohne die Felder (direkter INSERT ohne Provenance) liest als leere Strings zurück (kein Scan-Fehler bei NULL). Wenn ein Embedder verdrahtet ist, auch `SemanticSearch` prüfen; sonst mindestens der Volltext-`Search`-Pfad.
- [ ] **Step 5: Tests grün + `make cover` + Commit**
```bash
DOCKER_HOST=... go test ./internal/usecase/... ./internal/adapter/pgstore/... -race 2>&1 | tail -20
git add -A && git commit -m "feat(document): Provenance-Stamp — updated_by (Akteur Mensch/Agent) bei Create/Update/Upsert (Migr 0028)"
```

---

### Task 4: mermaid.min.js vendoren + lazy Init-JS + LICENSES

**Files:**
- Create: `internal/adapter/webui/static/vendor/mermaid.min.js` (gepinnte Version, MIT)
- Create: `internal/adapter/webui/static/js/mermaid-init.js`
- Modify: `internal/adapter/webui/static/LICENSES.md` (Eintrag Mermaid + Version + MIT)
- Test: `internal/adapter/httpserver/routes_test.go` bzw. ein Static-Serve-Smoke (die Datei wird unter `/static/vendor/mermaid.min.js` ausgeliefert → 200)

**Interfaces:**
- `mermaid.min.js`: eine **gepinnte** UMD/min-Build von mermaid (z. B. `mermaid@11.x`, exakte Version zum Implementierungszeitpunkt festhalten und in LICENSES.md notieren). Keine externe URL, keine CDN (CSP `script-src 'self'`).
- **`mermaid-init.js` lädt die schwere Lib SELBST lazy** (löst Codex #6 „SSE fügt nachträglich Mermaid hinzu" + agy #1/Codex #5 „Asset-Ladefehler ohne Error-State"): das winzige Init wird auf der Dokument-Seite **immer** eingebunden (Task 5, statt `HasMermaid`-bedingter Script-Tags); es fetcht `mermaid.min.js` **nur, wenn ein `pre.mermaid` tatsächlich im DOM ist** (initial ODER nach SSE-Swap). So bleibt die 2,8-MB-Lib lazy (nur bei echtem Diagramm geladen) und funktioniert auch für nachträglich per SSE eingeschwappte Diagramme.
```js
// Rendert alle <pre class="mermaid"> als gesetzte Figuren. Lädt mermaid.min.js
// selbst nach, aber nur wenn ein Diagramm im DOM ist. Idempotent über [data-mm-done].
(function () {
  var MAX = 20000;          // Input-Cap gegen Browser-DoS
  var loading = false, failed = false;
  function markError() { document.querySelectorAll('.mermaid-figure:not(.mermaid-error)').forEach(function (f) { f.classList.add('mermaid-error'); }); }
  function ensureLib(cb) {
    if (window.mermaid) return cb();
    if (failed) return markError();          // Lib zuvor nicht geladen → Figuren markieren, Quelle bleibt lesbar
    if (loading) return;
    loading = true;
    var s = document.createElement('script');
    s.src = '/static/vendor/mermaid.min.js';  // 'self' → CSP script-src ok
    s.onload = function () { loading = false; cb(); };
    s.onerror = function () { loading = false; failed = true; markError(); };
    document.head.appendChild(s);
  }
  function render() {
    var pending = document.querySelectorAll('pre.mermaid:not([data-mm-done])');
    if (!pending.length) return;
    ensureLib(function () {
      window.mermaid.initialize({ startOnLoad: false, securityLevel: 'strict', htmlLabels: false, flowchart: { htmlLabels: false } });
      pending.forEach(function (el) {
        el.setAttribute('data-mm-done', '1');
        if ((el.textContent || '').length > MAX) el.closest('.mermaid-figure').classList.add('mermaid-error');
      });
      try { window.mermaid.run({ querySelector: 'pre.mermaid[data-mm-done]:not(.mermaid-error pre)', suppressErrors: true }); }
      catch (e) { markError(); }
    });
  }
  if (document.readyState !== 'loading') render(); else document.addEventListener('DOMContentLoaded', render);
  document.body.addEventListener('htmx:afterSwap', render);
})();
```
  (Exakte `mermaid.run`-Signatur/-Selektor gegen die gepinnte Version prüfen; `suppressErrors` + `.mermaid-error`-Fallback sind Pflicht — ein kaputtes Diagramm ODER eine nicht geladene Lib darf die Seite nie sprengen; die Quelle bleibt im `<pre>` lesbar. Kein `securityLevel:'loose'`, kein `sandbox` in L3 — Offene Entsch. #2.)

**Zustände:** Doc ohne Mermaid (Lib wird nie gefetcht — `render()` findet kein `pre.mermaid`), Doc mit gültigem Mermaid (Figur rendert), Doc mit kaputtem Mermaid (`.mermaid-error` + lesbare Quelle), **Asset-Ladefehler** (`s.onerror` → `.mermaid-error`, Quelle lesbar — kein nackter Codefence), **SSE fügt Mermaid nach** (`htmx:afterSwap` → `render()` fetcht die Lib on demand), reduced-motion (mermaid animiert nicht kritisch).

- [ ] **Step 1: Failing-Test-first (Struktur wie die anderen Tasks)** — `internal/adapter/httpserver/routes_test.go` (o. ä.): ein Serve-Test, dass `/static/vendor/mermaid.min.js` + `/static/js/mermaid-init.js` **404** liefern (rot, weil noch nicht vendored) — Static-Route ist Bestand (`rg -n "/static/" internal/adapter/httpserver/server.go`). (JS-Logik hat keine Node-Test-Harness — AGENTS.md „No Node runtime" —, deshalb wird Idempotenz/Error-State/Asset-Fail im Live-Smoke Task 10 geprüft; hier deckt der Serve-Test die Auslieferung ab.)
- [ ] **Step 2: Laufen lassen** — FAIL (404).
- [ ] **Step 3: Vendoren + Init** — mermaid.min.js gepinnt in `static/vendor/`; `mermaid-init.js` wie oben; LICENSES.md-Eintrag (Name · Version · MIT · Bezugsquelle). Serve-Test grün.
- [ ] **Step 4: Commit**
```bash
git add -A && git commit -m "feat(lesesaal): mermaid.min.js gepinnt vendored + selbst-lazy Init (securityLevel strict, Asset-Fail-/SSE-sicher) + LICENSES"
```

---

### Task 5: Dokument-Seite Lesesaal — Pfad-Rückgrat + Provenance-Zeile + Lesespalte + ToC-Rail

**Files:**
- Modify: `internal/adapter/webui/document.templ` (`DocumentPage`/`documentBody`/`documentOuter`/`DocumentFragment` neu in Lesesaal-Sprache; `DocumentEmbedBadge` bleibt, an Tokens angleichen)
- Modify: `internal/adapter/webui/document_vm.go` (`DocumentVM`-Felder: Provenance + Spine-Crumbs + Lesezeit + HasMermaid; Kristall-Felder `KindGlyph`/`KindTone`/`ProjectColor` entfernen, soweit tot)
- Modify: `internal/adapter/httpserver/webui_document.go` (`handleWebDocumentView` baut das neue VM; Anpinnen-Route)
- Modify: `internal/adapter/httpserver/server.go` (`POST /wissen/{id}/pin` falls nicht vorhanden — sonst bestehende Pin-API nutzen)
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go`
- Test: `internal/adapter/webui/document_layout_test.go` (+ `internal/adapter/httpserver/webui_document_test.go`)

**Interfaces:**
- Consumes: `s.GetDocument`, `s.ListDocuments` (für Wikilink-Resolve, Bestand), `s.BacklinksDocument`, `s.nodeMaps`, `webui.RenderDocument(ctx, body, resolve) (HTML, DocMeta)` (Task 2), `webui.ReadingTime` (Task 2), `webui.ShortName`/`Initials`/`AvatarTone` (L1/L2), `components.Avatar` + Agent-Avatar (`rg "AgentAvatar|ava.agent|ava-agent"` — Bestand). **Ketten-Crumbs (Codex #9 — Bestandsnamen korrigiert):** `s.NodeAncestors.Execute(ctx, u.ID, *doc.NodeID)` liefert `[]domain.Node` leaf→root (verifiziert webui_cockpit.go:40, server.go:75) — daraus die `.up`-Crumbs bauen. **NICHT** `domain.Ancestors` (existiert nicht) und **NICHT** `nodeCrumbs` (private Cockpit-VM-Logik, nimmt `NodeCockpit`). Ohne `doc.NodeID` → nur „Wissen". Owner-scoped (`u.ID`).
- Produces: neues `DocumentVM` (Mockup Z.675–800). Crumb-Typ **webui-lokal** (`type DocCrumb struct{ Label, Href string }`) — kein Zwang zu `components.Crumb`:
  - Spine (Z.678–687): `.up`-Crumbs (`DocCrumb`, Node-Vorfahren klickbar → `/nodes/{id}`, letztes Segment „Wissen") + `h1` = Titel (nicht Kurzname — Doc-Titel ist die Identität; `word-break`, max-width via `.spine-main h1`).
  - Prov-Zeile (Z.688–695): `@components.Avatar`/Agent-Avatar aus `UpdatedByKind/Ref` (leer → neutraler Avatar, KEIN Ref, nur Zeit+Pfad) + „<b>Ref</b> · aktualisiert <Relativzeit>" + `<span class="mono">doc.Path</span>` + „<N> min Lesezeit" (`ReadingTime`) + Bearbeiten-Link (`/wissen/{id}/bearbeiten`, `hx-boost=false`) + **Anpinnen**-Button (`.btn.btn-q.btn-s`, `hx-post="/wissen/{id}/pin"`, Target `#document-fragment`, `hx-swap="outerHTML"`; Label wechselt „Anpinnen"/„Angepinnt" nach `doc.Pinned`). **Langer `UpdatedByRef` (agy #8):** `<b>` mit `max-width` + `text-overflow:ellipsis` + `title={Ref}` (oder `ShortName`-Analog), damit ein sehr langer MCP-Client-String die Prov-Zeile mobil nicht sprengt.
  - `.read`-Grid: `article.prose` = `@components.MarkdownProse(vm.HTML)` (die Prose-Klasse in `markdownprose.templ` an das Mockup angleichen: `class="prose"` statt `prose min-w-0 w-full max-w-[70ch]` — Task 1 `.prose` trägt max-width 680px + `min-width:0`; **verifizieren**, was `MarkdownProse` heute setzt, sonst dort angleichen).
  - `.docrail` (Z.777–793 ohne den Kontext-Block): **Block „Auf dieser Seite"** = ToC (`components.Toc` an Lesesaal angleichen — `<div class="blk" data-toc-block><span class="eyebrow">…</span><nav class="toc" data-toc-nav></nav></div>`; `toc.js` füllt sie). **Leer-ToC (Codex #10):** `toc.js` muss den umschließenden `[data-toc-block]` per `hidden`/`display:none` **ausblenden, wenn `headings.length === 0`** (sonst bleibt ein leerer `.blk`-Rahmen stehen) — dieser `toc.js`-Touch ist Teil dieses Tasks. **Block „Verweise"** liefert Task 6. **Block „Im Agenten-Kontext"** = **NICHT** in L3 (Offene Entsch. #3 → L5); die Anpinnen-Aktion in der Prov-Zeile deckt die einzige L3-Kontext-Interaktion ab.
  - **Mermaid (Codex #6 — self-lazy):** `documentBody` bindet `mermaid-init.js` (Task 4, winzig) **immer** ein (am Ende, außerhalb `#document-fragment`); das Init fetcht `mermaid.min.js` selbst nur bei vorhandenem `pre.mermaid` (initial ODER nach SSE-Swap). Keine `HasMermaid`-bedingten Script-Tags mehr (die brächen bei nachträglich per SSE eingeschwappten Diagrammen). `DocMeta.HasMermaid` aus Task 2 wird hier nur informativ genutzt (z. B. optional gar nicht — das Init self-detektiert).
- **SSE:** `documentOuter` behält `hx-get="/wissen/{id}"` + `hx-trigger="sse:document.updated"` + `hx-select/hx-target="#document-fragment"` (Bestand). Anpinnen emittiert `document.updated` (Pin-Handler, Bestand `s.SetPinned` + Emit — `rg "handlePinDocument|SetPinned"`; falls nur die JSON-API existiert, einen dünnen Web-Handler `handleWebDocPin` ergänzen, der `SetPinned` ruft, `document.updated` emittiert und das Fragment zurückgibt).

**Zustände (Pflicht benennen):**
- **leer:** Doc mit leerem Body → ruhige Prose, ToC leer (kein Rahmen-Rest), Verweise „keine".
- **lang:** 86-Zeichen-`doc.Path` in `.prov .mono` bricht (`overflow-wrap:anywhere`); breite Tabelle/`pre`/Mermaid scrollen im eigenen Rahmen; langer Titel `word-break`.
- **mobil 375px:** `.read` einspaltig, `.docrail` stackt unter die Prose, `.prose` volle Breite, Prov-Zeile bricht um.
- **laufender Timer:** unbeteiligt (Doc-Seite hat keinen Timer; die Topbar-Pill aus L1 bleibt).
- **Fehlerpfad:** Doc nicht gefunden → 404 (Bestand `handleWebDocumentView` mappt `ErrDocumentNotFound`); kaputter Wikilink → `.wikilink-broken`-Span (Bestand); kaputtes Mermaid → `.mermaid-error` (Task 4).

- [ ] **Step 1: Failing Render-Test** — `document_layout_test.go`:
```go
func TestDocumentFragment_LesesaalSpineProvAndRail(t *testing.T) {
	vm := DocumentVM{
		ID: "d1", Title: "Backstage ↔ GitLab: Token-Integration",
		Path: "docs/gitlab-token-integration", UpdatedByKind: "agent", UpdatedByRef: "Claude",
		ReadMinutes: 18, HTML: template.HTML("<p>x</p>"),
		Crumbs: []DocCrumb{{Label: "RTL Extern", Href: "/nodes/e1"}, {Label: "backstage", Href: "/nodes/r1"}},
	}
	out := renderToBuf(t, testCtx(t), DocumentFragment(vm))
	for _, want := range []string{`class="spine"`, `class="prov"`, "Claude", "docs/gitlab-token-integration", "18", `class="read"`, `class="docrail"`, "data-toc-nav", "/wissen/d1/pin"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doc fragment misses %q:\n%s", want, out)
		}
	}
	for _, gone := range []string{"glass", "shadow-soft", "font-display", "kindToneClass"} {
		if strings.Contains(out, gone) {
			t.Fatalf("kristall remnant %q still present", gone)
		}
	}
}
```
(`Crumb`/`renderToBuf`/`testCtx`/`DocumentVM`-Feldnamen vorher `rg`-verifizieren; `template` importieren.)

- [ ] **Step 2: Laufen lassen** — FAIL.

- [ ] **Step 3: `document_vm.go` umbauen** — `DocCrumb{Label,Href}` + `DocumentVM`: `ID`, `Title`, `Path`, `Crumbs []DocCrumb`, `UpdatedByKind`, `UpdatedByRef`, `UpdatedRel string` (Relativzeit), `ReadMinutes int`, `Pinned bool`, `HTML template.HTML`, `Backlinks`/`Outgoing` (Task 6). Kristall-Felder (`KindGlyph`/`KindTone`/`ProjectColor`/`CategoryHref`/`CategoryLabelKey`) entfernen, soweit die neue Seite sie nicht nutzt (`rg` auf Nutzer prüfen — `DocumentEmbedBadge` bleibt).

- [ ] **Step 4: `document.templ` neu** — Spine + Prov + `.read` + Prose + `.docrail` (ToC-Block; Verweise-Platzhalter für Task 6) + bedingte Mermaid-Assets. Glyph-frei, keine `glass`/`shadow`/`font-display`/Gradient. `DocumentEmbedBadge` an Tokens angleichen (kein `glass`; `.typechip`/benannte Klassen).

- [ ] **Step 5: Handler + Owner-Scope-Negativtest** — `handleWebDocumentView` baut das neue VM: `html, _ := webui.RenderDocument(r.Context(), doc.Body, resolve)` (neue Signatur, Task 2), Crumbs aus `s.NodeAncestors.Execute(r.Context(), u.ID, *doc.NodeID)` (nur wenn `doc.NodeID != nil`), `UpdatedRel` via Relativzeit-Helfer, `ReadMinutes=webui.ReadingTime(doc.Body)`, `UpdatedByKind/Ref` aus `doc`. Anpinnen-Web-Handler `handleWebDocPin` (ruft `s.SetPinned`, emittiert `document.updated`, rendert das Fragment) + Route `POST /wissen/{id}/pin` (`server.go`, statischer Pfad vor `{id}`-Wildcard). **Negativtest:** User A darf ein Doc von User B nicht laden (Bestand `GetDocument` owner-scoped → 404; als Test absichern).

- [ ] **Step 6: i18n** — beide Kataloge:
```go
"document.readtime":     "min Lesezeit",           // en: "min read"
"document.updatedRel":   "aktualisiert %s",         // en: "updated %s"
"document.pin":          "Anpinnen",                // en: "Pin"
"document.pinned":       "Angepinnt",               // en: "Pinned"
"document.pin.hint":     "In den Agenten-Kontext pinnen", // en: "Pin into agent context"
"document.toc":          "Auf dieser Seite",        // en: "On this page"
"common.edit":          — (Bestand prüfen/wiederverwenden)
```
(Die Figuren-Caption-Keys `document.figure.label`/`document.figure.mermaid` liegen bereits in Task 2. Bestehende `wissen.edit`/`wissen.toc`/`common.edit` prüfen und wiederverwenden statt duplizieren.)

- [ ] **Step 7: Bauen + Tests + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -race 2>&1 | tail -20
git add -A && git commit -m "feat(lesesaal): Dokument-Seite — Rückgrat + Provenance-Zeile (Akteur/Zeit/Pfad/Lesezeit) + Lesespalte + ToC-Rail + Anpinnen; Mermaid-Assets bedingt"
```

---

### Task 6: Verweise-Rail — ausgehend „von hier" + Backlinks „hierher"

**Files:**
- Modify: `internal/adapter/webui/document.templ` (Verweise-`.blk` im `.docrail`)
- Modify: `internal/adapter/webui/document_vm.go` (`Outgoing []RefRow` + `Backlinks []RefRow` vereinheitlichen)
- Modify: `internal/adapter/httpserver/webui_document.go` (ausgehende Links auflösen)
- Modify: `internal/adapter/webui/components/backlinks.templ` **oder** ersetzen durch eine Lesesaal-`.krow`-Liste (Kristall-Komponente prüfen)
- Test: `internal/adapter/webui/document_layout_test.go`

**Interfaces:**
- Consumes: `domain.WikilinkTargets(doc.Body)` (Bestand, `wikilink.go:103`) → je Target `domain.ResolveWikilink(doc, target, all)` (Bestand) → ausgehende Verweise („von hier"); `s.BacklinksDocument.Execute(ctx, u.ID, id)` (Bestand) → Backlinks („hierher"). Owner-scoped.
- Produces: `RefRow{ Title, Href, Dir string }` (`Dir` = `document.ref.from`/`document.ref.to`); Rendern als `.krow` (Mockup Z.788–793): `<div class="krow"><a class="n" href=…>↪/↩ Titel</a><span class="v">von hier/hierher</span></div>`. Der bestehende `components.Backlinks`/`components.Backlink` wird durch die `.krow`-Liste ersetzt (Kristall-Optik raus) — **Bestand prüfen** (`rg "templ Backlinks|type Backlink" internal/adapter/webui/components/`), Nutzer außerhalb des Doc (falls keine) → Komponente entfernen/umschreiben, sonst nur im Doc ablösen.
- **Kein neuer Store-Call:** ausgehende Links kommen aus dem schon geladenen `all`-Docs-Slice (`ListDocuments`, Bestand im Handler) + `WikilinkTargets`. Keine zusätzliche Query.

**Zustände:** leer (keine Verweise → „Keine Verweise"-Zeile, kein Rahmen-Rest), lang (langer Doc-Titel im `.krow .n` truncatet mit `title`), mobil (Rail stackt — Task 1), Fehlerpfad (unauflösbarer ausgehender Target → als `.wikilink-broken` in der Prose sichtbar, in der Rail NICHT gelistet — nur aufgelöste ausgehende Links erscheinen).

- [ ] **Step 0: rg-Verifikation (Bestand gewinnt)** — `rg -n "func WikilinkTargets|func ResolveWikilink" internal/domain/wikilink.go` · `rg -n "BacklinksDocument" internal/adapter/httpserver/server.go` · `rg -n "templ Backlinks|type Backlink" internal/adapter/webui/components/` (Nutzer außerhalb des Doc prüfen).

- [ ] **Step 1: Failing Render-Test** — Doc mit einem ausgehenden Wikilink (aufgelöst) + einem Backlink; asserten: beide `.krow`, „von hier"/„hierher"-Labels, Titel, `href="/wissen/…"`; kein `glass`/Kristall.
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3: VM + Handler** — `Outgoing` aus `WikilinkTargets`+`ResolveWikilink` bauen (nur aufgelöste, de-dupliziert), `Backlinks` aus `BacklinksDocument`; beide als `RefRow`. `document.templ` Verweise-`.blk` rendern.
- [ ] **Step 4: i18n** — `document.refs`: „Verweise" / „References"; `document.ref.from`: „von hier" / „from here"; `document.ref.to`: „hierher" / „to here"; `document.refs.empty`: „Keine Verweise" / „No references".
- [ ] **Step 5: Bauen + Tests + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -race 2>&1 | tail -20
git add -A && git commit -m "feat(lesesaal): Verweise-Rail — ausgehende (von hier) + Backlinks (hierher) als Haarlinien-Zeilen"
```

---

### Task 7: Wissen-Seite Lesesaal — Regale nach Typ + Bigsearch + Zuletzt aktualisiert

**Files:**
- Modify: `internal/adapter/webui/wissen.templ` (`WissenFragment`/`wissenOverviewCards`→Regale; `wissenSearchBar`→`.bigsearch`; `wissenResults` an Lesesaal-Zeilen; Kategorie-Seiten → Typ-Regal-Liste)
- Modify: `internal/adapter/webui/wissen_vm.go` (`WissenShelves()` type-basiert; `BuildWissenOverview` liefert Shelf-Counts + Zuletzt; `docRowFromDocument` → Lesesaal-Row mit `DocTypeChipClass`/`DocTypeLabel`)
- Modify: `internal/adapter/httpserver/webui_wissen.go` (Overview-Daten + Shelf-Liste; Summary-Zahlen)
- Modify: `internal/adapter/httpserver/server.go` (Typ-Regal-Route + Redirect der Alt-Kategorie-Slugs)
- Modify: `internal/i18n/catalog_de.go` + `catalog_en.go`
- Test: `internal/adapter/webui/wissen_vm_test.go`, `internal/adapter/httpserver/webui_wissen_test.go`

**Interfaces:**
- Produces: `webui.WissenShelf{ Type domain.DocumentType (oder Set); LabelKey, DescKey, TypeKey string; Count int }` + `WissenShelves()` = **7 Regale nach Typ** (Mockup Z.821–830): Projekt-Notizen `project` · Pläne `plan` · Specs `spec` · Erinnerungen `memory` · Tagesnotizen `daily` · Systemkontext `context` (= `activecontext`+`instruction`+`skill`) · Frei `free`. `DocAgent` (deprecated) → in das `spec`-Regal falten (B3d: was agent→spec). Ersetzt `WissenCategories()` (daily/projekte/frei/system).
- Produces: `WissenOverviewVM` neu — `Summary` („264 Dokumente · 12 angepinnt · 382 Verweise"; Zahlen: `len(docs)`, `count(Pinned)`, Σ `len(WikilinkTargets(body))` **oder** ein billiger Count; wenn Verweis-Σ teuer, weglassen und nur „N Dokumente · M angepinnt" — Offene Entsch. #5), `Shelves []WissenShelf`, `Recent []WissenRowVM` (Zuletzt aktualisiert, **Cap 8 + „Alle N ›"** wie L2-Wissen-Sektion, carry-forward #5).
- Produces: **NEUES** `WissenRowVM{ ID, Title, ChipClass, ChipLabel, Meta, TimeStr string }` (Meta = „Pfad · Akteur" wie Mockup Z.834; Akteur aus `UpdatedByRef`, sonst Node/Path). tc-*-Chips **verdrahtet** (`DocTypeChipClass`/`DocTypeLabel`, L2-Bestand — carry-forward #1). **NICHT `DocRow`/`docRowFromDocument` umbauen (Codex #18):** die teilen sich `BuildHomeNewest`/`homeNewestDocRow` (home_newest.go / home.templ) mit der **Schreibtisch-Seite (L4, noch Kristall)** — ein Umbau bräche Home. `WissenRowVM` ist eine eigene, zusätzliche Anzeige-Struktur; `DocRow` bleibt unangetastet bis L4. (`rg -n "docRowFromDocument|BuildHomeNewest|type DocRow" internal/adapter/webui/` verifizieren.)
- **SSE (agy-Finding: T7 muss den Container benennen):** der `#content`-Wrapper der Wissen-Übersicht behält `hx-get="/ui/wissen/list"` + `hx-trigger="sse:document.created, sse:document.updated, sse:document.deleted"` + `hx-swap="innerHTML"` (Bestand `wissenOuter` — `rg -n "wissenOuter|/ui/wissen/list" internal/adapter/webui/wissen.templ`), damit Regale-Counts + „Zuletzt aktualisiert" nach jeder Doc-Mutation live nachziehen. Bei der Umstellung NICHT verlieren.
- **Bigsearch** (`.bigsearch`, Mockup Z.815–819): `<form method="get" action="/wissen">` mit `<input type="search" name="q" placeholder="Suchen — Volltext und semantisch. „kompend" findet Kompendium.">` + Lupe-Glyph + `⌘K`-kbd. **Lupe-Klasse NICHT `.glass`** (Mockup Z.816 nutzt `class="glass"`, kollidiert mit der Kristall-`.glass`-Utility und dem Task-10-Sweep, Codex #13) → benannte Klasse `.bigsearch .ico` (Task 1). **Backend existiert** (`s.SearchDocuments.Execute` → `[]domain.SearchHit`, `wissenResults`) — nur die Optik auf `.bigsearch` + Ergebnis-Zeilen auf Lesesaal-Zeile umstellen. Tags bleiben **Filter im Suchergebnis** (Bestand `wissenTagChips`), nie Wand (Offene Entsch. #6).
- **Routen:** Shelf-Klick → `/wissen/typ/{type}` (neuer Handler, listet Docs eines Typs als Lesesaal-Zeilen + Pagination; `context` = 3 Typen als Set). Alt-Slugs → Redirect (kein toter Link, Offene Entsch. #7): `/wissen/daily`→`/wissen/typ/daily`, `/wissen/projekte`→`/wissen/typ/project`, `/wissen/frei`→`/wissen/typ/free`. **`/wissen/system` hat KEIN 1:1-Ziel (Codex #17):** die Alt-„system"-Kategorie bündelte `DocAgent/DocMemory/DocInstruction/DocSkill/DocPlan`, die im 7-Regal-Schema auf **plan · memory · context · spec** verteilt sind → `/wissen/system` **redirectet auf `/wissen`** (Übersicht mit allen Regalen). Die Asymmetrie im Task vermerken.

**Zustände:** leer (kein Doc → ruhige „Noch keine Dokumente"-Zeile, Regale mit Count 0 dezent), lang (86-Zeichen-Pfad in `.row .s`/`.path` bricht), mobil 375px (`.bigsearch` kompakt, `.row .right .k` weg), laufender Timer (unbeteiligt; Topbar-Pill bleibt), Fehlerpfad (Suche-Backend-Fehler → 500 wie Bestand; leere Suche → „keine Treffer"), **Suche-Recall** („kompend" findet „Kompendium" — FTS+pg_trgm, Bestand; im Gate live prüfen).

- [ ] **Step 0: rg-Verifikation (Bestand gewinnt)** — `rg -n "func SearchDocuments|SearchDocuments\b" internal/adapter/httpserver/server.go` · `rg -n "DocTypeChipClass|DocTypeLabel" internal/adapter/webui/` · `rg -n "wissenResults|wissenOuter|wissenTagChips|handleWebWissenCategory" internal/adapter/webui/wissen.templ internal/adapter/httpserver/webui_wissen.go` · `rg -n "docRowFromDocument|BuildHomeNewest|type DocRow" internal/adapter/webui/` (Home-Kopplung nicht brechen).

- [ ] **Step 1: Failing VM-Test** — `wissen_vm_test.go`: `WissenShelves()` liefert 7 Regale mit den richtigen Typ-Keys; `BuildWissenOverview(docs, …)` zählt je Regal korrekt (inkl. `activecontext`+`instruction`+`skill`→context, `agent`→spec) und liefert `Recent` cap 8. **`DocRow`/`docRowFromDocument` bleiben unverändert** (eigener Assert, dass `BuildHomeNewest` weiter kompiliert/rendert).
- [ ] **Step 2: Laufen lassen** — FAIL.
- [ ] **Step 3: `wissen_vm.go`** — `WissenShelves()` + Shelf-Counting + `WissenRowVM`-Bau mit `DocTypeChipClass`/`DocTypeLabel`; `Recent` cap 8. Alte `WissenCategories()`/`WissenCategoryCard`/`wissenOverviewCards`-Pfade ersetzen (tote Reste in Task 10 sweepen).
- [ ] **Step 4: `wissen.templ`** — `pagehead` + `.bigsearch` + `.sect.shelf` (Regale als `.row` mit `.mono` Typ-Key + `.right .v` Count) + `.sect` „Zuletzt aktualisiert" (`.row` mit `.typechip`+ChipClass + Titel + Meta + Zeit, „Alle N ›"). Kristall raus (glass/gradient/font-display). `wissenResults` an Lesesaal-`.row`.
- [ ] **Step 5: Handler + Routen + Owner-Scope-Negativtests** — Overview-Daten (Summary-Zahlen, Shelves, Recent) owner-scoped; `/wissen/typ/{type}` + Alt-Slug-Redirects (statische Pfade vor `{id}`-Wildcard `/wissen/{id}` beachten — Reihenfolge in `server.go`; `/wissen/system`→`/wissen`). **Owner-Scope-Negativtests für ALLE drei Flächen (Global Constraint + Codex #11):** (a) Regale/Recent auf `/wissen`, (b) **Suche `/wissen?q=`** (`s.SearchDocuments`), (c) **Regal-Listing `/wissen/typ/{type}`** — je: User A sieht kein Doc von User B.
- [ ] **Step 6: i18n** — beide Kataloge:
```go
"wissen.libraryTitle":   "Die Bibliothek",          // en: "The library"
"wissen.summary":        "%d Dokumente · %d angepinnt", // en: "%d documents · %d pinned"
"wissen.search.placeholder": "Suchen — Volltext und semantisch. „kompend“ findet Kompendium.", // en gleichwertig
"wissen.shelves":        "Regale",                   // en: "Shelves"
"wissen.recent":         "Zuletzt aktualisiert",     // en: "Recently updated"
"wissen.recent.all":     "Alle %d",                  // en: "All %d"
"wissen.shelf.project":  "Projekt-Notizen",  "wissen.shelf.project.desc": "Entscheidungen, Doku und Kontext zu Deinen Projekten",
"wissen.shelf.plan":     "Pläne",            "wissen.shelf.plan.desc":    "Implementierungspläne aus Brainstorm und Spec",
"wissen.shelf.spec":     "Specs",            "wissen.shelf.spec.desc":    "Design-Dokumente — validiert und versioniert",
"wissen.shelf.memory":   "Erinnerungen",     "wissen.shelf.memory.desc":  "Was die Agenten über die Arbeit gelernt haben",
"wissen.shelf.daily":    "Tagesnotizen",     "wissen.shelf.daily.desc":   "Journal — ein Dokument pro Tag",
"wissen.shelf.context":  "Systemkontext",    "wissen.shelf.context.desc": "Active Context, Instruktionen, Skills",
"wissen.shelf.free":     "Frei",             "wissen.shelf.free.desc":    "Lose Ideen ohne festen Ort",
```
(en-Parität für jeden Key; bestehende `wissen.search`/`wissen.noresults*`/`wissen.title` prüfen/wiederverwenden.)
- [ ] **Step 7: Bauen + Tests + Commit**
```bash
make generate && make web && go test ./internal/adapter/webui/... ./internal/adapter/httpserver/... -race 2>&1 | tail -20
git add -A && git commit -m "feat(lesesaal): Wissen-Seite — Bigsearch + Regale nach Typ + Zuletzt aktualisiert (tc-Chips), Alt-Kategorie-Slugs umgeleitet"
```

---

### Task 8: Editor auf Lesesaal-Tokens (minimal restyle, kein Redesign)

**Files:**
- Modify: `internal/adapter/webui/editor.templ`
- Modify: `internal/adapter/webui/editor_vm.go` (nur falls Klassen-Strings dort liegen)
- Test: `internal/adapter/httpserver/webui_editor_test.go` (Assertions auf entfallene Kristall-Klassen anpassen, nie Verhalten wegtesten)

**Interfaces:** kein Verhaltens-/Routen-Change. Nur Optik: `glass`/`shadow-soft`/`shadow-lift`/`bg-gradient-to-r`/`from-green`/`to-cyan`/`font-display`/`rounded-2xl`-Karten → Lesesaal-Tokens/Primitives (`.panel`, `.btn`/`.btn-pri`/`.btn-q`, benannte Klassen; Ein Primär-Button pro Sicht). Preview-Fläche nutzt dieselbe `.prose` wie die Dokument-Seite (Konsistenz). `/wissen/neu`, `/wissen/{id}/bearbeiten`, Preview, Create, Update, Delete bleiben funktional identisch.
- **`RenderDocument`-Caller:** die Editor-Preview (webui_editor.go:30/194) wurde in Task 2 bereits auf die neue Signatur gezogen → Mermaid-Fences erscheinen im Preview als lesbare `<pre class="mermaid">`-Quelle. **Optional (Empfehlung: mitnehmen, billig):** `mermaid-init.js` auch auf der Editor-Preview-Fläche einbinden, damit der Autor das Diagramm gerendert sieht (self-lazy, Task 4) — sonst als Nice-to-have deferren.
- **`wissenCategoryHrefAndLabel` bleibt (Codex #19):** die Funktion (webui_document.go:95) wird von webui_editor.go:139 für den „zurück zur Kategorie"-Link genutzt; da die Alt-Kategorie-Routen in Task 7 als **Redirect** erhalten bleiben (Offene Entsch. #7), verlinkt der Editor weiter gültig. Falls Soenne die Alt-Routen ersatzlos streicht, muss dieser Editor-Link hier mit auf ein Typ-Regal umgezogen werden.

**Zustände:** leer (neues Doc, leere Felder), lang (langer Titel/Pfad im Input), mobil 375px (Formular einspaltig), Fehlerpfad (Validierungsfehler-Anzeige an Tokens angleichen), Preview (die `.prose`-Vorschau matcht die Leseseite).

- [ ] **Step 1: Bestand prüfen** — `rg -n "glass|shadow|gradient|font-display|rounded-2xl|from-green|to-cyan" internal/adapter/webui/editor.templ`.
- [ ] **Step 2: Restyle** — Kristall-Klassen durch Lesesaal-Primitives ersetzen; Preview auf `.prose`.
- [ ] **Step 3: Tests reparieren** (Kristall-Assertions → Lesesaal), `make generate && make web && go test ./internal/adapter/... -race 2>&1 | tail`.
- [ ] **Step 4: Commit** — `git add -A && git commit -m "feat(lesesaal): Editor + Preview auf Lesesaal-Tokens (kein Redesign)"`.

---

### Task 9: CSP-Header + htmx-Härtung + Sanitizer-Smoke

**Files:**
- Create: `internal/adapter/httpserver/security_headers.go` (Middleware) + `internal/adapter/httpserver/security_headers_test.go`
- Modify: `internal/adapter/httpserver/server.go` (Middleware in die Web-Kette; Nonce in ctx)
- Modify: `internal/adapter/webui/components/base.templ` (Nonce an die zwei Inline-`<script>` Z.43/59; **`<meta name="htmx-config">`** im `<head>`)
- Test: `security_headers_test.go` + Smoke (Task 10)

**Interfaces:**
- **CSP** (zuerst `Content-Security-Policy-Report-Only`, nach grünem Smoke auf `Content-Security-Policy` flippen): `default-src 'self'; script-src 'self' 'nonce-<pro Request>'; script-src-attr 'none'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; connect-src 'self'; font-src 'self'`. **Inline-Scripts:** `base.templ` hat zwei Inline-`<script>` (Theme-Init Z.43, Live-Timer Z.59 — vorher `rg -n "<script>" internal/adapter/webui/components/base.templ` verifizieren) → **Nonce** anhängen (Middleware legt Nonce in ctx, templ liest ihn). `style-src 'unsafe-inline'` bewusst (Tailwind ist extern, aber htmx-Indicator-Styles, Mermaid-SVG-Inline-Styles und templ-`style=`-Attribute (`projectSwatchStyle`/`swatchStyle`) brauchen es; strenge style-src wäre ein separater, größerer Refactor mit geringem Sicherheitsgewinn — Offene Entsch. #8).
- **htmx-Härtung via `<meta>` (Codex #12 — ein Skript VOR `htmx.min.js` kann `window.htmx` noch nicht setzen):** im `<head>` von `base.templ` `<meta name="htmx-config" content='{"allowEval":false,"allowScriptTags":false,"selfRequestsOnly":true,"includeIndicatorStyles":false}'>`. htmx liest diese Meta beim Laden — kein Timing-Problem, kein zusätzliches Inline-Script (CSP-freundlich, `<meta>` ist kein Script). `includeIndicatorStyles:false` vermeidet den htmx-Inline-`<style>` (entlastet style-src). Danach verifizieren, dass htmx weiter swappt (Palette/SSE/Dialoge).
- **Sanitizer-Boundary bestätigen** (Kern §11): Test, dass Agent-Markdown mit `<script>`, `<div hx-get>`, `onclick`, `javascript:`-href durch `RenderDocument` neutralisiert wird (Teil in Task 2; hier der End-to-End-Smoke über die echte Doc-Seite).
- CSP darf **SSE** (`connect-src 'self'` deckt `/api/v1/events`), **Palette** (`palette.js`, self), **Dialoge** (`dialog.js`), **Mermaid** (`script-src 'self'` deckt die vendored Lib UND das dynamisch injizierte `<script src="/static/vendor/mermaid.min.js">` aus mermaid-init) und **bestehende Seiten** nicht brechen.

**Zustände:** jede Seite (Home/Projekte/Cockpit/Wissen/Dokument/Zeit/Editor) lädt ohne CSP-Verstoß; SSE verbindet; Palette/Dialoge öffnen; Mermaid rendert (inkl. dynamischem Lib-Nachladen); Report-Only sammelt Verstöße bevor enforce.

- [ ] **Step 0: rg-Verifikation** — `rg -n "<script>|<script " internal/adapter/webui/components/base.templ` (die zwei Inline-Scripts) · `rg -n "htmx-config|htmx.config" internal/adapter/webui/` · `rg -n "func .*webAuth|mux.Handle" internal/adapter/httpserver/server.go | head` (wo die Web-Kette hängt).
- [ ] **Step 1: Failing Test** — `security_headers_test.go`: ein Request durch die Middleware trägt einen `Content-Security-Policy-Report-Only`-Header mit `script-src 'self'` + einem `nonce-`; zwei Requests haben **verschiedene** Nonces.
- [ ] **Step 2: Middleware + Nonce-in-ctx** implementieren; in die Web-Kette hängen (nach Auth, vor Handler). `base.templ`-Inline-Scripts mit `nonce={ … }`; `<meta name="htmx-config">` in den `<head>`.
- [ ] **Step 3: htmx weiter funktionsfähig** verifizieren (Palette/SSE/Dialoge swappen nach der Meta-Härtung).
- [ ] **Step 4: Report-Only-Smoke** (Dev-Stack, Browser-Konsole): alle Flächen ohne Verstoß; DANN im selben Task auf `Content-Security-Policy` (enforce) flippen und erneut smoken. Falls ein Verstoß nicht auflösbar ist → **Report-Only belassen** und als Offene Entsch. #8 dokumentieren (kein Blocker).
- [ ] **Step 5: Bauen + Tests + Commit**
```bash
make generate && make web && go test ./internal/adapter/httpserver/... -race 2>&1 | tail -20
git add -A && git commit -m "feat(security): CSP (nonce script-src) + htmx-Härtung (allowEval/scriptTags/selfRequestsOnly); Sanitizer-Boundary bestätigt"
```

---

### Task 10: Wiring-Gate — Leichen-Sweep, tote i18n, volles CI, Live-Smoke, Breakpoints

**Files:** nur was der Sweep findet (+ die Nachbuchen-Dialog-Mini-Untersuchung).

- [ ] **Step 1: Composition-Root prüfen** — `rg -n "GetDocument|ListDocuments|SearchDocuments|BacklinksDocument|SetPinned|CreateDocument|UpdateDocument|UpsertDocumentByPath" cmd/flow-server/main.go` — alle Doc/Suche-Usecases sind bereits verdrahtet; **kein neuer Server-Feld/Konstruktor** in L3 (Provenance-Migration läuft über goose, kein main.go-Change). Falls ein neuer Web-Handler (Pin) eine neue Route braucht, ist sie in `server.go` (Task 5), nicht in main.go.
- [ ] **Step 2: Leichen-Sweep**
```bash
rg -n "glass|shadow-soft|shadow-lift|font-display|bg-gradient-to-r|from-green|to-cyan|kindToneClass|KindGlyph|rounded-3xl|◆|▲|●" \
  internal/adapter/webui/document.templ internal/adapter/webui/wissen.templ internal/adapter/webui/editor.templ \
  internal/adapter/webui/components/markdownprose.templ internal/adapter/webui/components/toc.templ internal/adapter/webui/components/backlinks.templ --glob '!*_templ.go'
rg -n "WissenCategories|WissenCategoryCard|wissenOverviewCards|wissenCategoryNav|/wissen/system|wissenCategoryHrefAndLabel" internal/adapter/webui/ internal/adapter/httpserver/ --glob '!*_templ.go'
```
Expected: 0 Kristall-Reste auf **L3-Flächen** (Home/Schreibtisch bleibt bewusst Kristall = L4 — `home*.templ`/`home_newest.go`/`homeNewestDocRow` NICHT im Sweep-Scope, Codex #18). Der `glass`-Sweep darf **nicht** die umbenannte Bigsearch-Lupe treffen (`.bigsearch .ico`, kein `glass` — Codex #13); schlägt er dort an, ist die Umbenennung aus Task 1/7 nicht durchgezogen. `wissenCategoryHrefAndLabel` bleibt (Editor-Nutzer, Codex #19). Tote Kategorie-Symbole entweder entfernt oder bewusst als Redirect belassen (Task 7). Jeden echten Treffer beseitigen.
- [ ] **Step 3: Tote i18n-Keys** — für in Task 7 ersetzte Kategorie-Keys (`wissen.daily.description`, `wissen.notes.description`, `wissen.free.description`, `wissen.system.description`, `wissen.overview`, `wissen.categories`) prüfen und aus BEIDEN Katalogen entfernen, wenn nirgends mehr per `T(`/`Tn(` referenziert (de+en-Parität bleibt).
- [ ] **Step 4: Nachbuchen-Dialog-Mini-Fix (carry-forward #4, gebudgetet)** — `rg -n "session-dialog|data-dialog|SessionDialog|afterRequest|htmx:afterOnLoad" internal/adapter/webui/static/js/dialog.js internal/adapter/webui/components/*.templ` — prüfen, ob der Nachbuchen-Dialog nach erfolgreichem Submit selbst schließt. Wenn ein billiger Fix (z. B. `dialog.js` schließt bei `htmx:afterOnLoad` mit 2xx auf ein `[data-dialog-close-on-success]`-Formular): einbauen + Mini-Test. **Wenn nicht billig: explizit deferren** (im Ledger vermerken, nicht stumm lassen).
- [ ] **Step 5: Volles CI**
```bash
make ci    # lint, verify-generate, verify-css, verify-no-popups, cover ≥75 %, build; DOCKER_HOST auf Podman-Socket
```
- [ ] **Step 6: Live-Smoke** (Dev-Stack; Cookie-Flow wie L1/L2-Gate, Bearer trifft webAuth nicht)
```bash
make dev-run &   # https://localhost:8080 (self-signed)
sleep 2
# Browser-Session (Cookie): echte Doc-ID einsetzen
for p in /wissen "/wissen/<DOC-ID>" "/wissen/typ/plan" "/wissen?q=kompend"; do echo "$p — 200, Lesesaal rendert"; done
```
Expected: `/wissen` zeigt Bigsearch + Regale nach Typ + Zuletzt; `/wissen?q=kompend` findet „Kompendium" (FTS+trgm); ein Doc mit Mermaid rendert die Figur (mit JS) bzw. lesbare Quelle (ohne JS); Provenance-Zeile zeigt Akteur/Zeit/Pfad/Lesezeit; Verweise-Rail zeigt beide Richtungen; ToC füllt sich; Anpinnen togglet live (SSE `document.updated`); CSP wirft keine Konsolen-Verstöße. Danach Server stoppen.
- [ ] **Step 7: Breakpoint-Sichtprobe für Soenne notieren** (Abschlusstext): **≤960px** (`.read`/`.docrail` stacken, Prose volle Breite) und **375px** (kein horizontales Pannen; breite Tabelle/`pre`/Mermaid scrollen im eigenen Rahmen; `.bigsearch` kompakt; `.prov .mono` bricht).
- [ ] **Step 8: Abschluss-Commit (falls der Sweep etwas fand)**
```bash
git add -A && git commit -m "chore(lesesaal): L3-Gate — Leichen-Sweep + tote Keys + Nachbuchen-Dialog + Live-Smoke"
```

---

## Offene Entscheidungen (Soennes Wahl — mit Empfehlung + Trade-offs)

> Die Task-Texte oben sind **nach den Empfehlungen** geschrieben. Wählt Soenne anders, greifen die genannten Kollaps-/Alternativpfade. Entscheidung am Ausführungsstart.

1. **Provenance: Actor-Stamp (Migration) vs. kein Akteur.** — *Empfehlung: Actor-Stamp (Task 3).* Es gibt **keinen** brauchbaren Heuristik-Ersatz: `Type.HumanOwned()` meint die MCP-Editier-Berechtigung, nicht die Autorschaft (81 % der Docs sind agent-verfasst, viele davon `project`-typisiert → Heuristik labelt falsch). Das `actor`-Modell (`actor.FromContext`, Kind human/agent + Ref) existiert und wird schon von der Activity genutzt → sauberer, gut abgegrenzter Stempel (2 nullable Spalten, Stamp in 3 Write-Usecases). Trade-off: eine Migration + pgstore-Touch; Bestandsdaten (264 Docs) bleiben NULL → Prov-Zeile ohne Akteur (nur Zeit+Pfad), füllt sich über neue Writes. **Alternative:** kein Akteur in L3 (nur „aktualisiert <Zeit>" + Typ) → Task 3 entfällt, Prov-Zeile ohne Avatar/Ref; präzise Attribution kommt in einem späteren Slice.
2. **Mermaid securityLevel: `strict` (inline SVG) vs. `sandbox` (iframe).** — *Empfehlung: `strict`.* Inline-SVG ist zugänglich (Screenreader/`role="img"`), styleable, ohne iframe-/A11y-Reibung, und braucht kein `frame-src` in der CSP. `sandbox` isoliert stärker, bringt aber iframe-Fokus-/Größen-/A11y-Reibung und `frame-src 'self'`. Trade-off: `strict` erlaubt mermaid, SVG in den DOM zu schreiben (kein aktives JS; `htmlLabels:false` + `securityLevel:'strict'` unterbinden Link-/Script-Vektoren). **Alternative sandbox** falls Soenne maximale Isolation über Zugänglichkeit stellt.
3. **docrail „Im Agenten-Kontext"-Block (enthalten ✓ · Rang 04/24).** — *Empfehlung: in L3 NUR die Anpinnen-Aktion (Prov-Zeile), den enthalten/Rang-Statusblock nach L5.* Anpinnen ist billig (Bestand `SetPinned` + `document.updated`). „enthalten ✓ · Rang 04/24" verlangt eine Compose-Auswertung je Dokument (`ComposeContext.Execute` je Node-Scope + Position finden) — das koppelt an die Kontext-Kuratierung, die explizit **L5** ist (Spec §17). Trade-off: die Doc-Meta-Spalte ist in L3 kürzer als im Mockup (ToC + Verweise statt + Kontext) — bewusst. **Alternative:** enthalten/Rang read-only in L3 mitnehmen (falls eine billige Per-Node-Compose-Query steht) — dann eigener Sub-Task.
4. **Artefakt-/Anhang-Figur (Mockup Z.766–774).** — *Empfehlung: in L3 NICHT bauen, auch kein Platzhalter.* Attachment-Storage existiert nicht → **L6** (Spec §16.6). Die `.attach`-Klasse wird in Task 1 bewusst weggelassen. Trade-off: der Anhang-Block des Mockups fehlt in L3 — dokumentiert.
5. **Wissen-Summary-Suffix „· N Verweise untereinander".** — *Empfehlung: nur „N Dokumente · M angepinnt" in L3; die Verweis-Σ weglassen, wenn sie einen Full-Body-Scan aller Docs kostet.* Trade-off: Mockup zeigt drei Zahlen, L3 zwei. **Alternative:** Verweis-Σ mitnehmen, falls ein billiger Count (z. B. aus dem Link-Index) vorliegt (`rg` prüfen) — dann dritte Zahl.
6. **Tags als Suchfilter-UI.** — *Empfehlung: bestehende `wissenTagChips` als Filterzeile im Suchergebnis behalten (Bestand), nie als Wand auf der Übersicht* (Spec §9/§3). Trade-off: minimal — die Übersicht zeigt keine Tags, nur die Suche.
7. **Alt-Kategorie-Routen (`/wissen/{daily,projekte,frei,system}`).** — *Empfehlung: auf die Typ-Regale **umleiten** (Redirect), Alt-Handler als dünne Redirect-Shims behalten* (kein toter Link aus Lesezeichen/alten Docs). **Alternative:** Alt-Handler + Templates ersatzlos entfernen (sauberer, aber bricht alte Deep-Links). Trade-off: Shim = etwas Altlast vs. saubere Deep-Link-Kontinuität.
8. **CSP enforce vs. Report-Only + style-src.** — *Empfehlung: auf enforce flippen, wenn der Smoke 0 Verstöße zeigt; `style-src 'self' 'unsafe-inline'` bewusst (Tailwind extern, aber htmx-Indicator-/Mermaid-/templ-Inline-Styles brauchen es).* Der Sicherheitsgewinn liegt in `script-src 'self' 'nonce'` + `script-src-attr 'none'`. **Alternative:** Report-Only belassen (kein aktiver Schutz, aber Null-Bruch-Risiko) falls ein Verstoß nicht auflösbar ist; strenge `style-src` = eigener späterer Refactor (templ-`style=` → Klassen). Trade-off: `unsafe-inline` bei style ist ein akzeptiertes Restrisiko (kein Script-Vektor).
9. **Mermaid-Diagramme außerhalb der Dokument-Seite (Cockpit-Node-Beschreibung, Editor-Preview).** — *Empfehlung: `RenderDocument` emittiert die Mermaid-Figur überall (die Signatur ist eh geteilt, Codex #7), aber die gerenderte Grafik (mermaid-init) läuft in L3 nur auf der Dokument-Seite; Cockpit/Editor-Preview zeigen die lesbare `<pre class="mermaid">`-Quelle. Editor-Preview optional mit mermaid-init nachrüsten (Task 8, billig).* Trade-off: im Cockpit bleibt ein Mermaid-Block in der Node-Beschreibung als Quelltext statt Grafik — bewusst (Node-Beschreibungen sind selten Diagramme). **Alternative:** mermaid-init auch im Cockpit einbinden (mehr Flächen, mehr Smoke).

---

## Self-Review-Appendix

### Grounding-Herkunft
- **Primär: First-Hand-Reads** (First-Hand ist kanonisch): Spec §9–17 vollständig; Mockup CSS Z.20–322 + Dokument Z.675–804 + Wissen Z.806–843; L2-Formatvorbild vollständig; und der echte Code: `document.templ`, `document_vm.go`, `webui_document.go`, `wissen.templ`, `wissen_vm.go`, `webui_wissen.go`, `markdown.go`, `wikilink.go` (inkl. `getDocPolicy`, `WikilinkTargets`, `ResolveWikilink`), `markdown_callout.go`, `markdown_chroma.go`, `components/markdownprose.templ`, `components/toc.templ`, `components/base.templ` (Inline-Scripts Z.43/59), `components/appshell.templ`, `documents.go` (Search-REST), `search_documents.go`, `domain/document.go` (KEIN created_by/updated_by → carry-forward #2 bestätigt), `domain/search.go`, `domain/user.go`, `internal/actor/actor.go` (`FromContext` Kind/Ref), `middleware.go`/`webauth.go` (`ctxWithUser` setzt Actor für API **und** WebUI — Provenance-Pfad bestätigt), `pgstore/documents.go` (`docCols` zentral, nächste Migration `0028`), `server.go` (alle Doc/Wissen/Suche-Routen + Usecase-Felder; **kein CSP-Header vorhanden**), `static/toc.js`. Bestehende Lesesaal-Klassen in `web/tailwind.css` per `rg` erhoben (`.prose`/`.callout`/`.frame`/`.spine`/`.pagehead`/`.sect`/`.typechip`/`.tc-b` vorhanden; `.docrail`/`.prov`/`.bigsearch`/`.shelf`/`.toc`/`.tblwrap`/`.lede`/`.mermaid` fehlen → Task 1).
- **Degradations-Modus (vermerkt):** das parallele `agy`-Dossier (gemini-bigcontext) wurde asynchron dispatcht, lieferte zur Abgabe aber **keine Ausgabe** (0-Byte-Scratch-Datei; agy-Async-Job nicht rechtzeitig fertig — vgl. [[reference_gemini_cli_oauth_dead]]-Klasse). Deshalb ist das Grounding **first-hand** (alle 35 Ziel-Dateien-Oberflächen selbst gelesen); kein Abbruch, da jede im Plan verwendete Signatur direkt am Code verifiziert ist.
- **Flow-Recall:** `flow_search_docs` (project-scope, type plan) für „Lesesaal L3" — nur L1/L2-Pläne (kein neuerer Remote-Stand); lokale Dateien kanonisch.

### Spec-Deckung L3 (§17-Scope) — jeder Spec-Absatz auf einen Task gemappt
- §9 Zeile **Dokument** (Lesespalte 680 + Meta-Spalte, Prov-Zeile) → T5 · §9 Zeile **Wissen** (Suche-Haupttür, Regale nach Typ, Zuletzt) → T7.
- §10 **Provenance** (wer Mensch/Agent · wann · Typ) → T3 (Stamp) + T5 (Render); **Kontext-Instrument-Randnote** („Im Agenten-Kontext · enthalten ✓ · Rang" + Anpinnen) → Anpinnen T5, enthalten/Rang **bewusst L5** (Offene Entsch. #3, Spec §17 Kontext-Kuratierung = L5).
- §11 **Lese-Ebene komplett**: H2-Haarlinie/Tabellen-Versalien/Code-`sheet`/Lede/Warn-Callout → T1+T5; **Mermaid als gesetzte Figur (Nr.+Unterschrift)** → T2+T4; **Wikilinks + Backlinks/ausgehend in Meta-Spalte** → T6; **Eindämmungs-Systemregel** (overflow-clip/break-word/eigener Scroll-Rahmen/min-width:0) → T1 (CSS) + Gate T10 (375px). **Artefakte als Figuren** → **bewusst L6** (Offene Entsch. #4, Spec §16.6 „Storage existiert nicht").
- §13 **A11y/Motion/i18n**: Fokus (L1-Bestand), `role="img"`+Label an Mermaid-SVG (T4), Kontraste, i18n de/en Parität (jeder Key-Step) → T1/T2/T4/T5/T6/T7.
- §16.7 **Mermaid-Render-Weg** → T2/T4 (C-lite, entschieden). §16.8 **created_by/updated_by prüfen** → T3 (geprüft: fehlt; ergänzt). §16.9 **Lesezeit** → T2.
- Querschnitt: **CSP/Offline** (§16.7 „CSP/offline beachten") → T9. **NICHT in L3 (bewusst, Spec §17):** Schreibtisch+Zeit (L4), Kontext-Kuratierung/Meter/enthalten-Rang (L5), Artefakte/Anhänge (L6), Dunkel-Zwilling (L7). Projekte-Summary-Suffix „seit <Datum>" (L2-carry-forward #6) ist eine **Projekte**-Fläche (L2), nicht L3 → bleibt deferred.

### Carry-forwards — Verbleib
1. **tc-*-Chips auf Wissen-Flächen** → eingearbeitet (T7, `DocTypeChipClass`/`DocTypeLabel`).
2. **Provenance-Feld fehlt in `domain.Document`** → eingearbeitet (T3 Migration + Stamp; Offene Entsch. #1 mit Alternative).
3. **Lesezeit** → eingearbeitet (T2 `ReadingTime`).
4. **Nachbuchen-Add-Dialog schließt nicht** → gebudgetete Mini-Untersuchung (T10 Step 4), Fix-wenn-billig-sonst-explizit-deferren.
5. **Zuletzt-Cap 8 + „Alle N ›"** → eingearbeitet (T7 `Recent`).
6. **Projekte-Summary „seit <Datum>"** → bewusst weiter deferred (L2-Fläche, nicht L3-Scope).

### Planner-Selbstprüfung (Raster a–d, VOR den Beratern)
- **(a) Spec-Absatz ohne Task:** keiner im L3-Scope (Mapping oben); L4–L7-Absätze bewusst außerhalb.
- **(b) Zustände je Task:** leer/lang/mobil-375/laufender-Timer/Fehler in T1–T2–T5–T6–T7 explizit; T3/T4/T8/T9 mit ihren relevanten Zuständen.
- **(c) Querschnitte:** main.go-Wiring → T10 Step 1 (kein neuer Server-Feld; Migration via goose); SSE je Mutation → `document.*` benannt (T5 Anpinnen, T7 Liste); i18n beide Kataloge → jeder Key-Step; Responsive → T1 (960/620) + Gate; Owner-Scoping → Negativtests in T5/T7, `u.ID` in jedem Handler-Step.
- **(d) Tests + rg-Verifikation:** jeder Task failing-Test-first; alle Bestandsnamen unter Global-Constraint „rg-Verifikation" + task-lokale Verifikationssteps; „Bestand gewinnt".

### Adversariale Lückensuche — Berater-Findings + Verbleib
Beide Berater liefen gegen Spec + Mockup + Plan-Entwurf: **`codex exec`** (via codex-second-opinion, 20 Findings inkl. 4 selbst-ergänzte des Gemini-Wrappers) und **`agy`/Gemini** (8 Findings + 4 selbst-verworfene False-Positives). Beide prüften ihre Rohfunde per `rg` gegen den echten Code. Jeder verbleibende Fund ist unten einzeln verbucht. Bei Überschneidung ist die Herkunft genannt.

**CRITICAL (Architektur):**
1. **[eingearbeitet — Task 2 komplett umgeschrieben]** (Codex #1/#2, verifiziert an goldmark v1.8.2) Ein Mermaid-Renderer auf `ast.KindFencedCodeBlock` hätte **alle** Codeblöcke von Chroma gestohlen (exklusiver NodeKind-Dispatch, kein Fallthrough) → stille, breite Highlighting-Regression, die der `-run 'Mermaid'`-Filter nicht gefangen hätte. → Task 2 nutzt jetzt das **AST-Transformer + eigener-Knoten-Muster** (wie `markdown_callout.go`), Chroma bleibt unangetastet; der Regressions-Wächter `TestRenderDocument_MermaidDoesNotBreakHighlighting` + volle Suite (nicht `-run`-gefiltert) in Step 5.

**Eingearbeitet (Codex):**
2. **[eingearbeitet]** #3 i18n: `RenderDocument` hatte kein `ctx` für die Mermaid-Caption → Signatur `RenderDocument(ctx, src, resolve) (HTML, DocMeta)`; Caption-Label-Strings einmal via `components.T(ctx, …)` aufgelöst; Test asserted die DE-Strings mit `i18n.WithLocale`.
3. **[eingearbeitet]** #4 `HasMermaid`-String-Scan divergierte vom Fence-Parser → als separater Helfer **gestrichen**; `DocMeta.HasMermaid` aus dem Transformer ist die einzige Quelle.
4. **[eingearbeitet]** #6 SSE-Swap fügt nachträglich Mermaid hinzu, Assets fehlen → `mermaid-init.js` (Task 4) **lädt die Lib selbst lazy** (bei jedem `render()` inkl. `htmx:afterSwap`), Doc-Seite bindet das Init immer ein (Task 5), keine `HasMermaid`-bedingten Script-Tags mehr.
5. **[eingearbeitet]** #7 `RenderDocument` hat 3 weitere Caller (cockpit :42, editor :30/:194) → Task 2 aktualisiert **alle 4** (rg-Step 0); Mermaid-außerhalb-Doc als Offene Entsch. #9 dokumentiert.
6. **[eingearbeitet]** #8 Suche nutzt `prefixedDocCols` + eigene `scanSearchHit`/`scanSemanticHit` (nicht `docCols`) → Task 3 erweitert beide Spaltenkonstanten + drei Scanner + Such-Roundtrip-Test (sonst wären Wissen-Zeilen-Akteure in Suchergebnissen leer).
7. **[eingearbeitet]** #9 nicht-existente Namen (`domain.Ancestors`/`nodeCrumbs`) → Task 5 nutzt `s.NodeAncestors.Execute` (verifiziert) + webui-lokales `DocCrumb`; Test-Snippet korrigiert.
8. **[eingearbeitet]** #10 Leer-ToC lässt leeren `.blk`-Rahmen → `toc.js` blendet `[data-toc-block]` bei 0 Headings aus (Task 5).
9. **[eingearbeitet]** #11 Owner-Scope-Negativtest fehlte für Suche + `/wissen/typ/{type}` → Task 7 Step 5 nennt alle drei Flächen.
10. **[eingearbeitet]** #12 htmx-Config-Script „vor htmx.min.js" hätte `window.htmx` nicht gehabt → Task 9 nutzt `<meta name="htmx-config">` (kein Timing-Problem, CSP-freundlich).
11. **[eingearbeitet]** #13 Mockup-`.glass`-Lupe kollidiert mit Kristall-`.glass` + Sweep → `.bigsearch .ico` (Task 1/7), Sweep-Notiz (Task 10).
12. **[eingearbeitet]** #14 `components.Styleguide()` heißt real `StyleguidePage()` → Task-1-Snippet korrigiert.
13. **[eingearbeitet]** #17 `/wissen/system` hat kein 1:1-Redirect-Ziel (5 Typen → 4 Regale) → redirectet auf `/wissen`-Übersicht, Asymmetrie im Task vermerkt (Task 7).
14. **[eingearbeitet]** #18 `DocRow`/`docRowFromDocument` teilt sich `BuildHomeNewest`/Home (L4, noch Kristall) → Task 7 baut ein **neues** `WissenRowVM`, lässt `DocRow` unangetastet; Sweep-Scope schließt Home aus (Task 10).
15. **[eingearbeitet]** #19 `wissenCategoryHrefAndLabel` von Editor genutzt → bleibt (Redirect-Pfad), Task 8 + Sweep vermerken es.
16. **[eingearbeitet]** #20 Mockup-`#fff`-Hartkodierungen → Task 1 nutzt `--surface`/`--paper`-Token (L7-Vorsorge). Dunkel-Sichtprobe bewusst L7 (base.templ fixiert light).
17. **[eingearbeitet]** #5 `mermaid-init.js` ohne Test + kein Failing-first → Task 4 bekommt Failing-Serve-Test-first-Struktur; JS-Logik (keine Node-Harness, AGENTS.md) im Live-Smoke (Task 10) geprüft.
18. **[begründet abgelehnt — bewusst deferred]** #15 §10 Kontext-Randnote (enthalten/Rang) + #16 §11 Artefakte: bereits als Offene Entsch. #3/#4 mit Spec-§17-Slicing (L5/L6) dokumentiert. Der strenge (a)-Raster nennt sie als „Spec-Absatz ohne L3-Task" — korrekt, aber Spec §17 autorisiert das Deferral explizit. Anpinnen (die L3-Kontext-Interaktion) IST in Task 5.

**Eingearbeitet (agy):**
19. **[eingearbeitet]** agy#1 Asset-Ladefehler ohne Error-State → `mermaid-init.js` `s.onerror` → `.mermaid-error` (Task 4).
20. **[eingearbeitet]** agy#2 Task 7 benannte keinen SSE-Container → Task 7 Interface nennt jetzt den `#content`-Wrapper + `sse:document.*`-Trigger (Bestand `wissenOuter` erhalten).
21. **[eingearbeitet]** agy#3 = Codex #11 (Suche-Negativtest) → Task 7.
22. **[eingearbeitet]** agy#4 fehlende Task-lokale rg-Steps in T6/T7/T9 → je ein Step 0 rg-Verifikation ergänzt.
23. **[eingearbeitet]** agy#5 `.mermaid-src`/`details`/`summary` ohne CSS → Task 1 Figuren-Block ergänzt.
24. **[eingearbeitet]** agy#6 `ReadingTime`-Signatur-Wortlaut widersprüchlich → auf `int` vereindeutigt (Task 2).
25. **[eingearbeitet]** agy#7 i18n-Key `document.figure.mermaid` in T2 konsumiert, in T5 katalogisiert → die Figuren-Keys wandern nach Task 2 (dort konsumiert + katalogisiert), Task-5-i18n-Notiz angepasst.
26. **[eingearbeitet]** agy#8 langer `UpdatedByRef` ohne Truncation → Prov-Zeile `<b>` mit ellipsis+`title` (Task 5).

**False-Positives (von den Beratern selbst verworfen, kein Plan-Change):** `overflow-x:clip`/`prefers-reduced-motion` fehlen (existieren seit L1), `role="img"`-Mermaid-Pflicht (agy-Fehllesung: §13 meint das Budget-**Meter** = L5), `wissenTagChips`/`dialog.js`/`palette.js` „nicht im Dossier" (existieren real, korrekt als Bestand benannt).

**Dissens:** keiner — die Berater überschnitten sich (Suche-Negativtest, Asset-Fail, i18n-Ordering) ohne Widerspruch; codex ging bei der goldmark-Architektur tiefer, agy bei den prozessualen rg-Steps/Zuständen. Beide Sichten sind eingearbeitet.
