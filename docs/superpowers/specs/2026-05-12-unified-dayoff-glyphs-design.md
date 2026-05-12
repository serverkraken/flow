# Vereinheitlichung der Free-Day-Glyphen über Worktime-Surfaces

**Datum:** 2026-05-12
**Status:** approved (Option A, full scope)
**Scope:** TUI worktime — Pace-Strip · Heatmap · Monatsraster · Frei-Tab · **tmux Status-Segment**

## Problem

Die Wochen-Fortschrittsanzeige (Pace-Strip in `internal/frontend/tui/screen/worktime/week.go` `renderPace`) mischt in einer 7-Punkte-Zeile vier unterschiedliche Glyph-Familien:

| Glyph    | Quelle                  | Visuelle Familie                         |
| -------- | ----------------------- | ---------------------------------------- |
| `●` `○`  | `glyphs.Filled/Empty`   | flache Kreis-Geometrie                   |
| `★`      | `glyphs.Holiday`        | 5-Zack-Stern, eckig, visuell schwer      |
| `☼`      | `glyphs.Vacation`       | Sonne mit 8 Strahlen, visuell sehr breit |
| `✚`      | `glyphs.Extra`          | dicker Balken-Kreuz, fremde Geometrie    |
| `·`      | `glyphs.BulletDot`      | Mittelpunkt, kaum sichtbar               |

Alle Glyphen sind technisch Single-Cell (Unicode-Breite 1), aber das Auge liest **visuelles Gewicht**, nicht Zellbreite. `☼` neben `●` bricht den Spaltenrhythmus. Dieselbe Diskussion, die `glyphs.go:6-8` für `◐ ◓` schon dokumentiert ("some fonts render them at emoji-width and the column rhythm goes off"), gilt für die Familien-Mischung eine Ebene höher.

Dieselbe `dayOffGlyph()`-Helferin (`internal/frontend/tui/screen/worktime/helpers.go:51`) wird auch in der Heatmap (`history_heatmap.go:84`) und dem Monatsraster (`history_month.go:161`) verwendet. Dort ist der visuelle Bruch geringer (die Nachbar-Glyphen sind `░▒▓█`-Blockschattierungen, also selbst eine eigene Familie), aber das konzeptionelle Problem ist dasselbe.

**Das tmux Status-Segment** (gerendert von `flow worktime status` über `internal/domain/status.go`) trägt das Problem **doppelt**:

1. Die Pace-Dots-Reihe im Status-Right (`BuildPaceDots`, `status.go:199-240`) mischt `●`/`○` für Werktage mit `dotDayOffGlyph(k)` für freie Tage — die liefert wiederum `★/☼/✚` (`status.go:258-268`). Genau derselbe Familien-Bruch wie in der TUI-Pace-Strip, nur dass nur Mo–Fr gerendert wird.
2. Die `[Frei: …]`-Banner-Spalte am Segment-Anfang (`status.go:67-69`) nutzt `bannerDayOffGlyph(k)` mit derselben `★/☼/✚`-Whitelist (`status.go:244-254`).

Beide Helfer kollabieren die Kind-Information ins Glyph und rendern die Farbe pauschal `pal.Cyan` — das Kind wird in tmux heute überhaupt nicht farblich differenziert.

Zusätzlich: die Heatmap (`history_heatmap.go:86`) und das Monatsraster (`history_month.go:162`) hardcoden die Farbe **jedes** freien Tages auf `Sem.Info` — die Kind-Unterscheidung (Feiertag/Urlaub/Krank) lebt heute **ausschließlich** im Glyph. Wenn wir den Glyph vereinheitlichen, muss die Information auf die Farbachse wandern.

## Entscheidung: Option A — Eine Glyph-Familie, Farbe trägt die Semantik

Über alle fünf Surfaces hinweg gilt:

- **Genau ein Glyph** für jeden freien Tag: `○` (`glyphs.Empty`).
- **Die Kind-Information** (Feiertag/Urlaub/Krank) wird **ausschließlich** über die Foreground-Farbe codiert, gemäß der bereits existierenden Sem-Mapping aus `helpers.go:51-69` und `week.go:77-81`:
  - Feiertag (`KindHoliday`) → `Sem.Info` (cyan)
  - Urlaub (`KindVacation`) → `Sem.Success` (grün)
  - Krank (`KindSick`) → `Sem.Warning` (orange)
- **Fallback** für unbekannte Kinds: `○` (`glyphs.Empty`) in `p.Fg` — konsistent mit dem schon existierenden `kindColor()`/`kindStyle()`-Fallback. Der bisherige `BulletDot ·`-Fallback aus `dayOffGlyph()` fällt weg.

Das löst den Familien-Bruch, nutzt das bereits etablierte Farbschema (kein neues Mapping), kostet keine neuen Glyphen und reduziert `glyphs.Holiday/Vacation/Extra` zu reinen Markdown-/Legacy-Symbolen ohne Worktime-Konsumenten.

### Warum Option A statt B oder C

| Option                        | Tradeoff                                                                |
| ----------------------------- | ----------------------------------------------------------------------- |
| **A** (mono-glyph, color)     | wählt: tightster Diff, perfekter Rhythmus, nutzt vorhandene Sem-Farben  |
| B (zwei Lanes über/untereinander)  | verworfen: doppelte vertikale Höhe, bricht Pace-Strip-Layout        |
| C (gefüllter Kreis für freie Tage) | verworfen: Doppelbedeutung von `●` (Ziel erreicht + Tag abgegolten) |

## Scope: fünf Surfaces, ein Patch

### 1. Pace-Strip (`week.go renderPace`)

**Vorher:**

```
●  ●  ●  ★  ○  ·  ·     1/4 Ziele   ▼ behind
gr gr gr cy dim dim dim
```

(Mo–So: drei Werktage hit + ein Feiertag-Stern + ein Werktag-miss + zwei Wochenende-Punkte)

**Nachher:**

```
●  ●  ●  ○  ○  ○  ○     1/4 Ziele   ▼ behind
gr gr gr cy dim dim dim
```

- Werktag hit → `●` `Sem.Success`
- Werktag heute-läuft → `●` `Sem.Warning`
- Werktag offen → `○` `FgMuted`
- Wochenende → `○` `FgMuted`
- Feiertag → `○` `Sem.Info`
- Urlaub → `○` `Sem.Success`
- Krank → `○` `Sem.Warning`

**Änderung:** `dayOffPaceGlyph(k)` (`week.go:477-479`) wird obsolet — der Glyph ist immer `glyphs.Empty`. Der Aufruf in `week.go:374` wird zu `w.styles.kindStyle(dayOff.Kind).Render(glyphs.Empty)`.

### 2. Heatmap (`history_heatmap.go`)

**Vorher:** `dayOffHeatmapGlyph(k)` liefert `" ★ "` / `" ☼ "` / `" ✚ "`, Farbe immer `sem.Info`.

**Nachher:** Glyph immer `" ○ "`, Farbe via `kindColor(pal, k)` (Info/Success/Warning).

**Legende** (`history_heatmap.go:142`):

```
Vorher:  ★/☼/✚ frei
Nachher: ○ Feiertag · ○ Urlaub · ○ Krank
         cyan        grün       orange
```

Die Legende wird damit etwas länger, aber semantisch ehrlicher — heute steht da "frei" als Sammelbegriff, was die im Glyph codierte Kind-Information unsichtbar lässt.

### 3. Monatsraster (`history_month.go`)

**Vorher:** `renderMonthCell` (Zeile 160-162) setzt `glyph = dayOffGlyph(k)`, `color = sem.Info` für **jeden** Free-Day, egal welcher Kind.

**Nachher:** `glyph = glyphs.Empty`, `color = kindColor(pal, k)`. Damit wird in der Monatsansicht zum ersten Mal die Kind-Unterscheidung farblich sichtbar (heute verschluckt).

### 4. Frei-Tab (`dayoffs.go`)

Zwei Stellen:

**`renderKindSummary` (`dayoffs.go:508-521`):** die Übersichts-Chips oben (`"Feiertag 3  ·  Urlaub 7  ·  Krank 1"`) sind heute alle in `stDim` — Label und Count gleich grau. Das Skill-Prinzip "Color carries meaning" gebietet hier, das Kind-Label in seiner Sem-Farbe zu rendern; der Count bleibt dim, um die Hierarchie (Label > Count) sichtbar zu halten:

```
Vorher:  Feiertag 3  ·  Urlaub 7  ·  Krank 1     (alles dim)
Nachher: Feiertag 3  ·  Urlaub 7  ·  Krank 1
         cyan dim       grn  dim     org  dim
```

**`renderKindPicker` (`dayoffs.go:590-606`):** im Add-Dialog. Hier bleibt die §Color-semantics-Regel "one accent per row" maßgeblich: das gewählte Chip bekommt `Sem.Accent` (Selection), die anderen bleiben `FgMuted`. **Aber** als visuelle Hilfe bekommt jeder Chip einen führenden farbigen `○`-Glyph in seiner Kind-Farbe, sodass das Mapping auch im Picker selbst sichtbar wird:

```
Vorher:  Feiertag    Urlaub      Krank        (alles dim, Cursor accent-underlined)
Nachher: ○ Feiertag  ○ Urlaub    ○ Krank
         cyan dim    grn dim     org dim      (Cursor-Chip: Akzent + Underline auf Glyph+Label)
```

Der `kindCell` in `renderEntryRow:526` benutzt bereits `kindColor()` — keine Änderung nötig, schon konsistent.

### 5. tmux Status-Segment (`internal/domain/status.go`)

Das Status-Right-Segment ist die fünfte und in der Hierarchie wichtigste Surface — es ist die Anzeige, die der User **dauerhaft** sieht, auch ohne Worktime-Screen geöffnet zu haben. Hier wirkt der Glyph-Mix am sichtbarsten, weil das Segment in einer einzigen Zeile direkt neben der Uhrzeit, dem Streak-Counter und dem Burndown-Indikator sitzt.

#### Pace-Dots in tmux (`BuildPaceDots`)

**Vorher:**

```
[Frei: Tag der Arbeit] ⏱ 06:12 ▶ 1:14 ✓ ● ● ★ ○ ●
                                       gr gr cy dim gr
```

**Nachher:**

```
[Frei: Tag der Arbeit] ⏱ 06:12 ▶ 1:14 ✓ ● ● ○ ○ ●
                                       gr gr cy dim gr
```

- Werktag hit → `●` `pal.Green`
- Werktag heute-läuft → `●` `pal.Yellow`
- Werktag offen → `○` `pal.Dim`
- Feiertag → `○` `pal.Cyan`
- Urlaub → `○` `pal.Green`
- Krank → `○` `pal.Yellow`

**Änderung:** in `status.go:213` wird `dot{dotDayOffGlyph(dayOff.Kind), pal.Cyan}` zu `dot{"○", kindStatusColor(dayOff.Kind, pal)}`. `dotDayOffGlyph()` (`status.go:258-268`) wird gelöscht.

#### Frei-Banner in tmux (`bannerDayOffGlyph`)

**Vorher:** `★ Tag der Arbeit` / `☼ Sommerurlaub` / `✚ Krankmeldung` — Glyph je nach Kind, Farbe immer cyan.

**Nachher:** `○ Tag der Arbeit` / `○ Sommerurlaub` / `○ Krankmeldung` — Glyph immer `○`, Farbe per Kind (cyan/grün/yellow).

**Änderung:** in `status.go:67-69` wird `bannerDayOffGlyph(in.DayOff.Kind), in.Palette.Cyan` zu `"○", kindStatusColor(in.DayOff.Kind, in.Palette)`. `bannerDayOffGlyph()` (`status.go:244-254`) wird gelöscht.

#### Neuer Helfer `kindStatusColor`

Eine einzige neue Funktion in `internal/domain/status.go`, parallel zu der TUI-seitigen `kindColor`/`kindStyle`. Sie mappt `Kind` auf die fünf-feldrige `StatusPalette` (die in der tmux-Welt nur Green/Yellow/Red/Cyan/Dim kennt):

```go
func kindStatusColor(k Kind, pal StatusPalette) string {
    switch k {
    case KindHoliday:  return pal.Cyan    // Info
    case KindVacation: return pal.Green   // Success
    case KindSick:     return pal.Yellow  // Warning → der Yellow-Slot der
                                          // StatusPalette wird heute schon
                                          // von Sem.Warning gefüttert
                                          // (theme/status_adapter.go:22),
                                          // rendert also denselben Orange-Hex
                                          // wie die TUI.
    }
    return pal.Dim
}
```

Warum kein neues `Orange`-Feld? Der existierende `StatusPaletteFor`-Adapter (`internal/frontend/tui/theme/status_adapter.go:21-25`) mappt `Yellow → Sem.Warning`. Der "Yellow"-Slot der `StatusPalette` rendert in der Praxis bereits den TUI-Orange-Hex `#ff9e64`. Ein neues Feld würde den Hex duplizieren ohne semantischen Gewinn. Der Name "Yellow" ist im Status-Composer ohnehin nur ein Slot, kein Farbversprechen — die Konvention wird im Doku-Header von `StatusPalette` festgehalten (siehe Doc-Änderung unten).

#### Doc-Update an `StatusPalette`

`status.go:11-13` bekommt einen Kommentar-Zusatz, der die Doppelnutzung des "Yellow"-Slots klarstellt: er trägt sowohl "Endspurt-Approaching" (`statusBanner`) als auch "Krank-Pace-Dot" (`BuildPaceDots`) — beide kontextuell distinkt im Render, beide Sem.Warning auf der TUI-Seite. Wer hier später aufräumt, kann den Slot in `Approaching` umbenennen und einen separaten `Warning` einführen — *out of scope für diesen Spec*.

#### Out of scope auf tmux-Seite

- **Wochenende-Punkte in tmux:** die Pace-Dots in tmux rendern nur Mo–Fr (`status.go:208-210`), die TUI rendert alle sieben Tage. Diese Asymmetrie bleibt. Begründung: der tmux-Segment hat horizontalen Spalten-Druck, fünf Punkte sind ohnehin schon viel.
- **`@tn_orange` als neue tmux-Option:** das oben begründet — keine Notwendigkeit, der Yellow-Slot trägt den Hex bereits.

## Architektonische Verschiebung

Heute gibt es zwei parallele Mappings, die dieselbe Aussage treffen:

- `kindColor(p, k)` in `week.go:461-472` — Sem-Farben für freie Tage
- `kindStyle(k)` auf `wocheStyles` in `week.go:77-92` — gecachte `lipgloss.Style` Variante

Plus `dayOffGlyph(k)` in `helpers.go:59` — der demnächst trivial wird (immer `glyphs.Empty`).

Die ersten beiden lassen wir bewusst nebeneinander stehen: `kindColor` ist außerhalb von `week.go` der Konsument (`dayoffs.go:526`), `kindStyle` ist die Hot-Path-Optimierung innerhalb `week.go` (round4-Performance-Arbeit). Beide referenzieren `Sem.Info/Success/Warning` — eine Änderung dort propagiert konsistent.

`dayOffGlyph()` und `dayOffPaceGlyph()` werden gelöscht. `dayOffHeatmapGlyph()` wird zu einer Konstante (`" " + glyphs.Empty + " "`) oder inline. Auf der tmux-Seite werden `bannerDayOffGlyph()` und `dotDayOffGlyph()` (`internal/domain/status.go`) gelöscht und durch den neuen `kindStatusColor()`-Helfer plus konstanten `"○"`-Glyph ersetzt. Die Whitelist-Glyphen `Holiday ★`, `Vacation ☼`, `Extra ✚` bleiben in `glyphs.go` definiert (Markdown-Renderer und ggf. zukünftige Konsumenten dürfen sie weiter nutzen), verlieren aber alle Worktime-Aufrufer.

Damit sammelt sich die Kind-zu-Farbe-Logik in genau zwei Stellen:
- **TUI-Seite:** `kindColor()` / `kindStyle()` (`week.go`, `helpers.go`) — beide gegen `Sem.Info/Success/Warning`.
- **tmux-Seite:** `kindStatusColor()` (`status.go`) — gegen `pal.Cyan/Green/Yellow`.

Beide Stellen drücken dasselbe Mapping in den jeweiligen Slot-Vokabularen ihrer Schicht aus. Eine Synchronisation per zentralem Helfer wäre möglich, würde aber die saubere Schichtentrennung (domain kennt keine TUI-Palette, TUI kennt keine fünf-feldrige StatusPalette) brechen.

## Test-Strategie

1. **`helpers_test.go:286-288`** wird angepasst — `dayOffHeatmapGlyph` liefert immer `" ○ "`, unabhängig vom Kind. Der Test verifiziert das.
2. **Neuer Test** in `week_test.go`: `renderPace` snapshot für eine Woche mit allen drei Kinds. Verifiziert dass alle freien Tage `glyphs.Empty` rendern und die Foreground-Farbe pro Kind unterscheidbar ist (via ANSI-Sequenz-Match).
3. **`history_month_test.go`** und **`history_heatmap_test.go`** (falls vorhanden — andernfalls neu): verifizieren dass freie Tage in der gewählten Kind-Farbe gerendert werden (nicht mehr pauschal `Sem.Info`).
4. **`dayoffs_test.go`**: snapshot oder Substring-Check, dass `renderKindSummary` jedes Kind-Label in seiner Sem-Farbe rendert.
5. **`internal/domain/status_test.go`** (existiert): Snapshot/String-Match-Tests müssen die alten `★/☼/✚`-Erwartungen auf `○` umstellen; neue Assertion: für jedes der drei Kinds liefert `BuildPaceDots` einen `○` mit der erwarteten `#[fg=…]`-Farbe (cyan/green/yellow). Banner-Test analog für `BuildStatusSegment`-Output.
6. **Glyph-Whitelist-Test** (`glyphs_test.go`): nichts ändert sich — die Glyphen bleiben definiert, nur die Worktime-Konsumenten ändern sich.
7. **Golden-File-Drift:** alle `*_baseline_test.go` und `render_repro_test.go` Snapshots laufen neu — Diff sichten, akzeptieren wenn nur die erwartete Visualänderung drin ist.

## Migration / Outstanding Risks

- **Cheatsheet/Help-Overlay:** `help.go` hat heute keinen Eintrag, der die Free-Day-Glyphen erklärt. Falls die Heatmap-Legende der einzige Erklär-Punkt ist (was sie war), reicht die aktualisierte Legend-Zeile. Wenn der `?`-Overlay einen Pace-Strip-Erklär-Abschnitt bekommen soll → separater kleiner Add-on im Plan, nicht im Pflichtumfang dieses Specs.
- **Konsumenten außerhalb worktime:** `glyphs.Holiday/Vacation/Extra` wird in `internal/kompendium/...` referenziert (`writepicker/model.go:87,90,92` nutzt aber `Filled/Empty/Extra` als generische Markdown-Icons, nicht als Free-Day-Marker). Diese Aufrufer bleiben ungetroffen.
- **A11y-Regel** der TUI-Skill (`§Visual hierarchy`): "Glyph + Farbe, niemals nur Farbe" — wird in der Heatmap heute ausdrücklich für die Heat-Cells eingehalten (`▲` für ≥150% statt nur Farbe). Mit Option A weicht **nur die Kind-Unterscheidung** auf reine Farbe aus. Begründung: alle drei Kinds teilen denselben Semantik-Cluster "Tag entfällt aus Arbeitsgründen ohne Defizit" — der Unterschied ist kontextuell sekundär. Wer die Farben nicht unterscheiden kann, sieht weiterhin den Glyph + die Status-Zeile (`history_heatmap.go:121`, `history_month.go:102`) mit dem expliziten `Kind.LabelDe()` Text als Fallback. Akzeptiertes Tradeoff.

## Implementierungs-Reihenfolge

Empfehlung für den Implementierungs-Plan (in der Reihenfolge der Abhängigkeiten):

1. **domain-Schicht zuerst** (`internal/domain/status.go`): neuer `kindStatusColor()`-Helfer, `bannerDayOffGlyph()` und `dotDayOffGlyph()` löschen, Aufrufer in `BuildStatusSegment` (Banner) und `BuildPaceDots` (Pace-Dots) anpassen, Doc-Kommentar auf `StatusPalette` ergänzen.
2. **`internal/domain/status_test.go`** mit der domain-Änderung gemeinsam: alte `★/☼/✚`-Erwartungen auf `○` umstellen, Farb-Assertions pro Kind hinzufügen.
3. **TUI-Seite, Helfer-Level:** `dayOffPaceGlyph` löschen, `dayOffGlyph()` löschen, `dayOffHeatmapGlyph()` zu Konstante degradieren.
4. **TUI-Seite, Render-Stellen:**
   - `week.go:374` (`renderPace`): `glyphs.Empty` mit `w.styles.kindStyle(k)`.
   - `history_month.go:160-162` (`renderMonthCell`): Glyph `glyphs.Empty`, Farbe `kindColor(pal, k)`.
   - `history_heatmap.go:84-86` (`renderHeatmapCell`): Farbe per Kind statt pauschal `Sem.Info`.
   - `history_heatmap.go:142` (`renderHeatmapLegend`): Legend-Zeile aufsplitten in drei farbige Chips.
   - `dayoffs.go:517` (`renderKindSummary`): Label in Kind-Farbe, Count in dim.
   - `dayoffs.go:603` (`renderKindPicker`): führenden `○ ` Glyph in Kind-Farbe vor jeden Chip.
5. **TUI-Tests** anpassen + Golden-Snapshots regenerieren.
6. **`make ci` grün**, dann **visueller Smoke-Test** in tmux: `flow worktime status` direkt aufrufen, Output mit erwartetem Banner + Pace-Dots prüfen; tmux refresh `tmux refresh-client -S` und Status-Right ansehen.

## Out of scope

- **Worktime-Code-Review Round-5-Fixes** (10 Befunde aus dem heutigen Review-Subagent): laufen als separater Plan/Stack. Pace-Strip-Redesign ist visuell, Round-5 ist korrektheits-/test-zentriert — entkoppelt landen ist klarer.
- **Notes-Marker-Dot in Heatmap/Monat:** der vor zwei Tagen gelandete `feat(worktime/history): Notes-Marker pro Tag`-Patch bleibt unverändert; der Dot lebt auf einer eigenen Position und kollidiert nicht mit dem Free-Day-Glyph.
