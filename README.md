# flow

> Ein TUI-Sidekick für tmux — Worktime-Tracker, Markdown-Notizbuch und
> Befehls-Palette in einem Binary.

```
╭─ Heute · Woche · History · Frei ───────────────────────────────────╮
│  6h 42m    ▶ läuft    84%                                          │
│  ▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱  Ziel 8h 00m   noch 1h 18m   ETA 17:42     │
│                                                                    │
│  SESSIONS HEUTE (3) ───────────────────────────────────────────    │
│  ▶ 09:12 → …    2h 30m   läuft                                     │
│  ▎ 13:00 → 15:42   2h 42m   [deep]                                 │
│    16:00 → 17:30   1h 30m   [meeting]                              │
│                                                                    │
│  s → stoppen  ·  j/k → bewegen  ·  : → aktionen  ·  enter → bearb. │
╰────────────────────────────────────────────────────────────────────╯
```

`flow` lebt als Sidekick-Pane in tmux: ein dauerhaft sichtbares Panel
neben Deinem Editor, das die Werkzeuge bereitstellt, die Du beim
Programmieren ständig brauchst — Zeiterfassung, Notizen, Projekt-
Wechsel, Cheatsheet — ohne den Kontext zu verlassen.

---

## Was steckt drin

▶  **Worktime** — Sessions tracken, pausieren, korrigieren. Sessions
   pro Tag, Streak-Anzeige, Heatmap, Tag-Clock und Monatsraster für die
   History. Brief-Markdown, CSV/JSON-Export, Stats für beliebige
   Ranges. Feiertage werden pro Bundesland automatisch synced.

★  **Kompendium** — Markdown-Notizbuch mit FTS5-Suche, Daily-/Project-/
   Free-Notes, Wikilinks, Backlinks. Git-backed Sync, In-Editor öffnen
   (nvim default) und ein Browser-TUI mit voll integriertem Markdown-
   Viewer (Syntax-Highlighting, OSC-8-Hyperlinks, Theme-Aware).

●  **Sidekick** — TUI-Shell mit fünf Tabs: Palette · Projekte ·
   Worktime · Cheatsheet · Notes. Globaler Filter, fzf-style Fuzzy-
   Picker, persistenter UI-State.

☼  **Palette** — Universeller Befehls-Launcher (wie ein eigener
   tmux-Popup), durchsuchbar mit Live-Filter, Direktwahl per `1`-`9`,
   Pin-bare Favoriten.

▲  **Status-Bar** — `flow worktime status` liefert ein tmux-`status-right`-
   Segment, das den aktuellen Session-Stand auf einen Blick zeigt
   (`▶ 2h 30m`, `⏸ Pause`, `✓ 8h 00m`).

✓  **Eine Binary, drei Einstiegspunkte** — `flow sidekick` (TUI),
   `flow worktime <verb>` (Tracker-CLI), `flow kompendium <verb>`
   (Notizbuch-CLI).

---

## Installation

Voraussetzungen: **Go 1.25**, **tmux**, ein TrueColor-fähiges Terminal,
optional `glow` für Markdown-Render im tmux-Split.

```sh
git clone https://github.com/serverkraken/flow.git
cd flow
make install        # → ~/.local/bin/flow
```

Oder ohne Install nur ins lokale `bin/`:

```sh
make build
```

`flow --help` zeigt die Subcommands.

---

## Quickstart

### Worktime

Im Sidekick (TUI) — oder als Standalone-TUI über `flow worktime today`:

```
1   Heute              ·   2   Woche
3   History            ·   4   Frei (Feiertage / Urlaub / Krank)
:   Aktions-Menü (Brief, Export, Stats, Korrektur, Land)
?   Hilfe              ·   q   Beenden
```

CLI für die Bindings in `tmux.conf`:

```sh
flow worktime start          # jetzt starten — auch HH:MM oder -30m
flow worktime stop            # Session beenden (idempotent)
flow worktime pause           # Pause einlegen
flow worktime resume          # weitermachen
flow worktime toggle          # start wenn idle, stopp wenn läuft
flow worktime correct 09:30   # Startzeit der laufenden Session fixen
flow worktime brief week      # Wochen-Standup als Markdown
flow worktime export csv      # Sessions exportieren
flow worktime stats month     # Aggregate-Stats
flow worktime status          # tmux status-right Segment
```

### Kompendium

```sh
flow kompendium new daily            # heutige Daily-Note
flow kompendium new project          # Projekt-Note (cwd-aware)
flow kompendium new free <slug>      # freie Notiz
flow kompendium today                # heutige Daily öffnen
flow kompendium ls                   # alle Notizen listen
flow kompendium search "<query>"     # FTS5-Volltextsuche
flow kompendium browse               # TUI-Browser mit Live-Preview
flow kompendium sync                 # gegen das Remote pushen/pullen
flow kompendium doctor               # Notebook-Health-Check
```

Daten-Default: `~/notes` (override mit `$NOTES_DIR`), Index unter
`$XDG_DATA_HOME/kompendium/index.db`.

---

## Konfiguration

Worktime-Verhalten:

| Variable                    | Default     | Bedeutung                                      |
| --------------------------- | ----------- | ---------------------------------------------- |
| `WORKTIME_TARGET_HOURS`     | `8`         | Tagessoll in Stunden                           |
| `WORKTIME_MAX_STREAK_MIN`   | `90`        | Minuten bis die Live-Glyph gelb wird           |
| `WORKTIME_SHOW_WEEKEND`     | unset       | `=1` zeigt Sa/So in der Woche-View             |
| `WORKTIME_POMODORO_MIN`     | unset       | Pomodoro-Länge für die TUI-Overlay             |
| `WORKTIME_LAND`             | `NW`        | Default-Bundesland für Feiertags-Sync          |

Per-Weekday-Targets gehen in `~/.tmux/worktime.conf`:

```ini
target_hours    = 8
target_mon      = 8
target_fri      = 6        # Halber Freitag
tag_target_deep = 4        # Optional: Pro-Tag-Mindestziel
```

Integration:

| Variable            | Default              | Bedeutung                                     |
| ------------------- | -------------------- | --------------------------------------------- |
| `NOTES_DIR`         | `~/notes`            | Kompendium-Notizbuch-Wurzel                   |
| `XDG_DATA_HOME`     | `~/.local/share`     | FTS5-Index unter `<root>/kompendium/index.db` |
| `FLOW_NOTE_VIEWER`  | `glow`               | Markdown-Viewer für `o` auf Heute-Note        |
| `SOURCECODE_ROOT`   | `~/Sourcecode`       | Wurzel für die Projekte-Liste                 |

---

## Architektur

`flow` ist hexagonal organisiert — zwei koexistierende Pyramiden
(flow's eigener Stack + die Kompendium-Subtree) treffen sich nur am
Composition-Root und über klar definierte Cross-Boundary-Ports.

```
cmd/flow/main.go                  Composition-Root (~290 Zeilen Wiring)
internal/
  domain/                         Pure Logik (stdlib only)
  ports/                          Interfaces — keine I/O
  usecase/                        Anwendungsfälle gegen Ports
  adapter/<backend>/              I/O-Implementierungen
  frontend/
    cli/                          cobra-Subcommands
    tui/
      components/                 picker · titlebox · toast · confirm · …
      markdown/                   Geteilter goldmark + chroma-Renderer
      sidekick/                   Bubbletea-Root (5-Tab-Routing)
      screen/{palette,projects,worktime,cheatsheet}/
  testutil/                       In-Memory-Fakes für jeden Port
  kompendium/                     Eigener hexagonaler Subtree
    domain/  ports/  usecase/  adapter/  frontend/
```

Layer-Regeln werden via depguard-Lints erzwungen — jede neue Datei landet
genau in der Schicht, die zu ihrer Verantwortung passt, sonst schlägt
`make lint` an.

Mehr Detail in `CLAUDE.md` (Projektkonventionen) und in den `CLAUDE-*-plan.md`-
Dateien zu den großen Refactor-Wellen (hexagonal, kompendium-integration,
worktime-menu).

---

## Build · Test · Lint

| Target          | Tut                                                                  |
| --------------- | -------------------------------------------------------------------- |
| `make build`    | Binary nach `bin/flow`                                               |
| `make install`  | `go install` nach `~/.local/bin`                                     |
| `make test`     | `go test -race ./...`                                                |
| `make cover`    | Coverage gegen den 87%-Gate                                          |
| `make lint`     | `golangci-lint run` (depguard, errcheck, staticcheck, revive, …)     |
| `make fmt`      | `gofumpt` + `goimports`                                              |
| `make ci`       | `lint cover build` — spiegelt GitHub Actions                         |

Single-Test-Lauf:

```sh
go test -race -run TestName ./internal/usecase/...
```

Cross-Build (Darwin amd64/arm64, Linux amd64) läuft in CI.

---

## Glyphen-Konvention

Die TUI verwendet bewusst nur Monospace-Glyphen, keine Emoji-
Pictogramme — auf jedem Font, in jedem Terminal exakt eine Zelle breit:

```
▶   läuft / start              ✓   ok / Ziel erreicht
●   ○   gefüllt / leer         ★   Feiertag
☼   Urlaub                     ✚   Krank
▲   on track                   ▼   behind
▰   Bar gefüllt                ▱   Bar leer
░ ▒ ▓ █  Heatmap-Stufen
```

---

## tmux-Integration

Beispiel-Bindings für `~/.tmux.conf`:

```tmux
bind -n M-s run-shell "flow worktime toggle"
bind -n M-S run-shell "flow worktime stop"
bind -n M-, run-shell "flow worktime pause"
bind -n M-. run-shell "flow worktime resume"

set -g status-right "#(flow worktime status)"
set -g status-interval 5
```

`flow worktime <verb>` ruft intern `tmux refresh-client -S` auf, sodass
das Status-Segment sofort den neuen Stand zeigt. Mutation-Verbs sind
**idempotent** (Stop bei kein-Lauf ist No-op statt Fehler) — perfekt
für blinde tmux-Bindings.

---

## Lizenz

[MIT](LICENSE) — siehe `LICENSE` für den vollen Text.
