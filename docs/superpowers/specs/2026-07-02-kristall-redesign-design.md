# flow — Kristall-Redesign (komplette App) + IA-Konsolidierung — Design Spec

> Datum: 2026-07-02 · Status: **DRAFT** (brainstormed, Design von Soenne sektionsweise approved) · Branch: `cockpit-story`.
> Umbrella für **5 Slices K1–K5** (jeder = eigener Plan + subagent-driven + Review + Dogfood).
> **Supersedes** die Slices 4+5 der Cockpit-Story-Spec ([[specs/2026-07-01-cockpit-story-rework-design]] §7) — deren Inhalte gehen hier auf. Slices 1–3 (Work/Privat, Logos, Aktivität-Ziel — gelandet bis `814b287`) werden konsumiert.
> Design-Referenz: `docs/superpowers/specs/assets/2026-07-01-cockpit-story/direction-b-APPROVED.html` (approved Mockup; Werte daraus sind normativ).

---

## 0. Grundsätze (binden jede Entscheidung in K1–K5)

- **flow ist multi-tenant** (M1-Spec §Kontext; `AGENTS.md` §Grundsätze). Ein-User-Dogfood ist Deployment-Zustand, kein Design-Parameter: jeder Datenzugriff owner-scoped, Limits pro Tenant, keine globalen Caches ohne Tenant-Schlüssel, „ist nur ein User"-Begründungen sind unzulässig — auch in Reviews und Trade-offs.
- **Menschen UND AI-Agents** sind gleichberechtigte Akteure (Kreis- vs. Hexagon-Avatar, AGENT-Tag, MCP).
- Kristall-Design-Sprache: Twilight-Gradient + Low-Poly-Facets, Glas-Karten (`backdrop-filter: blur(16px)`), Form-Codierung ● Engagement / ◆ Vorhaben / ⬡ Repo, `tabular-nums` für Uhren/Dauern, keine Emoji-Piktogramme, Motion hinter `prefers-reduced-motion`.

## 1. Kontext & Ziel

Slices 1–3 lieferten Daten (Work/Privat-Split, Logos/Icons, Aktivitäts-Ziele) — sichtbar wurde wenig („dem entschiedenen Design nicht näher gekommen"). Dieses Programm liefert das approved Design **ganzheitlich auf die komplette App** und räumt dabei die IA auf („keine Items doppelt", z. B. 3× Timer-Start/Stop). Rückgrat bleibt das **lebendige Projekt-Zuhause**; dazu kommt: **jede Seite** steht auf der Kristall-Sprache, und die Kinds fühlen sich lebendig an.

## 2. Entschiedene Fragen (Soenne, 2026-07-02)

| # | Frage | Entscheidung |
|---|---|---|
| 1 | Scope | **Komplette App** (nicht nur Cockpit+Sidebar) |
| 2 | Timer-Kanonik | **Shell-Widget (global) + Cockpit-Rail (objektgebunden)**; Home/Heute verlieren ihre Formulare |
| 3 | Theme-Default | **Dark = Default** (Kristall-Identität), Light abgeleitet, Toggle bleibt persistiert |
| 4 | Logo-Darstellung | **Auto nach Seitenverhältnis**: ≈ quadratisch → Hexagon-Crop; breit/hoch → unbeschnitten (contain) auf Glas-Kachel |
| 5 | Quick-Actions | In der **Rail** (eigene Karte, per approved Mockup) |
| 6 | Sidebar-Baum | **Form-Punkte** (● ◆ ⬡, Knotenfarbe+Glow) — Logos/Icons bleiben Cockpit + Projektliste, der Baum bleibt ruhig |
| 7 | Cockpit-Default-Tab | **Übersicht** (aus Vorgänger-Spec übernommen) |
| 8 | Slicing | **Ansatz 1**: Design-System-Hebel + 5 Slices K1–K5 |

## 3. IA-Konsolidierung („keine Items doppelt")

**Regel:** Eine Aktion hat genau **einen Mechanismus**. Globale Aktionen bekommen ein globales Zuhause in der Shell; objektgebundene Einstiege öffnen denselben Mechanismus mit Vorbelegung — keine parallel gepflegten Formulare.

1. **Timer** → neues **Timer-Widget** in der Shell. Desktop: Karte in der Sidebar über dem Baum. Mobil: kompakter Chip in der Top-Bar (Uhr + Node; Tap öffnet Sheet mit Stop/Wechseln/Start). Zustände: *idle* (Start + Node-Auswahl inkl. „Neues Projekt"-Quick-Create), *running* (Node-Pill, Live-Uhr via `[data-timer]`, Stop, Wechseln), *running-unbooked* (Stop verlangt Node-Wahl — das bisherige Home/Heute-Muster wandert hierhin). Eigene Fragment-Endpoints (`GET /ui/timer`, `POST /ui/timer/start|stop|switch`), SSE-Reload auf `session.*`. Die Handler-Logik zieht aus `handleWebStart/Stop` + `handleHomeStart/Stop` um (K1 baut das Widget, K3 entfernt die alten Formulare + Handler).
2. **Session anlegen/bearbeiten** → **eine** `SessionDialog`-Komponente (in-design `<dialog>`, Add- und Edit-Modus): ersetzt die zwei Inline-Formulare (Heute-Seite, Cockpit-Worktime-Tab). Entsteht in K2 (erster Konsument Cockpit), K3 migriert Heute/Woche. Kontext-Prefill: Heute → Datum; Cockpit → Node.
3. **Neues Wissen**: bereits ein Mechanismus (Editor mit `?node=`-Prefill); Einstiege (Wissen-Seite, Cockpit-Tab, Rail-Quick-Action) sind Links — nur Beschriftung vereinheitlichen.
4. **Projekt anlegen**: volles Formular `/nodes/new` bleibt der Mechanismus; das „Neues Projekt"-Schnellfeld existiert nur noch **im** Timer-Widget-Picker.

**Seiten-Rollen danach:** **Heute** = reiner Tages-Ledger (Blöcke, `SessionDialog`, Tages-Summen — keine Session-Steuerung). **Home** = reines Dashboard (Saldo/Burndown, Puls, Zuletzt-Wissen — Widget ist daneben immer sichtbar).

## 4. Containment-Regel + lebendige Kinds

**Inhalts-Vererbung:** Ein Cockpit zeigt die Inhalte seines **Subtrees**: Engagement ⊇ Vorhaben ⊇ Repo; Repo zeigt nur eigene Inhalte. Einheitlich für alle Inhaltsarten:

| Fläche | Engagement / Vorhaben | Repo |
|---|---|---|
| Zeit-Rollup | Subtree (macht `NodeStats` bereits) | eigener (Subtree == self) |
| Puls/Aktivität | Subtree-gefiltert (NodeRef ∈ Subtree) | eigene |
| Wissen (Karte + Tab) | Subtree-Docs, Umschalter „nur dieser Knoten" | nur eigene |
| Worktime-Tab | Subtree-Sessions **mit Node-Pill je Zeile** | eigene |
| Struktur-Tab | direkte Kinder (wie bisher) | (leer/Branches) |

**Kind-optimierte Übersicht** (gemeinsame Karten-Grammatik, kind-spezifische Komposition):
- **Engagement & Vorhaben:** zusätzlich **Zusammensetzungs-Karte** („Woraus besteht das?"): direkte Kinder mit Anteils-Balken, **Live-Punkt** am Kind mit laufendem Timer, „zuletzt aktiv vor X". Fließt-nach-oben-Kette kompakt. Identitäts-Karte zeigt eigenen Stundensatz + #Unterknoten.
- **Repo:** Kette nach oben prominent (Mockup-Fall), `upstream git` in der Identitäts-Karte (id-meta), geerbter Satz.

## 5. K1 — Fundament (App-weit sichtbar)

- **Tokens** (`web/tailwind.css`): Kristall-**Dark wird `:root`-Default**, Light zieht nach `[data-theme=light]` als Ableitung (bestehende Light-Palette als Basis, Glas = weiß-transluzent, helle Gradients). Dark-Werte normativ aus dem Mockup: canvas `rgb(28 24 56)`, surface `rgb(40 33 64)`, sunken `rgb(22 18 44)`, ink `rgb(236 233 245)`, body `rgb(185 178 207)`, muted `rgb(154 146 181)`, faint `rgb(122 115 151)`; Akzente blue `#7aa2f7`, cyan `#67e8f9`, green `#6ee7b7`, purple `#c4b5fd`, magenta `#f76ea8`, yellow `#e0af68`, orange `#ff9e64`; **neue Tokens** `--glass rgba(255,255,255,.055)`, `--border rgba(255,255,255,.10)`, `--border-soft rgba(255,255,255,.06)`, Kristall-Shadow. ThemeToggle-Persistenz bleibt; gespeicherte User-Wahl wird respektiert, nur der Default dreht.
- **Canvas:** Twilight-Radial-Gradients + `linear-gradient(150deg,…)` fixed auf `body` + **Facets-SVG** als fixe, `aria-hidden` Hintergrund-Ebene in `components.Base` (Polygone/Opacities aus dem Mockup). Light-Variante: helle Gradients, Facets stark reduziert.
- **Komponenten-Restyle** (`internal/adapter/webui/components`): Card→Glas (blur 16, `--border`, rounded-20, Kristall-Shadow), StatTile→`rtile`-Optik (Eyebrow-K, große tnum-Werte, Akzent-Unterkante, `big`-Gradient-Text, `earn`-Gelb), Badge/Chip→Pill-Optik, Primär-Button→Gradient-CTA (`#6ee7b7→#67e8f9`, dunkle Schrift), Sekundär→Glas-Button, Tabs/SubNav→Pill-Leiste mit Count-Chips, Dialog, EmptyState, Pagination. Alle Feature-Seiten erben automatisch.
- **Sidebar-Rework:** Brand + SiteNav in Kristall; **NavTree**: Form-Punkte je Kind (Kreis/45°-Raute/Hexagon-`clip-path`, Knotenfarbe via `--nc` + Glow), Label mit `mask-image`-Fade (aktiver Node voller Name + `title`-Tooltip), **Stunden-Badges** (Subtree-Stunden je sichtbarem Knoten; EIN owner-scoped Aggregations-Pass pro Nav-Fragment — Sessions+Nodes einmal laden, im Speicher rollen; Format `41h`, unter 1 h leer), Active-Glow + linke Akzent-Linie, `children`-Bordüren.
- **Timer-Widget** (§3.1) in der Sidebar über dem Baum + Mobile-Chip.
- **`/ui`-Styleguide** aktualisiert = Abnahmefläche für K1 (plus Klick durch alle Seiten: alles steht auf neuem Grund).

## 6. K2 — Cockpit Direction B (kind-differenziert)

Layout: `#cockpit-rail` (links, sticky, persistent) + rechts Pill-**Tabs** (Übersicht · Worktime · Wissen · Struktur · Bindings, mit Count-Chips) + `#cockpit-main`. Kanonische htmx-Regel bleibt: alles, was die Tab-Fläche neu rendert, targetet `#cockpit-main`; Rail reloadet auf `session.*`/`node.*`-SSE separat; Full-Page-Forms `hx-boost="false"`. Responsive: unter `lg` stapelt die Rail über die Tabs.

**Rail:**
- **Identitäts-Karte:** Hero 110 px — Logo per **Auto-Crop-Regel** (§2.4): `node_logos` bekommt `width`/`height` (Migration; beim Upload via `image.DecodeConfig` befüllt — WebP über `golang.org/x/image/webp`; Bestandszeilen werden beim ersten Serve lazy vermessen und zurückgeschrieben); Seitenverhältnis ∈ [0.8, 1.25] → Hexagon-Crop mit Ring, sonst **contain** auf Glas-Kachel mit Ring. Ohne Logo: Icon getönt im Hex, sonst Glyph. Dazu Kind-Badge-Ecke, Name, Kind-/Status-Badges, Beschreibung, id-meta (kind-abhängig §4), Beiträger (aus Activity-Actors des Subtrees).
- **Timer-Karte:** die 5 Zustände aus Slice 6 in Mockup-Optik — `elsewhere`-Banner (läuft woanders: Node, Live-Uhr, „Wechseln ›"), idle-Hero (`big-start`-CTA, „heute hier: Xh · zählt als Work/Privat"), running (Uhr + Stop). Wiederverwendet `NodeTimer`-Logik + Start/Stop/Switch-Handler.
- **Quick-Actions-Karte:** Nachbuchen (→`SessionDialog`, Node vorbelegt) · Neues Wissen (→Editor-Link `?node=`) · Struktur bearbeiten (→Struktur-Tab).

**Übersicht-Feed (Default-Tab):** 4 Rollup-Kacheln (Subtree Σ als Gradient-Hero · Woche mit Vorwochen-Delta · Monat · Verdienst = Work × geerbte Rate) → `grid2`: **Work/Privat-Karte** (Wochen-Split-Balken aus Slice-1-Daten, Note „Work zählt aufs Soll · Privat wird nur getrackt", Totals Work-Monat/Verdienst) + **Zusammensetzung** (Eng/Vorhaben) *oder* **Kette** (Repo) → **Puls-Karte** (LIVE-Indikator, Subtree-Aktivität, Kreis-/Hexagon-Avatare, AGENT-Tag, Ziel-Pills aus Slice 3, relative Zeit) → **Zuletzt-Wissen** (Subtree-Top-3, „alle N ›" → Wissen-Tab).

**Verhalten in K2 gefoldet:** Stop-Picker-Fix (#1 alt: Buchungs-`<select>` filtert auf `IsBookable` statt `== KindEngagement`) + **Session-Edit/Delete im Worktime-Tab** via `SessionDialog` (#6 alt). Worktime-Tab zeigt Subtree-Sessions mit Node-Pill (Eng/Vorhaben) bzw. eigene (Repo).

**Backend-Zuwachs K2 (kompositorisch, keine neuen Ports):** Subtree-Docs-Query (Subtree-IDs + Docs-Filter), Subtree-Aktivitäts-Filter (letzte N owner-Entries, NodeRef ∈ Subtree), Vorwochen-Delta (zweiter `RangeStats`-/Rollup-Aufruf), Ancestors-Ketten-Werte (`NodeStats` je Kettenglied), Zusammensetzungs-Daten (NodeStats je direktem Kind + laufende Session + letzte Aktivität je Kind), Logo-Maße (Migration + Decode).

## 7. K3 — Sweep Worktime + IA-Enforcement

- **Home** neu komponiert: Saldo-Hero + Burndown, Puls (Ziel-Pills), Zuletzt-Wissen — **ohne Timer-Block**; Glas-Karten.
- **Heute** = Ledger: Tagesnavigation, Session-Blöcke, `SessionDialog` (Add/Edit — die alten Inline-Formulare fallen), Tages-Summen; **Start/Stop-Form + Handler entfernt** (Widget übernimmt; tote i18n-Keys mit raus).
- **Woche / Historie / Stats / Frei / Export:** Kristall-Feinschliff auf den neuen Komponenten (Tabellen→Glas, Kalender-Optik, Kacheln); funktional unverändert (Historie-Bulk bleibt).

## 8. K4 — Sweep Wissen & Verwaltung

- **Wissen** (Kompendium-Look auf Glas, Chips/Filter), **Dokument** (Prose auf Glas, TOC), **Editor**, **Projektliste** (Baum-Rows in Glas + Logos), **Node-Formular** (Kristall-Formsprache), **Einstellungen**, **Login** (erster Kristall-Moment).
- **Wissen-Rollup** komplettiert §4 im Wissen-Tab: Subtree-Default für Eng/Vorhaben + „nur dieser Knoten"-Umschalter (Query-Param, kein Server-State), Repo eigene.

## 9. K5 — Politur & Gate

Light-Theme Seite für Seite gegenchecken (`/ui` + Toggle-Dogfood, Kontrast-Check), Mobile-Reflow (Rail-Stapelung, Widget-Chip, Bottom-Tabs), A11y (`prefers-reduced-motion` für Pulse/Glow, `title` bei Fade-Truncation, aria-Labels), Gesamt-Dogfood durch Soenne, Merge-Vorbereitung `cockpit-story → rebuild`.

## 10. Slicing, Reihenfolge, Prozess

K1 → K2 → K3 → K4 → K5. Abhängigkeiten: K2 braucht K1 (Komponenten/Tokens/Widget); K3 braucht K2 (`SessionDialog`); K4 nach K3 (gleiches Muster, Wissen-Rollup konsumiert Subtree-Query aus K2); K5 zuletzt. Jeder Slice: eigener Just-in-time-Plan (mit Main-Wiring-Task) → subagent-driven → per-Task-Review → `make ci` (Gate 75 %, `*_templ.go` ausgeschlossen, output-asserting Tests) → Live-Gate vs. Dev-Stack → Opus-Whole-Branch-Review → Soenne-Dogfood. Guards (`verify-css`, `verify-no-popups`) bleiben scharf; kein `make fmt`; i18n de+en parity.

## 11. Nicht-Ziele

- TUI / CLI / MCP unverändert (WebUI-Programm; TUI-Sem-Migration bleibt separat geplant).
- Keine globale Suche (die Mockup-Suchleiste ist Deko; eigenes Feature, falls gewünscht).
- Keine neuen Fachfeatures über die IA-Konsolidierung hinaus; Mermaid (M1-S7) und Kontext-Tab (M1-S8) separat.
- Kein Multi-Tenant-*Feature*-Ausbau (Sharing etc., M2/M3) — aber alle Entscheidungen halten multi-tenant (§0).

## 12. Risiken

1. Dark-Default-Umstellung kann Light-Kontrast-Regressionen erzeugen → K5 prüft systematisch; Toggle-Persistenz respektiert Bestandswahl.
2. Zwischen K1 und K4 tragen alte Seitenlayouts bereits neue Tokens — bewusster, konsistenter Mischzustand (dokumentiert, dogfoodbar).
3. Stunden-Badges: ein owner-scoped Aggregations-Pass pro Nav-Fragment — hält pro Tenant (O(Sessions des Owners)); wächst ein Tenant stark, ist ein per-Tenant-Cache der nächste Schritt (im Plan als Follow-up notiert, nicht vorgebaut).
4. Neue Dependency `golang.org/x/image` (WebP-Maße) — klein, Standard-Erweiterung.
5. `session.*`-SSE triggert künftig Widget + Seite — Doppel-Reloads sind idempotente Fragment-GETs (htmx), aber im K1-Live-Gate explizit prüfen.

## 13. Offene Detail-Entscheidungen (je Slice-Plan zu fixieren)

- K1: exakte Light-Ableitungswerte (am `/ui` iterieren); Mobile-Widget-Interaktion (Sheet vs. `<details>`); Stunden-Badge-Schwelle.
- K2: Zusammensetzungs-Karte Sortierung (Zeit desc vs. Baum-Reihenfolge); Puls-Länge (8–10); Beiträger-Ermittlung (Actors der letzten N Tage).
- K3: ~~Ledger-Optik Heute (Blöcke vs. Timeline)~~ → **ENTSCHIEDEN (Soenne, 2026-07-03): Blöcke/Cards** (Kristall-Glas-Karten, gleiches Karten-Vokabular wie Rail/Mockup; Klick → `SessionDialog`; Timeline verworfen, dupliziert Historie-Metapher). Ferner gefixt: **K3 = ein Plan** (nicht gesliced); **Zwei-CTA-Dogfood-Fund** (Sidebar-Widget vs. Rail-„Start" auf Knoten-Cockpit) → **K5 Politur** (kein IA-Verstoß nach §3 — global vs. objektgebunden —, sondern Gewichts-/Hierarchie-Frage; im K5-Kontrast/Hierarchie-Pass demoten/restylen).
- K4: Login-Seiten-Gestaltung (Dex-Redirect-Flow zeigt wenig eigene UI — Umfang klären).
