# Kristall K5 — Politur & Gate — Design Spec

> Datum: 2026-07-03 · Status: **APPROVED** (brainstormed + Live-Audit, Soenne „voller Scope") · Branch: `cockpit-story` · Base: `2341646`
> Letzter Slice des Kristall-Redesign-Programms. Umbrella: [[specs/2026-07-02-kristall-redesign-design]] §9 (Politur & Gate), §13 (offene K5-Detail-Entscheidungen).
> Konsumiert K1–K4 (gelandet `d2b513a..2341646`). Danach: Merge `cockpit-story → rebuild`.

---

## 1. Kontext

K1–K4 haben das Kristall-Design app-weit ausgerollt. K5 macht **keinen neuen Sweep**, sondern schließt das Programm: die restlichen §9-Achsen (Light Seite-für-Seite, Mobile-Reflow, A11y) verifizieren und die dabei gefundenen Lücken schließen, die zwei §13-K5-Entscheidungen umsetzen, dann Gesamt-Dogfood + Merge.

Statt zu raten wurde ein **Live-Audit** gegen den Dev-Stack gefahren: 48-Shot-Matrix (16 Seiten × light-1440 / light-390 / dark-390; dark-1440 = bekannter Design-Baseline). **Befund: das Redesign trägt in Light UND Mobile durchweg** — Glas, Kontrast und Stapelung stimmen fast überall. Die Funde unten sind der vollständige Rest, nichts Grundlegendes.

## 2. Scope (Soenne: voller Scope, 2026-07-03)

### Mobile-Reflow-Fixes

- **M1 — Aktivitäts-/Puls-Zeilen-Reflow.** Bei ≤ ~420px brechen die Feed-Zeilen hässlich: Verbphrasen („legte an", „stoppte den Timer") wickeln auf 2–3 Zeilen, die **Node-Pill läuft rechts aus der Karte**, der Zeitstempel wird rausgedrängt. Geteilte Zeilen-Komponente (Home-Feed + Cockpit-Puls) → **ein** Fix wirkt an beiden Stellen. Ziel: unter `sm` reflowt die Zeile in einen Block-Stack (Actor+Verb zusammen, Node-Pill darf umbrechen und bleibt in der Karte, Zeitstempel unter/rechts ohne Overflow). `tabular-nums` für die Zeit bleibt.
- **M2 — Cockpit-Tab-Strip Overflow.** Übersicht · Worktime · Wissen · Struktur · Bindings passen bei 390px nicht; „Struktur" abgeschnitten, „Bindings" **unerreichbar**, kein Scroll/Wrap. Fix: der Tab-Strip wird unter `lg` **horizontal scrollbar** (`overflow-x-auto`, Scroll-Snap optional, Count-Chips bleiben), kein Tab fällt raus. Kanonische htmx-Targets (`#cockpit-main`) unverändert.

### Light-Politur

- **L1 — `.field`-Rand im Light zu blass.** `.field` nutzt `border: 1px solid rgb(var(--line))` (css:359); auf hellem Glas verschwindet der Rand, leere Felder (Einstellungen-Wochentage, Editor, Node-Formular) lesen als kaum vorhanden. Fix: im Light-Theme kräftigerer Feld-Rand (eigener Token/Override, nur Light — Dark bleibt wie gehabt). Focus-Ring (css:360) unverändert.
- **L2 — letztes Sidebar-Item angeschnitten.** „Einstellungen" (unterster Nav-Eintrag) sitzt am Karten-Rundrand und wird leicht beschnitten. Fix: Bottom-Padding/Safe-Area, damit das letzte Item den Radius (und die `mask-image`-Fade) frei hat.

### §13-K5-Entscheidungen

- **H1 — Zwei-CTA-Hierarchie.** Auf einem Knoten-Cockpit sind **zwei gleich-schwere grüne Start-CTAs** sichtbar: das globale Sidebar-Timer-Widget („Timer starten") und der objektgebundene Rail-„Start". §13 markiert das als K5-**Hierarchie**-Frage (kein IA-Verstoß nach §3 — global vs. objektgebunden bleibt korrekt). Entscheidung: das **globale Widget demoten**, solange ein Knoten-Cockpit die Rail-Primäraktion zeigt (dezenter Zustand statt voller Gradient-CTA), sodass der Rail-„Start" die eine visuelle Primäraktion der Seite ist. Kein Mechanismus-Wechsel, reine Gewichtung.
- **A2 — Auth-Seiten ohne SSE.** Die K4-Auth/Fehler-Seiten (`AuthPage`) rendern über `components.Base`, das `sse-connect="/api/v1/events"` emittiert; unauthentifiziert 401t der Endpoint und EventSource retryt still (K4-Flag). Fix: eine **No-SSE-Base**-Variante für die Auth-Seiten (kein Live-Sync nötig, bevor man eingeloggt ist).

### A11y-Pass (verifizieren + Lücken)

Der globale `@media (prefers-reduced-motion: reduce)`-Block (css:240) killt bereits alle Animationen/Transitions — `breathe`/`rise`/Pulse/Glow **sind abgedeckt**; K5 **bestätigt** das nur (Toggle-Dogfood). Zu schließen:
- `title`-Tooltip auf fade-truncateten Nav-Labels (voller Name bei Kürzung).
- `aria-label` auf Icon-only-Buttons (× Löschen, Timer-Controls). Theme-Toggle hat bereits `aria-pressed` + `title` — als Muster übernehmen.

### Gate (immer)

- **Gesamt-Dogfood** durch Soenne (Light-Toggle + Mobile-Reflow app-weit, Kontrast-Check).
- **Merge-Vorbereitung** `cockpit-story → rebuild` (whole-branch Opus-Review, `make ci` grün, dann Merge).

## 3. Nicht-Ziele / bewusst nicht fixen

- Wissen-Kategorie-Karten wirken gestapelt luftig (leere `min-height`) — rein ästhetisch, geringer Wert → **nicht** in K5.
- Keine neuen Fachfeatures; TUI/CLI/MCP unverändert (WebUI-Programm).
- Kein Multi-Tenant-Feature-Ausbau (§0 der Umbrella bleibt bindend, aber kein neuer Umfang).

## 4. Constraints (aus Umbrella §0 + §10)

- **Multi-tenant, owner-scoped** bleibt bindend — kein Fund/Fix führt un-keyed globalen State ein.
- Guards `verify-css` + `verify-no-popups` scharf; kein `make fmt`; i18n de+en Parität (neue `aria`/`title`-Strings via i18n, wenn nutzersichtbar).
- Jede CSS-Änderung nur additiv/Override; `make generate` committen; `make web` (app.css) committen.
- Gate `make ci` grün (75 %, `*_templ.go` ausgeschlossen, output-asserting Tests).

## 5. Prozess

Ein Just-in-time-**Plan** (K5 = ein Plan, nicht gesliced) mit expliziter **Main-/Wiring-Verifikations-Task** am Ende → subagent-driven → per-Task-Review → `make ci` → Live-Gate vs. Dev-Stack (Re-Audit der gefixten Flächen mit demselben Screenshot-Harness) → Opus-Whole-Branch-Review → Soenne-Dogfood → Merge.

## 6. Offene Detail-Entscheidungen (im Plan zu fixieren)

- M1: exakter Breakpoint (`sm` 640 vs. eigener) + Stack-Anordnung (Zeitstempel unter Actor-Zeile vs. rechts-oben-absolut).
- M2: Scroll-Snap ja/nein + ob der aktive Tab beim Laden in Sicht gescrollt wird.
- L1: Light-Feld-Rand als neuer Token (`--field-border`) vs. `[data-theme=light] .field { border-color: … }` Override.
- H1: demoteter Widget-Zustand — komplett verstecken auf Knoten-Cockpit vs. zu kompaktem „läuft hier"-Chip reduzieren (Timer-Zustand bleibt sichtbar, nur die CTA-Schwere sinkt).
