# flow — Lesesaal: WebUI-Neuentwurf (content-first) — Design Spec

> Datum: 2026-07-04 · Status: **APPROVED** (Soenne hat Mockup v2.4 abgenommen: „Sehr gut" / „Super") · Branch: `cockpit-story`.
> **Normative Referenz:** `docs/superpowers/specs/assets/2026-07-03-lesesaal/lesesaal.html` (v2.4, self-contained, echte Prod-Inhalte). Werte daraus sind verbindlich; dieses Dokument benennt die Regeln dahinter.
> **Supersedes** die Design-Schicht der Kristall-Spec ([[specs/2026-07-02-kristall-redesign-design]]) vollständig. Die fachliche Substanz von K1–K5 (IA-Konsolidierung, SessionDialog, Rollups, node_logos, Aktivitäts-Ziele, Wissen-Rollup) bleibt Code-Basis.

---

## 1. Anlass & Auftrag

Nach Abschluss von K5 hat Soenne das gebaute Kristall-Design im Dogfood **komplett verworfen**: „nicht wie aus einem Guss", „kein gutes Layout", „Gesamtkonzept gefällt mir nicht", „frischer Wind — alles was vorhanden ist, muss so nicht weiter existieren". Auftrag: den **vorhandenen Inhalt bestmöglich darstellen** (Prod-DB-Dump als Referenz), alle bisherigen Design-Vorgaben ignorieren (Glyphen-Formcodierung, Glas/Facetten/Twilight, direction-b).

Diagnose des Scheiterns (aus den Dogfood-Screenshots): 20 gleiche weiße Rundkarten ohne Führung; konkurrierende Identitätssysteme (Farbe+Glyphe+Icon+Logo); Nullen und Roh-Markdown als Helden; überall abgeschnittene Namen; zwei inkonsistente Sidebar-Welten.

## 2. Werte & Gesetze (Soenne, 2026-07-03/04)

1. **Ein Work/Wissen-Hub** — ganzheitliches, rundes Konzept; eine Sprache für beide Welten.
2. **Übersichtlich. Inhalt steht im Vordergrund. Wissen wird vermittelt und einfach gefunden.**
3. **Max. 3 Spalten am Desktop** (Lesesaal nutzt 2: Bühne + funktionale Meta-Spalte).
4. Moderner Stil, **keine Serifen-Klassik** (kein Times-Gefühl).
5. **Kein Identitäts-Zirkus pro Projekt**: kein per-Node-Farb/Glyphen/Icon-System. Logo, wo eins existiert — sonst Initialen-Ersatz.
6. Zukunfts-Features gehören ins Konzept: **Kontext sortieren/priorisieren, Artefakte in Dokumenten, Seiten-Verlinkung, Mermaid**.
7. Keine Emoji-Piktogramme; Motion hinter `prefers-reduced-motion`; hell und dunkel als Zwillinge (Hell ist Zuhause).

## 3. Inhalts-Wahrheit (Prod-Dump, Stand 03.07.)

- **22 Knoten**: 2 Engagements (RTL Extern 304:46 h · Privat), 5 Vorhaben (kurz benannt), 15 Repos — Namen bis **86 Zeichen** (git-Remote-URLs), inkl. Schmutzdaten (`gitlab.com>/…`).
- **305 h in 41 Sessions** seit 24.04., fast alles auf Engagement-Ebene (Import) → auf Repo-Ebene sind **Null-Zustände der Normalfall**.
- **264 Dokumente, ~81 % agent-verfasst** (memory 93 · plan 66 · spec 48 · project 38 · daily 10 · Rest 9), Ø 13 KB, max. 108 KB; 139 Docs mit Wikilinks (382 Links), 12 Pins, 115 Tags, Mermaid in 3 Docs.

**Implikationen:** (a) Lange Pfade sind die Identität → Typografie-Aufgabe, nie Truncation. (b) Lese-Typografie und Verweise sind erstklassig. (c) Provenance (Mensch/Agent · wann) überall sichtbar. (d) Rollups über die Kette tragen die Zahlen, Nullen kriegen keine Bühne. (e) Tag-Wand taugt nicht als Navigation → Suche + Typen sind die Türen.

## 4. Konzept „Lesesaal"

flow als **ruhige Arbeitsbibliothek mit Ingenieurs-Seele**. Struktur entsteht aus Weißraum, Haarlinien und Typo-Skala — nicht aus Kästen. Der Charakter kommt aus der Typografie (Grotesk + Mono) und dem Pfad-Rückgrat, nicht aus Effekten.

**Signatur — das Pfad-Rückgrat:** Jede Knoten-/Dokumentseite beginnt mit der Kette: Vorfahren klein und klickbar (`RTL Extern / backstage /`), das letzte Segment groß in der Display-Grotesk, der volle Pfad darunter in Mono mit Kopier-Affordanz. Die 86-Zeichen-URL wird Bühne statt Problem.

**Doktrinen:**
- **Haarlinien statt Karten.** Inhalt (Prose, Listen, Regale, Baum, Puls) liegt nackt auf Papier, getrennt durch 1px-Linien und Zeilen-Hover.
- **Zwei-Flächen-Regel** (v2.4): *Inhalt liegt auf Papier, Instrumente liegen auf Fläche.* Genau eine zweite Fläche (`--panel`) für Funktionszonen: Cockpit-Meta-Blöcke (Kette · Kontext · Bindings), Start/Stop-Band, ToC/Verweise am Dokument, Wochenskala, „Jetzt"-Zeile. Kein Schatten, Radius 14, niemals verschachtelt, niemals für Inhalt.
- **Nullen ohne Bühne.** Leere Werte erscheinen als „—" in Zeilen, nie als 0:00-Kacheln; die Kette liefert die tragende Zahl daneben.
- **Namen nie abschneiden.** Kurzname groß + voller Pfad als Zweitzeile (Mono, `word-break`), Ellipsis nur in schmalen Panels mit `title`.

## 5. Navigationsmodell: Topbar + Drill + Palette

**Keine Seitenleiste.** Begründung (Soenne): der Baum aus Repo-URLs funktioniert in keiner Spaltenbreite. Stattdessen:

1. **Topbar** (sticky, 58px, Papier mit Blur): Wortmarke `flow.` · Bereiche **Projekte · Wissen · Zeit** (immer sichtbar, aktive Unterstreichung in Akzent) · Suche/⌘K · **Timer-Pill** (Live-Punkt + Mono-Uhr + Kurzname) · eigener Avatar.
2. **Projekte-Seite = der Baum als Inhalt** in voller Breite (Engagement-Header mit Doppellinie → Vorhaben-Zwischenköpfe → Repo-Zeilen mit Einzug).
3. **Pfad-Rückgrat** ersetzt den Baum als Orientierung auf jeder Detailseite; Kinder stehen als Listen im Inhalt.
4. **⌘K-Palette**: fuzzy Springen zu Knoten + Dokumenten, MRU zuerst (wie TUI-Picker); „kein Treffer → Enter legt neu an".
5. **Kurznamen-Regel**: Anzeigename = letztes Pfadsegment; bei Kollision im sichtbaren Kontext genau ein Segment dazu (`gitlab / project`, `oopii / base-infra`). Ableitung serverseitig, überall dieselbe.

Sichtbarkeits-Prinzip gewahrt ([[feedback_navigation_discoverability_over_minimalism]]): die drei Bereiche sind permanent sichtbar; nur der tiefe Baum wandert von „Möbel" zu „Inhalt + Sprungziel".

## 6. Design-Token (normativ, Mockup v2.4)

**Grund:** `paper #FAFAF7` · `ink #1C1B18` · `meta #6E6961` · `faint #98938A` · `hair #E7E4DC` · `hair2 #D9D5CA` · `wash #F3F1EB` (Hover) · `sheet #F4F2EC` (Code) · `panel #F0EDE4` + `hairp #E0DBCD` (Instrumente).
**Akzent & Zustände:** `accent #2B5BF6` · `accent-deep #1D46D8` · `accent-wash #EAF0FE` · `live #0F8A46` (Text) · `live-bright #1CC161` (Punkte/Balken) · `live-wash #E3F7EB` · `warn #B45309` · `warn-wash #FBF0DC`.

**Typografie:** **Schibsted Grotesk** (variabel 400–900; UI + Display; SIL-OFL — vendoren) + **JetBrains Mono** (variabel; Pfade, Zahlen, Zeiten, Typ-Keys; `tabular-nums`). ClashDisplay und Inter werden ausgemustert. Skala: Display 38/34/32 (mobil 27–29) · Prose 15.5/1.72 (mobil 15) · UI 15/14.5 · Meta 13/12.5 · Mono 12–13 · Eyebrow 11 Versalien +.09em.

**Form:** Radius 14 (Panels/instr) · 7–9 (Buttons, Chips, Avatare 28/36) · 12–22 (Avatare 56/64/96). Schatten nur am Palette-Overlay. Ein Primär-Button pro Sicht (Akzent, ohne Gradient); alles andere `quiet` (Haarlinien-Rand).

## 7. Farb-Prinzip: genau zwei Quellen + Funktion

1. **Dokumenttyp-Farben** (fest, semantisch, überall gleich): project/notiz-blau `#2554E8/#E8EEFE` · plan-violett `#7A3FE4/#F1EAFE` · spec/notiz-petrol `#0B8A7B/#E2F5F2` · memory-bernstein `#B45309/#FDF0D9` · daily/free-grün `#15883E/#E5F6EA` · Reserve-koralle `#D14324/#FDEAE4` · context → violett. Auf Typ-Chips und Regal-Keys.
2. **Ersatz-Avatare**: deterministische Tönung aus dem Namen (Hash → 6 satte Töne, weiße Initialen): `#3D6BF0 · #8250DF · #0E9888 · #D97706 · #1FA254 · #E25C3C`. **Farbe pro Projekt lebt ausschließlich im Avatar** — keine farbigen Zeilen, Punkte, Rahmen pro Knoten.
3. **Funktionale Farbe**: Live = Grün (Timer-Punkt, LIVE-Chip, Heute-Balken), Budget-fast-voll = Bernstein, Fehler = Rot-Reserve.

**Kinds bleiben neutral** (engagement/vorhaben/repo als Text-Chip) — die alte Formcodierung ●◆⬡ ist tot.

## 8. Identität: Logo oder Initialen — sonst nichts

- Knoten **können** ein Logo tragen (Upload existiert: `node_logos`, Migr 0027). Logos liegen auf weißem Tile mit Haarlinie.
- **Ohne Logo: Initialen-Ersatz** (getönt, nie leer, nie Platzhalter-Optik). Größen: 28 (Zeilen) · 36 (Listen/Topbar) · **96 im Cockpit-Rückgrat** (mobil 64), daneben „Logo hinzufügen".
- Akteure: Menschen = getönte Initialen; **Agenten = gestrichelter neutraler Rahmen** (z. B. `CL`).

## 9. Seiten (IA)

| Seite | Kern |
|---|---|
| **Schreibtisch** (Home) | Einspaltig, schmal (860px): Jetzt (Timer-Panelzeile) → Weiterarbeiten (MRU-Knoten) → Zuletzt im Wissen → Puls (LIVE). |
| **Projekte** | Baum als Inhalt: Engagement-Header (Σ h, Satz, Work/Privat-Note) → Vorhaben-Köpfe → Repo-Zeilen (Avatar · Kurzname · Mono-Pfad · rechts h/Docs/zuletzt). Schmutzdaten sichtbar mit dezenter „Pfad prüfen?"-Note. |
| **Cockpit** `/nodes/{id}` | Pfad-Rückgrat (96er-Identität) → instr-Band (■ Stop/▶ Start · Nachbuchen · Mono-Statistik hier/Repo/Kette) → Wissen-Liste → Puls. Meta-Spalte: Kette · **Kontext-Instrument** · Bindings. |
| **Dokument** | Lesespalte max. 680px + Meta-Spalte (ToC · Verweise beide Richtungen · Kontext-Rang). Provenance-Zeile unter dem Titel (Akteur · Zeit · Pfad · Lesezeit · Bearbeiten/Anpinnen). |
| **Wissen** | Suche als Haupttür (großes Feld, „Volltext + semantisch") → **Regale nach Typ** (dt. Namen + Typ-Key + Count) → Zuletzt aktualisiert. **Tags nur als Suchfilter, nie als Wand.** |
| **Zeit** | Tages-Ledger (Von–Bis Mono · Ziel · Dauer, LIVE-Zeile) → Wochenskala (Panel, Balken akzent/heute-grün, Soll-Zeile) → Werkzeuge (Export · Freie Tage · Statistik). |

## 10. Instrumente

- **Timer: genau einer.** Topbar-Pill (überall, tickend, verlinkt zum Ziel) + Start/Stop im Cockpit-instr-Band. Keine dritten Start-CTAs.
- **Kontext-Instrument** (Zukunfts-Feature, hier erstklassig): Budget-Meter (`11.891 tk / 12.000`, Bernstein ab ~95 %), Enthalten/Verworfen/Pins-Zeilen, nummerierte Top-Pins, **„Kuratieren — sortieren & pinnen ›"** (Reihenfolge + Prioritäten; Backend cap+rank existiert, Prioritäts-/Reorder-API ist neu). Am Dokument: „Im Agenten-Kontext · enthalten ✓ · Rang 04/24" + Anpinnen.
- **Provenance** an jedem Wissens-Artefakt: wer (Mensch/Agent) · wann · Typ.

## 11. Lese-Ebene (Markdown)

- Volle Doc-Typografie: H2 mit Haarlinie oben, Tabellen mit Versalien-Köpfen, Code auf `sheet`, Blockquote-Lede, Warn-Callouts (Bernstein).
- **Mermaid gerendert als gesetzte Figur** mit Nummer + Unterschrift („Abb. 1 · … — gerendert aus ```mermaid").
- **Artefakte als Figuren**: Datei-Chip (EXT-Badge · Mono-Name · Größe · Herkunft · Öffnen ↗) bzw. eingebettete Vorschau; nummeriert als Anhang.
- **Wikilinks** akzentblau gepunktet unterstrichen; Backlinks („hierher") und ausgehende Verweise („von hier") in der Meta-Spalte.
- **Eindämmungs-Systemregel — Markdown ist Fremd-Inhalt:** Agenten schreiben beliebig breite Tabellen/Codezeilen/Dateinamen. Deshalb systemisch: `html,body{overflow-x:clip}` · Prose `overflow-wrap:break-word` · jeder breite Block scrollt im eigenen Rahmen (`pre`, Tabellen-Wrapper, Diagramm-Frame; Diagramme behalten mobil ihre Naturgröße und scrollen) · Flex-Kinder mit Textinhalt brauchen `min-width:0` (gelernt am Suchfeld: Platzhalter verhinderte Schrumpfen). Die Seite pannt **nie** horizontal.

## 12. Responsive

- **≥960px**: Bühne + Meta-Spalte (280px Cockpit / 250px Dokument), Gap 56–64.
- **<960px**: Meta-Spalte stackt unter die Bühne (Panels bleiben Panels); Prose volle Breite.
- **<620px**: Topbar kompakt (Suche = Icon, ⌘K-kbd weg, Timer-Pill ohne Namen), Zeilen zeigen nur den Hauptwert (`.k`-Unterzeile entfällt, Werte bleiben), Hero 96→64, Sektions-Köpfe brechen um, Such-Platzhalter kurz.

## 13. A11y · Motion · i18n

Fokus sichtbar (2px Akzent, Offset 2) · aria-Labels an Icon-Controls, Meter mit `role="img"` + Label · Text-Kontraste ≥4.5:1 auf Papier und Washes (Avatar-Initialen dekorativ, ≥3:1, Name steht daneben) · Motion nur Hover/Unterstreichung + tickende Uhr, alles hinter `prefers-reduced-motion` unkritisch · keine Popups (Guard bleibt) · i18n de/en Parität.

## 14. Dunkel-Zwilling

Prinzip: **Token-Flip, Hell ist Zuhause.** Anker (bei Umsetzung zu kalibrieren, gleiche Struktur, keine neuen Regeln): paper→`#171613` · ink→`#ECEAE4` · panel→`#201D18` · Haarlinien entsprechend · Typ-/Avatar-Töne eine Stufe dunkler/entsättigter, Akzent bleibt elektrisch. Eigener Politur-Task inkl. Kontrast-Pass; Theme-Wahl persistiert wie bisher.

## 15. Nicht-Ziele

Kein per-Kind/per-Projekt-Farbsystem · keine Glyphen-Formcodierung · keine Glas/Facetten/Twilight-Sprache · keine Karten für Inhalt, keine Karten-in-Karten · keine Tag-Wand · kein Seitenleisten-Baum · keine Gradient-CTAs.

## 16. Technik-Realität & Lücken (Input für writing-plans)

1. **CSS-Fundament neu**: Tokens/Primitives ersetzen die Kristall-Schicht (tailwind.css/app.css, templ-Komponenten); `verify-css`-Guard mitziehen; Fonts vendoren (Schibsted rein; ClashDisplay/Inter raus).
2. **Shell-Umbau**: Sidebar-AppShell → Topbar-Shell (alle ~16 AppShell-Caller); Timer-Pill ersetzt Timer-Widget/Chip; ⌘K-Palette (Endpoint: fuzzy Knoten+Docs, MRU) — Suche nutzt bestehende Hybrid-Search.
3. **Kurznamen-Helper** serverseitig (letztes Segment + Kollisions-Dedup) — eine Quelle für Topbar, Listen, Palette, Pills.
4. **Logo groß**: Upload/Serving existiert; Render-Größen 96/64 + „Logo hinzufügen"-Affordanz; Initialen+Tönung als Fallback-Komponente (Hash-Funktion einheitlich Go).
5. **Kontext-Kuratierung**: Prioritäts-Feld + Reorder/Pin-API + Kuratieren-UI (neu; cap+rank/Pins existieren) — eigener Slice.
6. **Artefakt-Embeds**: Attachment-Storage + Markdown-Einbettung + Figuren-Render (neu; ggf. eigener Mini-Brainstorm zu Storage/Format).
7. **Mermaid**: Render-Weg entscheiden (vendored mermaid.js client-side vs. server-side SVG) — im Plan festlegen; CSP/offline beachten.
8. **Provenance**: Activity kennt Akteure; Documents brauchen ggf. created_by/updated_by-Felder (prüfen — Notiz vom 29.06.).
9. **Lesezeit**: trivial (Wortzahl/220) — nice-to-have, kein Blocker.
10. **Prozess offen**: `cockpit-story` (K1–K5, code-complete) mergen und Lesesaal als neues Programm obendrauf bauen — Empfehlung — oder Lesesaal direkt auf dem Branch weiterführen. **Soenne-Call vor writing-plans.**
11. Konstanten: multi-tenant/owner-scoped, SSE-Live-Sync-Regel, `make ci` (75 %-Gate, output-asserting Tests), kein `make fmt` in Dispatches, subagent-driven mit Main-Wiring-Task.

## 17. Slicing-Vorschlag (je Slice eigener Just-in-time-Plan)

**L1 Fundament** (Tokens, Fonts, Zwei-Flächen-Primitives, Topbar-Shell, Timer-Pill, ⌘K-Palette) → **L2 Projekte + Cockpit** (Baum-als-Inhalt, Pfad-Rückgrat, instr-Band, Kette/Bindings-Panels, Logo/Ersatz groß) → **L3 Lese-Ebene** (Dokument-Seite, Eindämmung, Mermaid, Wikilinks/Backlinks-Rail, Wissen-Regale + Suche) → **L4 Schreibtisch + Zeit** → **L5 Kontext-Kuratierung** → **L6 Artefakte** → **L7 Dunkel-Zwilling + Politur-Gate**. Reihenfolge L5/L6 tauschbar; L1–L4 ersetzen die sichtbare App vollständig.
