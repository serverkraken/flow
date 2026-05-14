# flow

> A TUI sidekick for tmux — Worktime-Tracker, Markdown Notebook, and Command Palette in one binary.

## Project Overview

`flow` is a developer-centric productivity tool designed to live as a persistent pane in tmux. It follows a **Hexagonal Architecture** (Ports and Adapters) to ensure maintainability, testability, and clear separation of concerns.

### Key Features
- **Worktime Tracker:** Session tracking, daily/weekly goals, history heatmap, and status reporting for the tmux status bar.
- **Kompendium:** A markdown-based notebook with FTS5 full-text search, wikilinks, and Git-backed synchronization.
- **Palette:** A universal command launcher with fuzzy filtering.
- **Projects:** Fast project switching within tmux sessions.
- **Cheatsheet:** Instant access to markdown-based cheatsheets.

### Tech Stack
- **Language:** Go 1.25+ (Pure Go, no CGO).
- **CLI Framework:** [Cobra](https://github.com/spf13/cobra).
- **TUI Framework:** [Bubbletea](https://github.com/charmbracelet/bubbletea), [Lipgloss](https://github.com/charmbracelet/lipgloss).
- **Storage:** Plain text (TSV/Markdown) for most data, SQLite (FTS5) for notebook indexing.
- **Integration:** Deeply integrated with **tmux** for session management and UI display.

---

## Architecture & Dependency Rules

The project strictly adheres to hexagonal layers (enforced via `depguard` lints):

| Layer | Path | Dependency Rules | Responsibility |
| :--- | :--- | :--- | :--- |
| **Domain** | `internal/domain` | Stdlib only (no I/O, no `time.Now()`) | Pure logic, value types, and parsers. |
| **Ports** | `internal/ports` | Domain + Stdlib only | Interface definitions for all external interactions. |
| **Use Cases** | `internal/usecase` | Domain + Ports + Stdlib | Orchestrating application logic against ports. |
| **Adapters** | `internal/adapter` | Domain + Ports + External Libs | I/O implementations (Filesystem, Tmux, SQLite). |
| **Frontend** | `internal/frontend` | Domain + Ports + Usecases + TUI Libs | CLI subcommands and TUI screens. |
| **Composition** | `cmd/flow/main.go` | Everything | Wiring the dependency graph and launching the app. |

*Note: `kompendium` (the notebook) is its own hexagonal subtree within `internal/kompendium`.*

---

## Development Workflow

### Key Commands

| Task | Command |
| :--- | :--- |
| **Build** | `make build` (Binary at `bin/flow`) |
| **Install** | `make install` (Installs to `~/.local/bin/flow`) |
| **Test** | `make test` (Runs `go test -race ./...`) |
| **Lint** | `make lint` (Runs `golangci-lint`) |
| **Coverage** | `make cover` (Enforces **85%** threshold via `scripts/coverage-gate.sh`) |
| **CI Check** | `make ci` (Run this before pushing: lint, cover, build) |

### Continuous Integration
The project uses GitHub Actions (`.github/workflows/`) for CI and CD.
- **Linting:** Enforces strict rules including `depguard` for layer separation.
- **Testing:** Runs full suite with race detection and coverage gating.
- **Releases:** Automated via [release-please](https://github.com/googleapis/release-please) and [GoReleaser](https://goreleaser.com) using **Conventional Commits**.

---

## Coding Conventions

### 1. Hexagonal Integrity
- Never import `internal/adapter` or `internal/usecase` from `internal/domain`.
- Never import `internal/adapter` from `internal/usecase`.
- Use `internal/testutil` for in-memory fakes when testing usecases.
- Every port implementation in `testutil` should have a compile-time assertion: `var _ ports.X = (*FakeX)(nil)`.

### 2. TUI Design (Bubbletea/Lipgloss)
- **Monospace Only:** Use only ASCII/Monospace glyphs (e.g., `▶`, `✓`, `▰`). No emojis.
- **Style Caches:** Use per-model style caches (e.g., `newPaletteStyles(p theme.Palette)`) to avoid allocations in `View()` hot paths.
- **Theme Support:** TUI themes are aware of tmux options. User overrides can be set via tmux global options `@tn_*`.
- **TrueColor:** The app expects a TrueColor terminal (Ghostty, iTerm2, Alacritty, etc.).

### 3. Data Integrity & Concurrency
- **Atomic Operations:** Use atomic batches (e.g., `Store.AddBatch`) for filesystem writes to prevent data corruption.
- **Context Handling:** Always thread `context.Context` through I/O operations and use timeouts (e.g., `context.WithTimeout`) for SQLite or potentially blocking operations.
- **Thread Safety:** Use `atomic.Pointer` for frequently accessed global state like TUI themes.

### 4. Commits & Versioning
Follow [Conventional Commits](https://www.conventionalcommits.org/):
- `feat:` for new features (triggers Minor bump).
- `fix:` for bug fixes (triggers Patch bump).
- `feat!:` or `fix!:` for breaking changes (triggers Major bump).
- `chore:`, `docs:`, `test:`, `ci:` for internal maintenance (no bump).

---

## Key Files & Locations
- `cmd/flow/main.go`: The composition root. Check here to see how dependencies are wired.
- `internal/frontend/cli/`: Definition of all Cobra subcommands.
- `internal/frontend/tui/sidekick/`: The main TUI entry point and tab routing.
- `internal/frontend/tui/theme/`: Theme loading and palette definitions.
- `scripts/coverage-gate.sh`: Script that enforces the coverage threshold.
- `CLAUDE-activeContext.md`: Documents the current state of development and recent changes (often ignored by git).
