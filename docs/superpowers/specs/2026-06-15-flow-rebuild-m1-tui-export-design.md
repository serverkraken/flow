# flow Rebuild · M1 — TUI-Export-Affordance · Design

**Datum:** 2026-06-15
**Status:** Approved — Brainstorm abgeschlossen, User-Freigabe erteilt
**Scope:** TUI-Export-Panel als M1-Restpunkt. Bringt den server-berechneten Worktime-Export (CSV/JSON/Markdown, pro Projekt aggregiert + Detail; siehe [[2026-06-15-flow-rebuild-m1e-export-design]]) als interaktives Overlay in die Worktime-TUI. Schließt die M1e-Spec-vs-Plan-Lücke: die M1e-Spec nannte eine TUI-Export-Affordance in-scope, der M1e-Plan beschränkte den Slice bewusst auf CLI (`flow export` → stdout). Architektur-Fundament siehe `2026-06-13-flow-rebuild-design.md`, Worktime-TUI siehe `2026-06-14-flow-rebuild-m1a-worktime-design.md`.
**Branch:** Code auf dem langlebigen Orphan-Branch `rebuild` (kein main-merge pro Slice); Planungs-Docs auf `main` (M0/M1a–M1e-Präzedenz).

## Warum dieser Schnitt

M1 ist die Worktime-Vertikale „in Server + TUI + WebUI". REST/CLI/WebUI-Export sind mit M1e fertig und live-verifiziert. Die TUI hat bisher kein Export-Surface — der einzige verbleibende M1-Restpunkt nach den Done-Gate-Verifikationen (M1d-Stats + M1-Live-Sync, beide grün). Dieser Slice fügt das Panel hinzu; Rate-Setzen bleibt CLI/API (wie in M1e entschieden).

## Done-Gate (Akzeptanztest)

> `flow worktime` starten → Taste `e` öffnet das Export-Panel → Range (Preset oder freie von/bis) + Format wählen → Enter → die Datei liegt unter dem angezeigten Pfad und enthält den erwarteten Export (MD mit Projekt-Summary + Σh×Satz, CSV mit Detail-Zeilen). Esc schließt das Panel.

## Scope / Non-Goals

**In Scope:**
- Neues Export-Overlay in der Worktime-TUI (`internal/tui`), getriggert per Taste `e`.
- Range-Auswahl: Presets (aktuelle KW / aktueller Monat / letzter Monat) **plus** editierbare freie `von`/`bis`-Felder (yyyy-mm-dd).
- Format-Auswahl: csv / json / md (durchschaltbar).
- Editierbares Ziel-Pfad-Feld, vorbelegt mit `~/Downloads/flow-export-<von>_<bis>.<ext>`.
- Schreiben der vom Server gelieferten Bytes in die Datei + Pfad-/Fehler-Anzeige.

**Non-Goals (bewusst draußen):**
- **Kein Per-Projekt-Filter im TUI** — Export deckt immer alle Projekte ab; `flow export --project <slug>` per CLI bleibt der Weg für gefilterten Export.
- **Kein Rate-Setzen im TUI** — bleibt CLI/API (`flow project rate`), konsistent zu M1e.
- **Kein neues Server-/REST-/apiclient-Verhalten** — nutzt das bestehende `apiclient.Export(ctx, from, to, format, projectID)`.
- **Kein automatisches Öffnen der Datei** (kein `open`/xdg-open) — nur Pfad anzeigen.
- Pixelgenaue Politur → später.

## Bestehender Kontext (TUI)

- Worktime-TUI = ein einzelnes Bubbletea-`Model` in `internal/tui/worktime.go`. Overlay-Screens werden über Bool-Flags auf dem Model geschaltet (`showWeek`, `showStats`, `showDayOffs`) und über Tasten getriggert (`s` start, `x` stop/book, `d` Frei, `w` Woche, `t` Stats; Esc schließt; `q`/Ctrl-C quit). Stats-Overlay hat `m`/`W` für Monat/Woche.
- Overlay-Logik liegt in eigenen Dateien als Methoden auf demselben `Model` (`dayoffs.go` mit `handleDayOffKey`, `stats.go`). Neues Export-Overlay folgt diesem Muster in `export.go`.
- Das `Model` hält `client *apiclient.Client`. Netzwerk-Arbeit läuft als `tea.Cmd`, die eine Ergebnis-`tea.Msg` zurückgibt (Muster `reload()`/`startCmd()`).
- `apiclient.Client.Export(ctx, from, to, format, projectID string) ([]byte, error)` existiert (M1e) und liefert die rohen Datei-Bytes.
- Launch: `flow worktime` → `tui.New(client, user)` → Bubbletea-Programm; Logs gehen in eine Datei (slog darf die TUI nicht zerschießen).
- Styling: `internal/tui/styles.go` (Tokyonight-nah); Tasten-Hinweise im Footer/Help der View.

## Architektur & Komponenten

**Neu — `internal/tui/export.go`** (Methoden auf `Model`):
- `handleExportKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd)` — Key-Handling im Panel.
- `exportView() string` — Overlay-Rendering.
- `exportCmd() tea.Cmd` — ruft `client.Export(...)`, schreibt die Datei, gibt `exportDoneMsg`/`exportErrMsg` zurück.
- Helfer: Pfad-Expansion (`~` → Home), Preset→Range-Berechnung (nutzt `m.now`/Clock-Äquivalent für „heute"), Default-Pfad-Bildung.

**Geändert — `internal/tui/worktime.go`:**
- `Model`-Felder: `showExport bool`, `expPreset string` (`kw`|`monat`|`letzter`|`custom`), `expFrom string`, `expTo string`, `expFormat string` (`csv`|`json`|`md`), `expPath string`, `expPathEdited bool`, `expFocus int`, `expStatus string`.
- Haupt-Key-Switch: `case k.Text == "e"` → `showExport=true`, Defaults setzen (Preset=aktueller Monat, Format=md, Pfad vorbelegen), Range füllen.
- Esc-Handling: schließt das Export-Overlay (wie die anderen).
- Dispatch: wenn `m.showExport`, Keys an `handleExportKey` routen.
- `View()`: wenn `m.showExport`, `exportView()` rendern.
- Footer/Help: `e export` als Hinweis ergänzen (Nav-Symmetrie zu den anderen Screens).

## Interaktion (Panel)

Fokus-Reihenfolge (Tab / Shift-Tab, alternativ j/k): `[Preset] → [von] → [bis] → [Format] → [Pfad]`.

| Feld | Typ | Bedienung |
|---|---|---|
| Preset | Choice | ←/→ oder Space schaltet KW → Monat → letzter Monat → Custom |
| von | Text | Tippen/Backspace editiert (yyyy-mm-dd); Editieren setzt Preset = Custom |
| bis | Text | wie von |
| Format | Choice | ←/→ oder Space schaltet csv → json → md |
| Pfad | Text | Tippen/Backspace editiert; vorbelegt, setzt `expPathEdited=true` beim ersten Edit |

- **Preset-Wechsel** füllt `von`/`bis` neu (außer Custom). **Format-Wechsel** aktualisiert die Endung im Pfad, **solange** `expPathEdited==false`. **von/bis-Wechsel** aktualisiert den Default-Pfad-Range-Anteil ebenfalls nur solange `expPathEdited==false`.
- **Enter** löst den Export aus (von überall im Panel). **Esc** schließt das Panel.

## Datenfluss

1. `e` → Panel öffnet mit Defaults (aktueller Monat, md, Pfad `~/Downloads/flow-export-<von>_<bis>.md`).
2. User wählt Range/Format/Pfad.
3. Enter → Validierung: bei Preset=Custom werden `von`/`bis` als `2006-01-02` geparst; Parse-Fehler oder `bis < von` → `expStatus` Inline-Fehler, kein Call.
4. Sonst `exportCmd()`: `client.Export(ctx, von, bis, format, "")` → bei Erfolg Bytes in den expandierten Pfad schreiben (`os.WriteFile`, 0o644) → `exportDoneMsg{path}`; bei Netzwerk-/Schreibfehler `exportErrMsg{err}`.
5. `exportDoneMsg` → `expStatus = "✓ geschrieben: <pfad>"`; `exportErrMsg` → `expStatus = "Fehler: <…>"`. Panel bleibt offen.

## Error-Handling

- Ungültiges Custom-Datum / `bis < von` → Inline-Fehler im Panel, kein Server-Call.
- `apiclient.Export` non-200/Netzwerkfehler → Statuszeile zeigt den Fehler (kein Crash der TUI).
- Datei-Schreibfehler (Verzeichnis fehlt/keine Rechte) → Statuszeile zeigt den Fehler. (`~/Downloads` wird vorausgesetzt; bei Fehlschlag sieht der User den Pfadfehler und kann das Pfad-Feld anpassen.)
- Leerer Range (keine Sessions) → Server liefert validen Export (Header-only CSV / „keine Einträge" MD); Datei wird geschrieben, Pfad angezeigt.

## Testing

- **TUI-Unit-Tests** (`internal/tui/export_test.go`, Muster `worktime_test.go`/`stats_test.go`, Model-zentriert):
  - `e` → `showExport==true`, Defaults gesetzt (Format=md, Pfad endet `.md`, von/bis = aktueller Monat relativ zu `m.now`).
  - Preset-Cycling: Wechsel auf „letzter Monat" setzt erwartete `von`/`bis`.
  - Format-Cycling: md→csv aktualisiert die Pfad-Endung auf `.csv` (solange Pfad nicht editiert); nach manuellem Pfad-Edit bleibt der Pfad unverändert.
  - Fokus-Navigation: Tab wandert durch die Felder; Tippen editiert nur das fokussierte Textfeld.
  - Enter mit ungültigem Custom-Datum → Inline-Fehler, kein Cmd.
  - `exportDoneMsg{path}` → `expStatus` enthält den Pfad.
- **Done-Gate manuell:** wie oben (vs. realer Dev-Server) — `e` → Range/Format → Enter → Datei vorhanden + korrekt.

## Offene Punkte (für die Plan-Phase)

- Genaue Tasten für Choice-Felder (←/→ vs. Space vs. h/l) — Plan-Detail, an bestehende TUI-Konventionen anlehnen.
- Wie „heute"/Clock in der TUI bereitsteht (`m.now`) für die Preset-Berechnung — im Plan an der bestehenden Stats-Range-Logik spiegeln.
- Ob das Panel nach erfolgreichem Export automatisch schließt oder offen bleibt (Design: offen bleiben, Esc schließt) — bestätigt, kein offener Punkt, hier nur dokumentiert.
