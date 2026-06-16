# flow rebuild M3a — Design-System-Revival Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Das auf `main` (Pre-Rebuild) bewährte TUI-Design-System (`theme` + Komponenten) in den `rebuild` portieren — als domain-freie, getestete Grundlage für die Sidekick-Shell (M3b) und die Screen-Ports (M3c/M3d). Keine Screen-Änderungen in M3a.

**Architecture:** Reiner **Port**. Die Pakete sind präsentations-pur und laufen schon auf charm-v2 — also **verbatim kopieren** (`git show main:<pfad>`) und nur die **Import-Pfade umschreiben** (`internal/frontend/tui/components` → `internal/tui/ui`, `internal/frontend/tui/theme` → `internal/tui/theme`). Die zwei domain-gekoppelten Theme-Dateien (`kind_color.go`, `status_adapter.go`) bleiben für M3d (DayOffs) liegen — die Theme-Core ist nachweislich domain-frei. Reihenfolge folgt dem Abhängigkeitsgraph (Blätter zuerst).

**Tech Stack:** Go, charm.land/lipgloss/v2 (vorhanden), charm.land/bubbletea/v2 (vorhanden), **charm.land/bubbles/v2 v2.1.0 (NEU hinzuzufügen** — `form`/`confirm` brauchen `textinput`/`key`).

**Spec:** `docs/superpowers/specs/2026-06-16-flow-rebuild-m3-tui-design-system-shell.md`

---

## Port-Methodik (für JEDEN Port-Task gleich — bitte exakt so)

Dies ist ein **verbatim-Port**. Den Code **nicht** neu tippen oder „verbessern" — exakt kopieren, nur Imports umschreiben. Pro Paket:

1. **Kopieren** (kopiert `.go` **und** `_test.go` mit):
   ```bash
   mkdir -p <ZIEL_DIR>
   for f in $(git ls-tree -r --name-only main -- <QUELL_DIR> | rg '\.go$'); do
     git show "main:$f" > "<ZIEL_DIR>/$(basename "$f")"
   done
   ```
2. **Imports umschreiben** (macOS-`sed`, beide Rewrites in einem Pass):
   ```bash
   grep -rl 'internal/frontend/tui' <ZIEL_DIR> | xargs sed -i '' \
     -e 's#serverkraken/flow/internal/frontend/tui/components#serverkraken/flow/internal/tui/ui#g' \
     -e 's#serverkraken/flow/internal/frontend/tui/theme#serverkraken/flow/internal/tui/theme#g'
   ```
3. **Testen** (Paket grün) → 4. **Committen**.

Wenn ein Test-Hilfsdatei-Pfad oder ein Paketname nach dem Rewrite nicht passt, ist das ein echter Fehler — melden, nicht raten.

---

## File map

| Paket (Quelle `main:internal/frontend/tui/…`) | Ziel | Abhängt von |
|---|---|---|
| `components/glyphs` | `internal/tui/ui/glyphs` | — |
| `components/strings` | `internal/tui/ui/strings` | — |
| `theme` (ohne `kind_color.go`,`status_adapter.go`) | `internal/tui/theme` | lipgloss |
| `components/titlebox` | `internal/tui/ui/titlebox` | strings, theme |
| `components/statusbar` | `internal/tui/ui/statusbar` | glyphs, theme |
| `components/toast` | `internal/tui/ui/toast` | glyphs, theme |
| `components/help` | `internal/tui/ui/help` | titlebox, theme |
| `components/confirm` | `internal/tui/ui/confirm` | strings, theme, bubbles/key |
| `components/form` | `internal/tui/ui/form` | theme, bubbles/textinput |
| `components/picker` | `internal/tui/ui/picker` | glyphs, strings, theme |
| `internal/tui/ui/uidemo/render_test.go` | **neu** | alle (Smoke) |

Reihenfolge ist so gewählt, dass nach jedem Task gebaut + getestet werden kann.

---

## Task 1: glyphs (Blatt, keine Deps)

**Files:**
- Create: `internal/tui/ui/glyphs/*.go` (Port von `main:internal/frontend/tui/components/glyphs`)

- [ ] **Step 1: Kopieren**
```bash
mkdir -p internal/tui/ui/glyphs
for f in $(git ls-tree -r --name-only main -- internal/frontend/tui/components/glyphs | rg '\.go$'); do
  git show "main:$f" > "internal/tui/ui/glyphs/$(basename "$f")"
done
```

- [ ] **Step 2: Imports umschreiben** (glyphs hat keine internen Imports — der `grep`/`sed` ist ein No-op, trotzdem ausführen für Konsistenz)
```bash
grep -rl 'internal/frontend/tui' internal/tui/ui/glyphs | xargs sed -i '' \
  -e 's#serverkraken/flow/internal/frontend/tui/components#serverkraken/flow/internal/tui/ui#g' \
  -e 's#serverkraken/flow/internal/frontend/tui/theme#serverkraken/flow/internal/tui/theme#g' 2>/dev/null || true
```

- [ ] **Step 3: Testen**
Run: `go test ./internal/tui/ui/glyphs/ -v`
Expected: PASS (`glyphs_test.go` prüft die Whitelist).

- [ ] **Step 4: Commit**
```bash
git add internal/tui/ui/glyphs
git commit -m "feat(m3a): port glyphs design-token package"
```

---

## Task 2: strings (Blatt, keine Deps)

**Files:**
- Create: `internal/tui/ui/strings/*.go`

- [ ] **Step 1: Kopieren**
```bash
mkdir -p internal/tui/ui/strings
for f in $(git ls-tree -r --name-only main -- internal/frontend/tui/components/strings | rg '\.go$'); do
  git show "main:$f" > "internal/tui/ui/strings/$(basename "$f")"
done
```

- [ ] **Step 2: Imports umschreiben**
```bash
grep -rl 'internal/frontend/tui' internal/tui/ui/strings | xargs sed -i '' \
  -e 's#serverkraken/flow/internal/frontend/tui/components#serverkraken/flow/internal/tui/ui#g' \
  -e 's#serverkraken/flow/internal/frontend/tui/theme#serverkraken/flow/internal/tui/theme#g' 2>/dev/null || true
```

- [ ] **Step 3: Testen**
Run: `go test ./internal/tui/ui/strings/ -v`
Expected: PASS (`strings_test.go`).

- [ ] **Step 4: Commit**
```bash
git add internal/tui/ui/strings
git commit -m "feat(m3a): port ui strings helpers"
```

---

## Task 3: theme core (ohne kind_color/status_adapter — domain-frei)

**Files:**
- Create: `internal/tui/theme/{palette,semantic,tokens,builders,contrast,pill,load}.go` + Tests `{builders,contrast,palette,pill}_test.go`
- **NICHT** portieren: `kind_color.go`, `status_adapter.go`, `kind_color_test.go`, `status_adapter_test.go` (domain-gekoppelt → M3d).

- [ ] **Step 1: Kern-Dateien kopieren**
```bash
mkdir -p internal/tui/theme
for f in palette semantic tokens builders contrast pill load; do
  git show "main:internal/frontend/tui/theme/$f.go" > "internal/tui/theme/$f.go"
done
for f in builders contrast palette pill; do
  git show "main:internal/frontend/tui/theme/${f}_test.go" > "internal/tui/theme/${f}_test.go"
done
```

- [ ] **Step 2: Imports umschreiben**
```bash
grep -rl 'internal/frontend/tui' internal/tui/theme | xargs sed -i '' \
  -e 's#serverkraken/flow/internal/frontend/tui/components#serverkraken/flow/internal/tui/ui#g' \
  -e 's#serverkraken/flow/internal/frontend/tui/theme#serverkraken/flow/internal/tui/theme#g' 2>/dev/null || true
```

- [ ] **Step 3: Domain-Freiheit + Build verifizieren**
Run: `rg -n 'internal/domain|domain\.' internal/tui/theme/ ; go build ./internal/tui/theme/`
Expected: **kein** `domain`-Treffer; Build **clean**. Falls der Build eine fehlende `KindColor`/`StatusPaletteFor`/`domain.Kind`-Referenz meldet, referenziert eine Kern-Datei doch die ausgelassenen Files — dann den Aufrufer melden (sollte laut Analyse nicht passieren).

- [ ] **Step 4: Testen**
Run: `go test ./internal/tui/theme/ -v`
Expected: PASS (`palette_test.go` nutzt `theme.Themes`/`theme.TokyonightNight`; `builders/contrast/pill` grün).

- [ ] **Step 5: Commit**
```bash
git add internal/tui/theme
git commit -m "feat(m3a): port theme core (palette/semantic/tokens/builders/contrast/pill/load), domain-free"
```

---

## Task 4: titlebox (Deps: strings, theme)

**Files:**
- Create: `internal/tui/ui/titlebox/*.go`

- [ ] **Step 1: Kopieren**
```bash
mkdir -p internal/tui/ui/titlebox
for f in $(git ls-tree -r --name-only main -- internal/frontend/tui/components/titlebox | rg '\.go$'); do
  git show "main:$f" > "internal/tui/ui/titlebox/$(basename "$f")"
done
```

- [ ] **Step 2: Imports umschreiben**
```bash
grep -rl 'internal/frontend/tui' internal/tui/ui/titlebox | xargs sed -i '' \
  -e 's#serverkraken/flow/internal/frontend/tui/components#serverkraken/flow/internal/tui/ui#g' \
  -e 's#serverkraken/flow/internal/frontend/tui/theme#serverkraken/flow/internal/tui/theme#g'
```

- [ ] **Step 3: Testen**
Run: `go test ./internal/tui/ui/titlebox/ -v`
Expected: PASS (`titlebox_test.go`, `edge_test.go`).

- [ ] **Step 4: Commit**
```bash
git add internal/tui/ui/titlebox
git commit -m "feat(m3a): port titlebox component"
```

---

## Task 5: statusbar (Deps: glyphs, theme) — enthält BarColored

**Files:**
- Create: `internal/tui/ui/statusbar/*.go`

- [ ] **Step 1: Kopieren**
```bash
mkdir -p internal/tui/ui/statusbar
for f in $(git ls-tree -r --name-only main -- internal/frontend/tui/components/statusbar | rg '\.go$'); do
  git show "main:$f" > "internal/tui/ui/statusbar/$(basename "$f")"
done
```

- [ ] **Step 2: Imports umschreiben**
```bash
grep -rl 'internal/frontend/tui' internal/tui/ui/statusbar | xargs sed -i '' \
  -e 's#serverkraken/flow/internal/frontend/tui/components#serverkraken/flow/internal/tui/ui#g' \
  -e 's#serverkraken/flow/internal/frontend/tui/theme#serverkraken/flow/internal/tui/theme#g'
```

- [ ] **Step 3: Testen**
Run: `go test ./internal/tui/ui/statusbar/ -v`
Expected: PASS (`statusbar_test.go` testet `Bar(0/50/100/-10/200,…)`; `hints_test.go`). Damit ist **`BarColored` (▰▱, schwellen-farbig)** im Rebuild.

- [ ] **Step 4: Commit**
```bash
git add internal/tui/ui/statusbar
git commit -m "feat(m3a): port statusbar (BarColored progress + hints)"
```

---

## Task 6: toast (Deps: glyphs, theme)

**Files:**
- Create: `internal/tui/ui/toast/*.go`

- [ ] **Step 1: Kopieren**
```bash
mkdir -p internal/tui/ui/toast
for f in $(git ls-tree -r --name-only main -- internal/frontend/tui/components/toast | rg '\.go$'); do
  git show "main:$f" > "internal/tui/ui/toast/$(basename "$f")"
done
```

- [ ] **Step 2: Imports umschreiben**
```bash
grep -rl 'internal/frontend/tui' internal/tui/ui/toast | xargs sed -i '' \
  -e 's#serverkraken/flow/internal/frontend/tui/components#serverkraken/flow/internal/tui/ui#g' \
  -e 's#serverkraken/flow/internal/frontend/tui/theme#serverkraken/flow/internal/tui/theme#g'
```

- [ ] **Step 3: Testen**
Run: `go test ./internal/tui/ui/toast/ -v`
Expected: PASS (`toast_test.go`, `slot_test.go`).

- [ ] **Step 4: Commit**
```bash
git add internal/tui/ui/toast
git commit -m "feat(m3a): port toast component (Success/Info, slot rows)"
```

---

## Task 7: help (Deps: titlebox, theme)

**Files:**
- Create: `internal/tui/ui/help/*.go`

- [ ] **Step 1: Kopieren**
```bash
mkdir -p internal/tui/ui/help
for f in $(git ls-tree -r --name-only main -- internal/frontend/tui/components/help | rg '\.go$'); do
  git show "main:$f" > "internal/tui/ui/help/$(basename "$f")"
done
```

- [ ] **Step 2: Imports umschreiben**
```bash
grep -rl 'internal/frontend/tui' internal/tui/ui/help | xargs sed -i '' \
  -e 's#serverkraken/flow/internal/frontend/tui/components#serverkraken/flow/internal/tui/ui#g' \
  -e 's#serverkraken/flow/internal/frontend/tui/theme#serverkraken/flow/internal/tui/theme#g'
```

- [ ] **Step 3: Testen**
Run: `go test ./internal/tui/ui/help/ -v`
Expected: PASS (`help_test.go`).

- [ ] **Step 4: Commit**
```bash
git add internal/tui/ui/help
git commit -m "feat(m3a): port help overlay component"
```

---

## Task 8: bubbles/v2-Dependency + confirm (Deps: strings, theme, bubbles/key)

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `internal/tui/ui/confirm/*.go`

- [ ] **Step 1: bubbles/v2 hinzufügen** (Version exakt wie auf `main`: v2.1.0)
```bash
go get charm.land/bubbles/v2@v2.1.0
```
Expected: `go.mod` bekommt `charm.land/bubbles/v2 v2.1.0` als direkten Require.

- [ ] **Step 2: confirm kopieren**
```bash
mkdir -p internal/tui/ui/confirm
for f in $(git ls-tree -r --name-only main -- internal/frontend/tui/components/confirm | rg '\.go$'); do
  git show "main:$f" > "internal/tui/ui/confirm/$(basename "$f")"
done
```

- [ ] **Step 3: Imports umschreiben**
```bash
grep -rl 'internal/frontend/tui' internal/tui/ui/confirm | xargs sed -i '' \
  -e 's#serverkraken/flow/internal/frontend/tui/components#serverkraken/flow/internal/tui/ui#g' \
  -e 's#serverkraken/flow/internal/frontend/tui/theme#serverkraken/flow/internal/tui/theme#g'
```

- [ ] **Step 4: Testen**
Run: `go test ./internal/tui/ui/confirm/ -v`
Expected: PASS (`confirm_test.go`, `init_test.go`).

- [ ] **Step 5: Commit**
```bash
git add go.mod go.sum internal/tui/ui/confirm
git commit -m "feat(m3a): add bubbles/v2 dep + port confirm dialog component"
```

---

## Task 9: form (Deps: theme, bubbles/textinput)

**Files:**
- Create: `internal/tui/ui/form/*.go`

- [ ] **Step 1: Kopieren**
```bash
mkdir -p internal/tui/ui/form
for f in $(git ls-tree -r --name-only main -- internal/frontend/tui/components/form | rg '\.go$'); do
  git show "main:$f" > "internal/tui/ui/form/$(basename "$f")"
done
```

- [ ] **Step 2: Imports umschreiben**
```bash
grep -rl 'internal/frontend/tui' internal/tui/ui/form | xargs sed -i '' \
  -e 's#serverkraken/flow/internal/frontend/tui/components#serverkraken/flow/internal/tui/ui#g' \
  -e 's#serverkraken/flow/internal/frontend/tui/theme#serverkraken/flow/internal/tui/theme#g'
```

- [ ] **Step 3: Testen**
Run: `go test ./internal/tui/ui/form/ -v`
Expected: PASS (`textinput_test.go`).

- [ ] **Step 4: Commit**
```bash
git add internal/tui/ui/form
git commit -m "feat(m3a): port form/textinput component"
```

---

## Task 10: picker (Deps: glyphs, strings, theme)

**Files:**
- Create: `internal/tui/ui/picker/*.go`

- [ ] **Step 1: Kopieren**
```bash
mkdir -p internal/tui/ui/picker
for f in $(git ls-tree -r --name-only main -- internal/frontend/tui/components/picker | rg '\.go$'); do
  git show "main:$f" > "internal/tui/ui/picker/$(basename "$f")"
done
```

- [ ] **Step 2: Imports umschreiben**
```bash
grep -rl 'internal/frontend/tui' internal/tui/ui/picker | xargs sed -i '' \
  -e 's#serverkraken/flow/internal/frontend/tui/components#serverkraken/flow/internal/tui/ui#g' \
  -e 's#serverkraken/flow/internal/frontend/tui/theme#serverkraken/flow/internal/tui/theme#g'
```

- [ ] **Step 3: Testen**
Run: `go test ./internal/tui/ui/picker/ -v`
Expected: PASS (`picker_test.go`, `row_test.go`, `row_edge_test.go`).

- [ ] **Step 4: Commit**
```bash
git add internal/tui/ui/picker
git commit -m "feat(m3a): port picker (section header + selectable rows)"
```

---

## Task 11: Design-System-Smoke-Test (Done-Gate „kleiner Render")

Beweist, dass die portierten Teile **zusammen** komponieren (neue, nicht portierte Datei).

**Files:**
- Create: `internal/tui/ui/uidemo/doc.go` (Paket-Klausel, damit `go build ./...` kein „no non-test Go files" meldet)
- Create: `internal/tui/ui/uidemo/render_test.go`

- [ ] **Step 0: Paket-Datei anlegen**
```go
// Package uidemo holds composition smoke tests for the M3a design-system
// packages. It has no production code — only the tests verify that theme,
// glyphs and statusbar render together after the port.
package uidemo
```
(als `internal/tui/ui/uidemo/doc.go`)

- [ ] **Step 1: Test schreiben**
```go
package uidemo

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
	"github.com/serverkraken/flow/internal/tui/ui/statusbar"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// TestDesignSystemComposes rendert die zentrale Progress-Primitive mit der
// geladenen Palette und prüft, dass die kanonischen Bar-Glyphen erscheinen —
// ein Rauchtest, dass theme + glyphs + statusbar nach dem Port zusammenspielen.
func TestDesignSystemComposes(t *testing.T) {
	p := theme.Load()

	full := statusbar.BarColored(100, 10, p.Sem().Success, p)
	if !strings.Contains(full, glyphs.BarFilled) {
		t.Fatalf("100%% bar should contain BarFilled %q: %q", glyphs.BarFilled, full)
	}
	if strings.Contains(full, glyphs.BarEmpty) {
		t.Fatalf("100%% bar should have no empty cells: %q", full)
	}

	half := statusbar.BarColored(50, 10, p.Sem().Active, p)
	if !strings.Contains(half, glyphs.BarFilled) || !strings.Contains(half, glyphs.BarEmpty) {
		t.Fatalf("50%% bar should mix filled+empty: %q", half)
	}

	empty := statusbar.BarColored(0, 10, p.Sem().Active, p)
	if strings.Contains(empty, glyphs.BarFilled) {
		t.Fatalf("0%% bar should have no filled cells: %q", empty)
	}
}
```

- [ ] **Step 2: Testen**
Run: `go test ./internal/tui/ui/uidemo/ -v`
Expected: PASS. (Falls `p.Sem().Success`/`.Active` anders heißen, in `internal/tui/theme/semantic.go` den echten Feldnamen prüfen und anpassen — die Felder existieren, nur der Name ist zu verifizieren.)

- [ ] **Step 3: Commit**
```bash
git add internal/tui/ui/uidemo/doc.go internal/tui/ui/uidemo/render_test.go
git commit -m "test(m3a): design-system composition smoke test"
```

---

## Task 12: Volle CI + Coverage-Gate

**Files:** keine (nur Verifikation).

- [ ] **Step 1: Gesamtes Modul bauen + vetten**
Run: `go build ./... && go vet ./internal/tui/...`
Expected: clean. (M3a ist additiv — bestehende Screens/`internal/tui/*.go` bleiben unberührt und kompilieren weiter.)

- [ ] **Step 2: `make ci`**
Run: `make ci`
Expected: `lint`, `verify-generate`, `cover` (≥ 80 %), `build` grün. Die portierten Pakete bringen ihre eigenen Tests mit — Coverage sollte eher steigen. Falls `golangci-lint` an portiertem Code mäkelt (z.B. ungenutzter Parameter), das im **portierten** File minimal beheben (kein Logik-Umbau) und im selben Commit nachziehen.

- [ ] **Step 3: Abschluss-Commit (falls Lint-Fixups nötig waren)**
```bash
git add -A && git commit -m "chore(m3a): lint fixups for ported design-system packages"
```

---

## Notes for the implementer

- **Verbatim-Port, kein Redesign.** Der einzige erlaubte Edit am kopierten Code ist der Import-Pfad-Rewrite (+ ggf. ein minimaler Lint-Fixup in Task 12). Logik, Kommentare, Tests bleiben wie auf `main`.
- **Ausgelassen (bewusst, → M3d):** `theme/kind_color.go`, `theme/status_adapter.go` (+ deren Tests) — sie koppeln an `domain.Kind`/`StatusPalette`, die im Rebuild erst beim DayOffs-Screen verdrahtet werden.
- **Nicht in M3a:** `components/modal`, `components/titlebox` ist drin (help braucht es), aber `modal` + `markdown_overlay` kommen in M3b/M3d. `header`/`tabstrip`/`breadcrumb`/`overlay` sind NEU und gehören zu M3b.
- **Additiv:** M3a fasst die bestehenden `internal/tui/*.go`-Screens nicht an; die laufen unverändert weiter, bis M3c/M3d sie auf das neue System umstellen.
- [[feedback_subagent_git_commits_isolated]]: nach jedem Subagent-Commit HEAD prüfen; verwaiste Commits via reflog einsammeln.
```
