# Übergabe: Karteikasten — Designkonzept für die flow-Oberfläche

## Worum es geht

41 Mockups plus Elementkatalog für die komplette flow-WebUI: Schiene, Lesesaal,
Zeit, Tagebuch, Ausstattung, Repo, Overlays, Dunkelmodus, Tablet und Telefon.
Ziel der Umsetzung ist **nicht**, das HTML auszuliefern, sondern die Oberfläche
in der bestehenden flow-Umgebung nachzubauen: **templ + htmx + Tailwind v4,
serverseitig gerendert, kein SPA, kein Node-Runtime**.

## Was in diesem Paket liegt

| Datei | Inhalt |
| --- | --- |
| `design/flow - Designkonzept.dc.html` | Das Konzeptdokument: Kapitel 01–04 (Haltung, mentales Modell, Navigationsbaum, Elementkatalog 3.1–3.13) **plus alle 41 Mockups** in Arbeitstag-Reihenfolge |
| `design/Karteikasten III.dc.html` | Dieselben 41 Mockups ohne Kapiteltext — zum Nebeneinanderlegen beim Bauen |
| `design/K3Rail.dc.html`, `K3Article.dc.html`, `K3Crumb.dc.html` | Wiederverwendete Bausteine der Mockups (Schiene, Artikel, Brotkrume) |
| `design/fonts/*.woff2` | Schibsted Grotesk + JetBrains Mono, Variable |
| `design/flow-banner*.svg` | Register-Banner hell/dunkel |
| `TOKENS.md` | Jeder Farb-, Maß- und Typo-Wert des Designs, mit Zuordnung auf `web/tailwind.css` |
| `SCREENMAP.md` | Screen → Route → `.templ`-Datei, in empfohlener Bau-Reihenfolge |
| `CLAUDE-CODE-PROMPT.md` | Der Text, mit dem Du Claude Code startest |

Die `.dc.html`-Dateien öffnen direkt im Browser (Doppelklick). `support.js`,
`image-slot.js` und der Ordner `fonts/` müssen dabei daneben liegen — sind sie.

## Fidelity: hoch

Alle Werte sind endgültig: Hex-Codes, Schriftgrößen auf halbe Pixel, Zeilen- und
Spaltenmaße, Zustände. Die Umsetzung soll **pixelnah** sein, mit den Tailwind-
Tokens des Repos — nicht mit Inline-Styles und nicht mit neuen Utility-Klassen,
wo ein Token existiert.

## Die eine große Vorentscheidung: Token-Drift

`web/tailwind.css` trägt heute die Studio-Identität aus
`direction-b-APPROVED.html`: Papier `#FAFAF7`, Akzent Blau `#2B5BF6`.
Das Karteikasten-Design ist **wärmer und ockerfarben**: Papier `#F8F6F0`,
Karton `#F1EDE2`, Akzent Ocker `#B8720F`.

Das ist kein Detail, das man pro Seite nachzieht — **das ist der erste Slice**:
die Tokens in `web/tailwind.css` (`:root` und der Dunkel-Block) auf die Werte in
`TOKENS.md` umstellen, `make web`, `make verify-css`, dann visuell durch alle
bestehenden Seiten gehen. Erst danach Screens bauen. Alles andere führt zu einer
halb umgestellten App, in der niemand mehr sagen kann, was Absicht ist.

Die Farbnamen bleiben, wo möglich, dieselben (`--canvas`, `--sunken`, `--line`,
`--ink`, `--accent`, `--live`, `--warn`) — es ändern sich die Werte, nicht die
Semantik.

## Reihenfolge (Slices)

1. **Tokens** — `web/tailwind.css`, `make web`, `verify-css` grün. Keine Seiten-
   änderung außer den Token-Werten.
2. **Schiene + Hülle** — `components/appshell.templ`, `cockpit_rail.templ`:
   264px, Ebenen-Einzug, 3px-Ebenenstreifen über der Fläche, Farbe je Ebene.
   Danach sieht jede Seite schon richtig aus.
3. **Listenzeile + Datumsspalte** — der Baustein, der in 20 Screens vorkommt
   (Katalog 3.10): eine Bedeutung pro Spalte, `zuletzt geändert` als Standard,
   gestaffelte Schreibweise, Sortierkopf, Filterchips.
4. **Lesesaal** — `document.templ`, `wissen.templ`, `K3Article` (Screens 01, 10).
5. **Zeit** — `heute.templ`, `woche.templ`, `historie.templ`, `frei.templ`
   (Screens 09, 05, 32, 26, 25).
6. **Schreiben** — `editor.templ` (11, 16), `kontext.templ` (07).
7. **Ausstattung** — neu (13, 13 A, 13 B); Repo (15, 15 A, 33).
8. **Zustände und Formate** — Dialoge (19), Fehlerseiten (20), Leer/Lädt
   (Katalog 3.8), Dunkelmodus (22), Tablet (21 A–D), Telefon (21).

## Regeln aus dem Repo, die beim Bauen gelten

Diese stehen in `AGENTS.md` und sind nicht verhandelbar:

- **i18n**: keine Anzeigetexte im Template. Jeder String als Schlüssel in
  `internal/i18n/catalog_de.go` **und** `catalog_en.go`, im Template
  `components.T(ctx, "key")`. Parität ist testgeprüft. Die Mockups sind auf
  Deutsch — die englischen Fassungen gehören mit angelegt (Katalog 3.12 nennt
  die Wortpaare für die Datumsstaffel).
- **Keine Browser-Popups** — Bestätigen läuft über `components.ConfirmDialog`
  (Screen 19 ist genau dieses Muster). `make verify-no-popups` prüft das.
- **Keine Emoji.** Zeichen sind Monospace-Glyphen (● ◆ ⬡ ▶ ■ ▾ ▴ ✓ ⌕) und SVG.
  Der Katalog 3.6 listet, welches Zeichen was bedeutet.
- **Eine Verantwortung pro Datei.** Neue Flächen bekommen eigene `.templ` +
  eigenes `_vm.go`, nicht einen Anbau an `cockpit_main.templ`.
- **htmx**: Fragmente als `XFragment(vm)`, SSE über `hx-ext="sse"` am Body und
  `hx-trigger="sse:<event>"` am Container. Im Cockpit zielt alles, was den
  Tab-Bereich neu rendert, auf `#cockpit-main`.
- **generierte Dateien committen**: `make generate` (templ) und `make web`
  (CSS), sonst laufen `verify-generate` / `verify-css` rot.
- **`make ci` muss grün sein**, bevor irgendetwas „fertig" heißt. `make fmt`
  **niemals** laufen lassen.
- **TDD**: erst der fehlschlagende Test, dann der Code. Die 75%-Abdeckungs-
  schwelle gilt (`*_templ.go` ausgenommen) — Tests, die echte Ausgabe prüfen.

## Verhalten, das die Mockups zeigen

- **Bewegung** (Katalog: Zeiten in 3.7): Auswahl 0ms — sofort. Aufklappen 140ms,
  Panel/Overlay 180ms, Dialog 200ms, alles `cubic-bezier(.2,0,.3,1)`. Uhr,
  Live-Punkt und Zähler animieren **nicht** weich, sie springen.
- **Leerzustand** trägt die Zeilenhöhe der echten Liste und einen Satz, der
  sagt, was hier hineinkäme. **Ladezustand** erst nach 400ms, nie sofort,
  versetzt pulsierende Platzhalterzeilen in Zeilenhöhe.
- **Fokus** ist immer sichtbar (3.9): Felder sind Linien, keine Kästen; die
  Beschriftung steht über dem Feld, nie als Platzhalter darin.
- **Tastatur**: ⌘K Suche, ⌘N neue Karte, ⌥L Kuratieren-Panel, Esc schließt an
  die Stelle zurück, aus der geöffnet wurde.
- **Responsiv** (Katalog 3.13, Screens 21 A–D und 21): Umbrüche bei **768** und
  **1200** px. Über 1200 drei Spalten; 768–1199 Schiene auf 76px Monogramme und
  die mittlere Spalte wird **Band** (Steuerung, die stehen bleiben muss) oder
  **Panel** (Liste, aus der man wählt); unter 768 Dock unten, jede Fläche eine
  eigene Seite. Trefffläche auf Berührungsgeräten min. 44px, kein Hover-Zustand
  als einziger Hinweis, langes Tippen statt Rechtsklick.

## Was das Design **nicht** vorgibt

Datenmodell, Store-Queries, Usecases, Routen-Namen. Die Screens zeigen Zustände,
nicht Endpunkte. Wo ein Mockup Zahlen zeigt (Kartenzahlen, Summen, Budgets),
sind sie erfundene, aber untereinander konsistente Beispielwerte — sie sagen
etwas über Format und Stellenzahl, nichts über echte Inhalte.

## So gibst Du es Claude Code

`CLAUDE-CODE-PROMPT.md` enthält den fertigen Text. Kurz:

1. Dieses Paket in das flow-Repo legen, z. B. als
   `docs/superpowers/specs/assets/2026-08-karteikasten/`.
2. Claude Code **im Repo-Wurzelverzeichnis** starten, damit `AGENTS.md` und
   `CLAUDE.md` greifen.
3. Den Prompt aus `CLAUDE-CODE-PROMPT.md` schicken — er verweist auf die
   Dateien, nennt die Slice-Reihenfolge und verbietet den großen Rundumschlag.
4. **Slice für Slice** arbeiten lassen, jeweils mit `make ci` als Schlusspunkt
   und einem eigenen Commit. Nicht alle 41 Screens in einer Sitzung.
