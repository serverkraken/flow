# flow WebUI — Overhaul / Feature-Complete Redesign (Design Spec)

- **Datum:** 2026-06-23
- **Branch:** `rebuild`
- **Status:** Draft — zur Review
- **Design-Richtung:** „Studio" (gewählt aus 3 Vorschlägen A/B/C)
- **Verwandt:** [[2026-06-23-flow-project-management-design]] (Projekte/M4), [[2026-06-23-worktime-import-design]] (Quelle der neu zuzuweisenden Sessions)
- **Mockups (Design-Referenz):** `docs/superpowers/specs/assets/2026-06-23-webui/` (Studio-Set, alle Surfaces, Dark/Light)

---

## 1. Kontext & Ziel

Die bestehende WebUI (`internal/adapter/webui` templ + `internal/adapter/httpserver` Handler, htmx + Tailwind v4, SSE, OIDC-Cookie) deckt Worktime/Frei/Stats/Export/Docs/Projekte bereits funktional ab — aber **ohne Design-System, ohne i18n, mit CDN-Abhängigkeit, fragiler CSS-Build, lückenhaftem Markdown und ohne durchdachte Multi-Page-IA**.

Dieses Vorhaben überarbeitet die WebUI zu einer **feature-completen, mobil + Desktop gleichermaßen guten** Oberfläche mit einem **wiederverwendbaren Design-System (Dark + Light), i18n (Deutsch primär)** und allen Kern-Flächen als eigene Seiten — voll lauffähig im Docker-Image (offline, kein CDN).

### Erfolgskriterien
1. Jede Domäne hat eine vollständige, eigene Seite (Multi-Page); Dokumente öffnen in eigener Seite.
2. Ein einheitliches, semantisches Design-System mit **Dark + Light** (Umschalter, persistent, respektiert OS-Präferenz), gespeist aus dem Glyphen-/Farb-Erbe des TUI.
3. **DRY:** eine wiederverwendbare templ-Komponenten-Bibliothek; keine pro-Seite duplizierten HTML-Gerüste.
4. **i18n:** alle Strings über eine Übersetzungsschicht, Deutsch primär, weitere Sprachen anflanschbar.
5. **Mobil + Desktop** beide erstklassig (responsive, mobiles Bottom-Tab, Desktop-Sidebar; Desktop nicht „Mobile skaliert").
6. **Live:** Worktime tickt live; alle SSE-getriebenen Flächen aktualisieren sich automatisch; Docs aktualisieren live.
7. **Markdown feature-complete** im Web (GFM-Tabellen/Tasklists/Strikethrough, Callouts, Syntax-Highlighting, Fußnoten, Wikilinks + Backlinks, Frontmatter).
8. **Worktime-Historie** als Kalender (Woche/Monat) mit **Bulk-Neuzuweisung** für importierte Sessions (auswahl-basiert).
9. **Docker:** Image baut CSS selbst, bündelt alle Assets lokal (htmx, Fonts) — keine externen Origins zur Laufzeit.

### Non-Goals (YAGNI)
- Keine SPA / kein JS-Framework / keine Node-Laufzeit. Server-rendered bleibt.
- Keine neue Auth-Architektur (OIDC-Cookie für Web, Bearer für API bleibt).
- Keine Mehrbenutzer-Features über das bestehende Per-User-Modell hinaus.
- Keine zweite UI-Sprache *ausliefern* (nur die i18n-Schicht + DE-Katalog; EN-Katalog als Stub). Übersetzen kann später erfolgen.
- Kein Drag-to-resize von Kalender-Blöcken in v1 (Zeiten ändern via Edit-Formular). Kann später kommen.

---

## 2. Entscheidungen (mit Begründung)

| Entscheidung | Wahl | Begründung |
|---|---|---|
| Stack | templ + htmx + Tailwind, server-rendered, ein Binary | Architektur-treu (server-autoritativ), kein Node, embeddet ins Binary, Docker-tauglich. SPA wäre Scope-Explosion + Auth-Umbau. |
| Design-Richtung | „Studio" | Vom Nutzer aus A/B/C gewählt: modern, übersichtlich, ruhig; Farbe als Akzent; trägt datendicht (Monats-Kalender) auf Desktop. |
| Themes | Dark **+** Light über semantische CSS-Variablen | Nutzer-Anforderung; `data-theme` + Toggle + `localStorage` + `prefers-color-scheme`. „Bunter" = nur Token-Werte hochziehen. |
| i18n | Übersetzungsschicht, DE primär | Nutzer-Anforderung; Strings raus aus Templates → Katalog. |
| IA | Multi-Page (echte Routen); Docs eigene Seite | Nutzer-Anforderung; passt perfekt zu server-rendered (jede Route = URL, htmx swap für Partials *innerhalb* einer Seite). |
| Assets | lokal gebündelt (go:embed), kein CDN | „muss im Docker komplett laufen" (offline/airgapped); erlaubt strikte CSP. |
| CSS-Build | im Docker-Build + CI-Guard | Beseitigt den „app.css manuell committen"-Stolperstein. |
| Bulk-Reassign | auswahl-basiert (nicht muster-basiert) | Importierte Sessions tragen „fast nur Zeiten" → Sehen → Auswählen → Zuweisen. |
| Dialoge | in-design `<dialog>`-Komponenten, **keine** Browser-Popups | einheitliches Design + A11y; kein `window.confirm/alert/prompt`. |
| Destruktiv | immer Bestätigung (`ConfirmDialog`) | Schutz vor versehentlichem Datenverlust. |
| Pagination | serverseitig, von Anfang an | Listen (Sessions/Docs/Suche) skalieren — nach dem Import besonders viele Einträge. |

---

## 3. Design-System

### 3.1 Tokens (semantisch, Dark + Light)
Alle Farben als **CSS Custom Properties** in zwei Theme-Blöcken (`:root` light, `:root[data-theme="dark"]`), gemappt auf Tailwind-`theme.extend.colors` via `rgb(var(--x) / <alpha-value>)`. So flippen alle Utility-Klassen automatisch.

Semantische Rollen (aus dem TUI-Erbe, web-getunt — siehe `direction-b-studio.html`):
- **Foundation:** `canvas`, `surface`, `sunken`, `line`, `ink`, `body`, `muted`, `faint`
- **Akzent/Status:** `accent`(blue) · `active`(cyan, live) · `success`(green) · `warning`(yellow) · `notice`(orange) · `danger`(red) · `highlight`(purple) · `schedule`(blue)
- **Projekt-Hues (9, Whitelist):** blue/cyan/green/purple/magenta/yellow/orange/red/teal — pro Theme kontrast-getunt (light: abgedunkelt, dark: Tokyonight-hell).
- **Day-Off-Kinds (7):** Feiertag=schedule, Urlaub=highlight, Krank=notice, Gleittag=success, Sonderurlaub=warning, Kind-krank=danger, Fortbildung=info.
- **Doc-Kinds:** Daily=accent ●, Projekt=success ◆, Frei=highlight ○, Agent/System=warning ▪.

> „Bunter"-Option: Akzent-Sättigung/Flächigkeit der Tokens Richtung „Bunt" (C) anheben — reine Token-Änderung, keine Markup-Änderung.

### 3.2 Glyphen-Erbe
Das Glyphen-Vokabular bleibt sinntragend (kein Deko): `▶ ■ ‖ ✓ ✗ ▲ ▼ ● ○ ★ ☼ ✚ ▎ ▰▱ › · █▓▒░ ◆`. Als Inline-Unicode (eine Zelle), `aria-hidden` wo dekorativ, sonst mit `aria-label`.

### 3.3 Typografie
- **Display:** Clash Display (Headlines, große Zahlen)
- **Body:** Inter
- **Mono:** JetBrains Mono (Zeiten, Dauern, Zahlen, Code — tabular numerals)
Alle Fonts **lokal gebündelt** (woff2, `@font-face` → `/static/fonts`), kein Google-Fonts/Fontshare-CDN zur Laufzeit.

### 3.4 Komponenten-Bibliothek (DRY)
Wiederverwendbare templ-Komponenten unter `internal/adapter/webui/components/` (je Verantwortung eine Datei, „keine Monolithen"). Jede hat klare Parameter (Interface) und ist isoliert testbar:

- **Shell/Layout:** `Base` (HTML-Hülle: Head, Tokens, Fonts, htmx, Theme-Boot-Script), `AppShell` (Desktop-Sidebar + Mobile-Topbar + Bottom-Tab), `Nav`, `SubNav`/`TabStrip`, `Breadcrumb`, `ThemeToggle`, `UserMenu`
- **Primitives:** `Button`(Varianten), `IconButton`, `Card`, `StatTile`, `Badge`, `Chip`/`Tag`, `ProgressBar`(Varianten: hit/over/under/running), `PaceDots`, `Glyph`, `EmptyState`, `Toast`, `Dialog`(in-design Modal, gestyltes `<dialog>`)/`ConfirmDialog`/`Popover`, `Dropdown`, `Pagination`, `FuzzyPicker`(Projekt-Picker, MRU + inline „✚ neu"), `DatePicker`, `SearchInput`, `FilterBar`
- **Domain:** `ProjectChip`(Farbe+Glyph+Name), `SessionRow`, `SessionBlock`(Kalender), `DayRow`(Woche), `KennzahlenPanel`, `WeekTotalBanner`, `DayOffChip`, `DocRow`, `DocCard`, `MarkdownProse`, `Toc`, `Backlinks`
- **Pattern Fragment/Page:** jede Seite = `XPage` (volle Hülle) + `XFragment` (htmx-Teil-Refresh), wie bisher.

### 3.5 Theming-Mechanik
`data-theme` auf `<html>`; No-Flash-Boot-Script im `<head>` setzt das Theme vor Paint aus `localStorage('flow-theme')` → sonst `prefers-color-scheme`. `ThemeToggle` (☀/☾) flippt + persistiert. `prefers-reduced-motion` unterdrückt Übergänge.

### 3.6 Dialoge, Bestätigungen & keine Browser-Popups (verbindliche Regel)
- **Keine nativen Browser-Popups** — kein `window.alert/confirm/prompt`, keine nativen Form-Validierungs-Bubbles. *Alles* läuft über in-design-Dialoge im flow-Design.
- **`Dialog` (wiederverwendbar):** gestyltes `<dialog>` mit Focus-Trap, Esc-/Backdrop-Schließen, `aria-modal`, Rückfokus auf den Auslöser. Basis für alle modalen Inhalte: Edit-Popover (Session/Doc), Projekt-Picker, Formulare, Bestätigungen.
- **`ConfirmDialog` für jede destruktive Aktion** (Session/Dokument/Projekt/Day-Off löschen, Bulk-Löschen, ICS-Token neu generieren [invalidiert den alten]): klarer Konsequenz-Text in UI-Stimme, sichere Default-Aktion (Abbrechen fokussiert), destruktiver Button in `danger`. **Keine destruktive Aktion ohne Bestätigung.**
- Validierung **inline & in-design**, nicht über native Tooltips.

---

## 4. Informationsarchitektur / Sitemap

### Routen
| Route | Seite | Live |
|---|---|---|
| `/` | **Heute** (Worktime live: Timer, heutige Sessions, Wochenstreifen, Tagesziel) | ja |
| `/woche` | **Woche** (Tagesbalken + WOCHE GESAMT + KENNZAHLEN) | ja |
| `/historie` | **Historie** (Kalender Woche/Monat + Bulk-Neuzuweisung) | ja |
| `/stats` | **Stats** (Burndown, Streak, Range-Stats, Tags) | ja |
| `/frei` | **Frei** (Day-Offs, Feiertage, ICS) | ja |
| `/export` | **Export** (CSV/JSON/MD, Projekt-Aggregat × Satz) | — |
| `/wissen` | **Wissen** (kategorie-spezifische Liste, Suche, Tag-Filter) | ja |
| `/wissen/{slug}` | **Dokument** (Lese-Seite, volles Markdown + Backlinks + ToC) | ja |
| `/wissen/neu`, `/wissen/{slug}/bearbeiten` | **Editor** | — |
| `/projekte` | **Projekte** (Liste) | ja |
| `/projekte/{id}` | **Projekt-Hub** (Sammelstelle) | ja |
| `/einstellungen` | **Einstellungen** (Bundesland, Wochenziele, ICS-Token, Theme) | — |
| `/auth/*` | Login/Callback/Logout (OIDC) | — |

### Navigation
- **Top-Nav (Desktop-Sidebar / Mobile-Bottom-Tab):** Heute · Wissen · Projekte · Stats — plus Menü für Frei · Export · Einstellungen · Abmelden · ☀/☾.
- **Worktime-Sub-Nav** (auf Heute/Woche/Historie/Stats/Frei): Tab-Strip `Heute · Woche · Historie · Stats · Frei`. Export als Drill aus Stats/Historie.
- (Exakte Top-Nav-Gruppierung ist Feinschliff im ersten Plan-Slice.)

---

## 5. Surfaces (pro Seite)

> Jede Seite referenziert ihr Mockup in `assets/2026-06-23-webui/`. Alle: Dark/Light, responsive, deutsch, SSE-live wo markiert, leere/Fehlerzustände definiert.

### 5.1 Heute (`/`) — `direction-b-studio.html`
Live-Session-Karte (▶ Projekt, große tickende Dauer, Stopp), Start (+ Projekt-Fuzzy-Picker), Heute-gesamt vs Tagesziel + Saldo, heutige Sessions (Edit/Delete inline), Wochenstreifen mit Pace-Dots. **Live:** `session.*`, `project.created` → Fragment-Refresh; Timer tickt client-seitig.

### 5.2 Woche (`/woche`) — `studio-worktime-week.html`
Tagesbalken Mo–So (Ziel-Marker + Overflow-Segment), Zustände: erreicht/über (grün), unter (gelb), Wochenende (dezent), Frei (Kind-Farbe+Glyph), heute/laufend (cyan). **WOCHE GESAMT**-Banner (Mo–Fr-Ziel). **KENNZAHLEN:** Schnitt · Ziele X/5 · Saldo · Pace-Dots · auf-Kurs/Rückstand. Wochen-Navigation ‹ KW ›.

### 5.3 Historie (`/historie`) — `studio-worktime-calendar.html`
**Kern für den Import-Cleanup.** Wochen-Kalender (Default) mit zeit-positionierten Blöcken in Projektfarbe; **„ohne Projekt" fällt auf** (grau/gestrichelt, Warn-Tint, Banner „N ohne Projekt"). Monats-Ansicht umschaltbar. Datums-/Filter-Leiste (Alle / ohne Projekt / nach Projekt).
- **Bulk-Modus:** „Auswählen" → Checkboxen; Tag-Header wählt ganzen Tag; „ganze Woche"; Aktionsleiste „N ausgewählt → **Projekt zuweisen** (Fuzzy-Picker) / Löschen / Abbrechen".
- **Einzel-Edit:** Block-Klick → Popover (Projekt-Picker, Start/Stop, Tag, Notiz, Löschen).
- Mobil: Agenda-Liste pro Tag, Checkboxen, Sticky-Bulk-Leiste.
- **Live:** `session.*`, `project.*`.

### 5.4 Stats (`/stats`)
Range (Woche/Monat), Burndown (total vs Ziel vs erwartet, on-track), Streak/Best-Streak, Hits, Saldo, per-Tag-Aggregat. Diagramme als CSS/SVG (keine Highcharts-Abhängigkeit nötig; einfache Balken/Sparklines). **Live:** `session.*`, `settings.changed`, `dayoff.changed`.

### 5.5 Frei (`/frei`)
Day-Off-Liste/Kalender, 7 Kinds (Farbe+Glyph), Range hinzufügen (von–bis, Kind, Label, Teil-Tag-Target, skipWeekends), löschen; Feiertage (Bundesland) angezeigt; ICS-Token (Regenerate + URL kopieren). **Live:** `dayoff.changed`.

### 5.6 Export (`/export`)
Range + Projekt-Filter, Format CSV/JSON/MD, Vorschau-Fragment, Download. Per-Projekt Σh × Satz + Gesamt.

### 5.7 Wissen (`/wissen`) — `studio-docs-categories.html`
**Kategorie-spezifische Darstellung** (nicht Einheitsliste):
- **Daily** → chronologische Journal-Timeline (Monats-gruppiert, ● blau).
- **Projekt-Notizen** → gruppiert *unter* dem Projekt (Projektfarbe+Glyph-Header), ◆ grün.
- **Frei** → Karten-Grid (○ lila).
- **Agent/System** (memory/instruction/skill/plan) → abgesetzter, technischerer Bereich (eigene Badges, dichter), klar getrennt von menschlichen Notizen.
Oben: Such-Leiste (Volltext **+** semantisch), Tag-Filter-Chips, Kategorie-Tab-Strip (Scroll-Spy), „Neu". **Live:** `document.*`.

### 5.8 Dokument (`/wissen/{slug}`) — `studio-document-view.html`
**Eigene Lese-Seite.** Header (Kind-Badge, Titel, Projekt-Link, Datum, Tags, Bearbeiten/Overflow). Lese-Spalte (~70ch) + Meta-/ToC-Rail + **„Referenziert von"** (Backlinks). **Volles Markdown** (siehe §6). **Live:** `document.*` (eigenes Doc).

### 5.9 Editor (`/wissen/neu`, `/wissen/{slug}/bearbeiten`)
Titel + Body (Textarea, Monospace), Tags via Frontmatter (single source), Projekt-Zuordnung, Live-Vorschau optional (htmx). Speichern → Redirect auf Lese-Seite.

### 5.10 Projekte (`/projekte`)
Liste (Farbe+Glyph+Name, Status-Badge, Kennzahlen-Kurz), Filter nach Status, „Neu". (Erbt M4-Projektverwaltung.)

### 5.11 Projekt-Hub (`/projekte/{id}`) — `studio-project-hub.html`
**Sammelstelle.** Identität (Glyph+Name, Status, Satz, Bearbeiten/Pausieren), **Kennzahlen** (Stunden gesamt/Woche/Monat, Verdienst @ Satz, #Sessions, #Docs), Beschreibung (Markdown), **Projekt-Notizen**, **Letzte Sessions** (+ Mini-Chart, Link → Historie), **Bindings/Worktrees**, **Tags**. Desktop 2-spaltig, Mobile gestapelt. **Live:** `project.updated`, `session.*`, `document.*`.

### 5.12 Einstellungen (`/einstellungen`) — neu
Bundesland-Picker, Wochen-Ziele pro Wochentag, Standard-Tagesziel, ICS-Token, Theme-Präferenz (client-seitig). **Backend:** Settings-DTO um `weekdayTargetMin` erweitern (siehe §9).

---

## 6. Markdown — feature-complete (Web)

Aktuell rendert die WebUI nur CommonMark (+ Wikilinks). Ziel: **Parität zum TUI**.

- **Pipeline:** goldmark mit `extension.GFM` (Tabellen, Strikethrough, Tasklists, Autolink), `extension.Footnote`, definitionslisten optional, **Frontmatter** strippen (Tags), **Wikilinks** (bestehender Custom-Parser, valid → Link / broken → rot), **Callouts** (NOTE/TIP/WARNING/IMPORTANT/DANGER → Erb-Farben), **Syntax-Highlighting** via Chroma (server-seitig; theme-bewusst — entweder konstanter dunkler Code-Hintergrund in beiden Themes [wie Mockup] oder zwei Stylesheets).
- **Sanitize:** erweiterte bluemonday-Policy (erlaubt `class`, Code-Highlight-Spans, Tabellen-Elemente, Task-Checkboxen, relative hrefs).
- **ToC + Backlinks:** ToC aus Headings (client-seitig Scroll-Spy ok), Backlinks via bestehendem `/documents/{id}/backlinks`.
- Ein **gemeinsamer Renderer** für Doc-Ansicht, Projekt-Beschreibung, Editor-Vorschau (DRY).

---

## 7. Live-Updates (SSE)

Bestehend: `GET /api/v1/events` (per-User, Bearer **oder** Cookie). Browser: `<body hx-ext="sse" sse-connect="/api/v1/events">`, Fragmente mit `sse-swap`.

Event → Surface:
| Event | aktualisiert |
|---|---|
| `session.started/stopped/updated/deleted` | Heute, Woche, Historie, Stats, Projekt-Hub |
| `project.created/updated/deleted` | Heute(Picker), Historie(Picker), Projekte, Projekt-Hub |
| `document.created/updated/deleted` | Wissen, Dokument, Projekt-Hub |
| `dayoff.changed` | Frei, Woche, Stats |
| `settings.changed` | Stats, Einstellungen |

Live-Timer (Sekunden) tickt **client-seitig** (kleines Vanilla-JS), unabhängig vom Fragment-Refresh.

---

## 8. i18n

- **Schicht:** `T(ctx, "key", args…)` + `Tn` (Plural). Aufruf aus templ via Helper.
- **Katalog:** `de.toml` (primär, vollständig) + `en.toml` (Stub). Empfehlung Lib: `nicksnyder/go-i18n/v2` (TOML-Bundles, CLDR-Plural) — Endwahl im Infra-Slice; alternativ leichtgewichtige Go-Map-Lösung, falls Dependency unerwünscht.
- **Locale-Auflösung:** Cookie `flow_lang` → sonst `Accept-Language` → Default `de`.
- **Regel:** keine hartkodierten Anzeige-Strings in Templates. Test/Lint: alle Keys in `de` vorhanden; (optional) Heuristik gegen deutsche Literale in `.templ`.

---

## 9. Backend-Änderungen (minimal, additiv)

1. **Bulk-Reassign:** `POST /api/v1/sessions/reassign` `{ids:[…], projectId}` → Projekt vielen Sessions zuweisen (owner-scoped, validiert), publiziert `session.updated`. (Zeiten unverändert → keine Overlap-Probleme.) Usecase `BulkAssignProject`. Optional: Bulk-Delete.
2. **Settings-DTO:** `weekdayTargetMin` (derzeit `json:"-"`) im Settings-Wire-DTO exponieren (GET + POST), für den Wochentags-Ziel-Editor.
3. **Markdown-Renderer:** GFM/Footnote/Callouts/Chroma in den Web-Renderer (`internal/adapter/webui/markdown.go`/`wikilink.go`) — siehe §6.
4. **Pagination:** Listen-Endpunkte (`GET /documents`, `GET /sessions`, Suche, ggf. `GET /projects`) um `?limit=&offset=` + Gesamtanzahl/Next-Hinweis erweitern (oder Cursor); Usecases/Stores reichen das durch. Default-Seitengröße pro Liste. (Kalender/Historie paginiert über Zeitfenster Woche/Monat; klassische Pagination für Listen/Agenda/Suchergebnisse.)
5. (Alles andere nutzt bestehende Endpunkte: Range-Listing `GET /sessions?since&until`, PATCH/DELETE, documents, projects, dayoffs, export.)

---

## 10. Infrastruktur / Docker

1. **Assets lokal bündeln:** `htmx.min.js`, `htmx-ext-sse.min.js`, Fonts (woff2) nach `web/static/vendor/` bzw. `web/static/fonts/`, via `go:embed` ins Binary, Serve unter `/static/`. `Base` referenziert nur lokale Pfade. **Kein unpkg, kein Google-Fonts** zur Laufzeit.
2. **CSS im Build:** `deploy/podman/Dockerfile.server` bekommt einen Schritt, der die **Tailwind-Standalone-CLI (gepinnte Version)** holt und `tailwindcss -i web/tailwind.css -o internal/adapter/webui/static/app.css --minify` *vor* `go build` ausführt → `go:embed` nimmt frisches CSS. Beseitigt den „manuell committen"-Stolperstein.
3. **CI-Guard:** Job/Target `verify-css`, der prüft, dass das committete `app.css` einem frischen Build entspricht (kein Drift im Dev). `make web` bleibt für lokal.
4. **CSP:** Da alles lokal ist, strikte Content-Security-Policy (nur `self`) setzen — Sicherheits-Plus.
5. **Smoke:** Container startet, alle Routen liefern 200 (auth), `/static/app.css` + Fonts laden lokal, SSE verbindet.

---

## 11. Architektur & Datei-Organisation

Hexagonal beibehalten (siehe `CLAUDE-hexagonal-plan.md`); „keine Monolithen" — fokussierte Dateien:

```
internal/adapter/webui/
  base.templ                 # HTML-Hülle, Head, Theme-Boot
  components/                # wiederverwendbare Komponenten (je Datei)
    nav.templ subnav.templ card.templ badge.templ chip.templ
    button.templ progressbar.templ pacedots.templ statile.templ
    fuzzypicker.templ datepicker.templ emptystate.templ toast.templ
    projectchip.templ sessionrow.templ sessionblock.templ dayrow.templ
    kennzahlen.templ docrow.templ doccard.templ markdownprose.templ
    toc.templ backlinks.templ themetoggle.templ ...
  pages/                     # eine Datei pro Surface (Page + Fragment)
    heute.templ woche.templ historie.templ stats.templ frei.templ
    export.templ wissen.templ document.templ editor.templ
    projekte.templ projekthub.templ einstellungen.templ
  markdown.go wikilink.go    # gemeinsamer Renderer (erweitert)
  i18n/                      # T-Helper + de.toml/en.toml (oder internal/i18n)
  static/ (app.css, vendor/, fonts/)  # go:embed
internal/adapter/httpserver/
  webui_*.go                 # Handler pro Surface (Viewmodel bauen → Page/Fragment)
web/tailwind.css             # CSS-Quelle (+ Token-Layer)
```

Handler-Muster bleibt: Handler baut typisiertes Viewmodel → `pages.XPage(vm).Render(ctx, w)`; htmx-Refresh → `pages.XFragment(vm)`.

---

## 12. Querschnitt: Zustände, Dialoge, Bestätigungen, Pagination

**Zustände** pro Surface definiert: **leer** (z.B. „Noch keine Sessions" / „Kompendium leer — Neu anlegen"), **Fehler** (klare deutsche Meldung in UI-Stimme, was tun), **Lade** (htmx-Indikator). Spezialfall Historie: prominenter Hinweis bei vielen „ohne Projekt". Embed-Status (ok/pending/failed) im Doc sichtbar + Reembed-Aktion (bestehend).

**Dialoge & Bestätigungen:** siehe §3.6 — alle modalen Inhalte und Confirms über die wiederverwendbaren `Dialog`/`ConfirmDialog`-Komponenten; **keine Browser-Popups**; **jede destruktive Aktion erst nach Bestätigung**.

**Pagination (von Anfang an):** alle potenziell langen Listen — Dokumente, Historie-Agenda/Sessions, Suchergebnisse (und Projekte bei Bedarf) — paginieren serverseitig via `Pagination`-Komponente + `?limit&offset`; Default-Seitengrößen pro Liste; htmx-getriebenes „Zurück/Weiter" bzw. „Mehr laden". Der Kalender paginiert über das Zeitfenster.

---

## 13. Accessibility & Responsive

- Sichtbarer Keyboard-Fokus, semantisches HTML (Landmarks, `role=timer/progressbar`, `aria-*`), `prefers-reduced-motion` respektiert (Timer tickt weiter — Information, keine Deko).
- **AA-Kontrast in beiden Themes** (inkl. gesättigter Flächen, Code-Token).
- Responsive 360→1440: Desktop = Sidebar + dichte Mehr-Spalten; Mobile = Bottom-Tab + gestapelt (echtes Mobile-Layout, nicht gequetscht).

---

## 14. Testing & Done-Gate

- **Unit:** templ-Komponenten-Render-Tests; Handler-Tests (`httptest`); Markdown-Renderer-Feature-Tests (Tabelle/Tasklist/Callout/Code/Wikilink); i18n-Key-Vollständigkeit; Bulk-Reassign-Usecase-Test.
- **`make ci`** grün (lint, verify-generate, cover ≥ Gate, build) **plus** neuer `verify-css`-Guard. Integrations-Implementierer führen **`make ci` (Lint!)**, nicht nur `go test` (Lektion: QF1002 rutschte mal durch).
- **Live-Done-Gate** gegen echtes Postgres+Dex (Dev-Stack): jede Route 200, SSE-Live (Session start/stop spiegelt sich), Bulk-Reassign wirkt, Markdown-Parität sichtbar, Dark/Light + Mobile geprüft, Docker-Container offline lauffähig (keine externen Requests).
- Mockups in `assets/` sind die visuelle Referenz.
- **Dialoge/Confirm:** Lint/Test, dass kein `window.alert/confirm/prompt` im Code vorkommt; jede destruktive Aktion geht durch `ConfirmDialog`.
- **Pagination:** Endpunkt-Tests für `limit/offset` + Gesamtanzahl; `Pagination`-Komponente rendert „Zurück/Weiter" an den Grenzen korrekt (erste/letzte Seite).

---

## 15. Implementierungs-Slicing (für writing-plans)

Jeder Slice = eigener Plan, **mit expliziter Main-Wiring-/Verifikations-Task am Ende** (main.go/Routen verdrahtet + curl-Smoke je Route) — Lektion [[feedback_plan_main_wiring_task]].

- **Slice 0 — Fundament:** Design-Tokens (Dark/Light) + Tailwind-Build im Docker + Asset-Vendoring (htmx, Fonts) + i18n-Schicht + `Base`/`AppShell`/Nav + Komponenten-Bibliothek-Grundstock (inkl. **`Dialog`/`ConfirmDialog`/`Pagination`**) + ThemeToggle. (Keine neuen Features; trägt alles Weitere.) Pagination-**Backend** kommt je Slice dazu (Sessions in Slice 1, Dokumente/Suche in Slice 2).
- **Slice 1 — Worktime:** Heute (live) + Woche + Historie (Kalender + Bulk-Reassign inkl. Backend-Endpoint).
- **Slice 2 — Wissen:** kategorie-spezifische Liste + Dokument-Lese-Seite (Markdown-Parität) + Editor.
- **Slice 3 — Projekte:** Liste + Projekt-Hub.
- **Slice 4 — Rest:** Stats + Frei + Export + Einstellungen (inkl. Settings-DTO).
- **Slice 5 — Politur & Gate:** A11y-/Mobile-Pass, leere/Fehler-Zustände, i18n-Vollständigkeit, CSP, Docker-Offline-Done-Gate.

Ausführung subagent-getrieben; nach jedem Slice Holistic-Review (Opus) + Dogfood-Gate.

---

## 16. Offene Punkte / Risiken

- **Chroma-Theme bei Dark/Light:** konstanter dunkler Code-BG (einfach, wie Mockup) vs. zwei Stylesheets (sauberer). → Plan-Detail Slice 2.
- **i18n-Lib:** go-i18n/v2 vs. leichte Map-Lösung → Slice 0.
- **Top-Nav-Gruppierung** (was primär, was im Menü) → Slice 0 Feinschliff.
- **Tailwind-CLI im Docker-Build:** Standalone-Binary-Download im Build-Image (Pinning, arch amd64/arm64) verifizieren.
- **Bestehende WebUI** wird Surface-für-Surface ersetzt; während der Slices koexistiert Alt/Neu — Routen sauber umstellen.

---

## 17. Referenzen
- Mockups (Studio-Set): `docs/superpowers/specs/assets/2026-06-23-webui/` — `direction-b-studio` (Heute/Sprache), `studio-worktime-week`, `studio-worktime-calendar`, `studio-project-hub`, `studio-docs-categories`, `studio-document-view`.
- Aktueller Code: `internal/adapter/webui/`, `internal/adapter/httpserver/`, `web/tailwind.css`, `deploy/podman/Dockerfile.server`.
- TUI-Design-Erbe: `internal/tui/theme`, `internal/tui/ui/glyphs`, `internal/tui/kindcolor`.
