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

## Voraussetzungen

| Komponente            | Pflicht | Wofür                                                              |
| --------------------- | ------- | ------------------------------------------------------------------ |
| **Go 1.25+**          | Ja      | Build (Pure-Go-Binary, kein cgo, kein C-Compiler nötig)            |
| **tmux**              | empf.   | Sidekick-Pane, Status-Right-Segment, Aktions-Menü-Output-Target    |
| **git**               | empf.   | Kompendium-Sync, Snapshot-Adapter                                  |
| **glow**              | optional| Markdown-Render im tmux-Split (Brief / Note-Viewer)                |
| **less**              | optional| Pager-Fallback für CSV/JSON/Stats im tmux-Split                    |
| **nvim** / `$EDITOR`  | optional| Kompendium-Note-Edit (Default `nvim`; `$VISUAL`/`$EDITOR` greifen) |
| **pbcopy** / **xclip**| optional| Aktions-Menü Clipboard-Target (macOS / Linux)                      |
| **TrueColor-Terminal**| empf.   | Lipgloss-Themes erwarten 24-Bit-Farbe (Ghostty, iTerm2, Alacritty) |

Standalone-CLI (`flow worktime stop`, `flow worktime brief`, …) läuft
auch ohne tmux. Nur das interaktive TUI macht ohne tmux wenig Sinn,
weil sich Sidekick und Status-Bar gegenseitig brauchen.

## Installation

### macOS (Homebrew)

```sh
brew install go tmux glow neovim git
git clone https://github.com/serverkraken/flow.git
cd flow
make install            # → ~/.local/bin/flow
```

`pbcopy` / `pbpaste` sind auf macOS Bordmittel — kein extra Install.
Stelle sicher, dass `~/.local/bin` im `PATH` liegt:

```sh
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
```

### Debian / Ubuntu

`apt`s Go-Paket hinkt in der Regel mehrere Versionen hinterher; für
Go 1.25 die offizielle Binary von `go.dev/dl` ziehen oder einen
Version-Manager (`mise` / `asdf`) verwenden.

```sh
# Laufzeit-Tools aus apt — alle pflichtfrei außer git
sudo apt update
sudo apt install -y tmux git glow less xclip neovim

# Go 1.25 offiziell
GO_VERSION=1.25.0
curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf /tmp/go.tar.gz
echo 'export PATH="/usr/local/go/bin:$HOME/go/bin:$HOME/.local/bin:$PATH"' >> ~/.bashrc
exec $SHELL -l

# Build
git clone https://github.com/serverkraken/flow.git
cd flow
make install            # → ~/.local/bin/flow
```

Falls `glow` in Deinem Debian-Release noch nicht in apt liegt, gibt es
das Charm-APT-Repo (`https://github.com/charmbracelet/charm/blob/main/INSTALLING.md`)
oder ein `go install github.com/charmbracelet/glow@latest`.

### Build-Targets

| Target          | Tut                                                       |
| --------------- | --------------------------------------------------------- |
| `make build`    | Binary nach `bin/flow` (lokal, kein Install)              |
| `make install`  | `go install` nach `~/.local/bin/flow`                     |
| `go build -o /irgendwo/flow ./cmd/flow` | Manueller Build mit eigenem Pfad      |

Cross-Compile (z.B. Linux-Binary von macOS aus):

```sh
GOOS=linux   GOARCH=amd64 go build -o bin/flow-linux-amd64  ./cmd/flow
GOOS=darwin  GOARCH=arm64 go build -o bin/flow-darwin-arm64 ./cmd/flow
GOOS=darwin  GOARCH=amd64 go build -o bin/flow-darwin-amd64 ./cmd/flow
```

CGO ist nicht erforderlich (`modernc.org/sqlite` ist Pure-Go) — der
Cross-Compile-Build braucht keinen C-Toolchain für die Zielarchitektur.

### Verifizieren

```sh
flow --help              # Subcommand-Übersicht
flow worktime status     # tmux status-right Segment auf stdout
flow kompendium doctor   # Notebook-Health-Check
```

`flow sidekick` startet die TUI direkt; üblicher ist sie als tmux-Pane
über die Bindings im Abschnitt [tmux-Integration](#tmux-integration).

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

## Releases

Versionierung ist automatisch — der Release-Workflow nutzt
[release-please](https://github.com/googleapis/release-please) und
[GoReleaser](https://goreleaser.com).

**Conventional-Commit-Flow:**

```
feat:     → Minor-Bump (0.1.0 → 0.2.0)
fix:      → Patch-Bump (0.1.0 → 0.1.1)
feat!:    → Major-Bump
chore: ci: docs: test:    → kein Bump (im Hidden-Changelog)
```

**Was beim Push auf main passiert:**

1. **release-please** liest neue Conventional-Commits seit dem letzten
   Tag und öffnet (oder aktualisiert) eine Release-PR mit dem
   nächsten SemVer + auto-generiertem Changelog.
2. Beim **Mergen der Release-PR**: ein Tag `vX.Y.Z` wird gepusht und
   eine GitHub-Release mit den Notes aus dem Changelog angelegt.
3. **GoReleaser** läuft danach im selben Workflow, baut Cross-
   Plattform-Binaries (linux/amd64, darwin/{amd64,arm64}) und hängt
   sie + `checksums.txt` an die Release.

Releases liegen unter `Releases` im GitHub-Repo. Install ohne Go-
Toolchain — Archiv-Name ist über alle Versionen stabil, das `latest`-
Redirect resolved automatisch:

```sh
# macOS Apple Silicon
curl -fsSL https://github.com/serverkraken/flow/releases/latest/download/flow_darwin_arm64.tar.gz \
  | tar -xz && sudo install flow /usr/local/bin/

# macOS Intel
curl -fsSL https://github.com/serverkraken/flow/releases/latest/download/flow_darwin_amd64.tar.gz \
  | tar -xz && sudo install flow /usr/local/bin/

# Linux amd64
curl -fsSL https://github.com/serverkraken/flow/releases/latest/download/flow_linux_amd64.tar.gz \
  | tar -xz && sudo install flow /usr/local/bin/
```

Eine bestimmte Version pinnen geht über `releases/download/vX.Y.Z/`:

```sh
curl -fsSL https://github.com/serverkraken/flow/releases/download/v0.1.0/flow_darwin_arm64.tar.gz \
  | tar -xz
```

Checksums liegen unter `checksums.txt` in jeder Release. Verify:

```sh
curl -fsSL https://github.com/serverkraken/flow/releases/latest/download/checksums.txt -o checksums.txt
sha256sum -c checksums.txt --ignore-missing
```

---

## Lizenz

[MIT](LICENSE) — siehe `LICENSE` für den vollen Text.
