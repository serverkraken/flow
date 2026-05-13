# Spec — Filled Pace-Dots with Distinct Per-Kind Colors (supersedes 2026-05-12)

**Status:** Implemented (2026-05-13).
**Supersedes:** `2026-05-12-unified-dayoff-glyphs-design.md` (in-tree, kept as historical context).
**Touches:** `internal/domain/status.go`, `internal/usecase/status_composer.go`, related `_test.go`. No changes required in `dotfiles` (`@tn_*` user-options stay optional defaults).

## Why this revisits the May-12 decision

The May-12 spec unified the day-off glyph to `○` (outlined circle) across the tmux pace-dot row and the TUI pace-strip, with kind carried by colour (`Sem.Info` / `Sem.Success` / `Sem.Warning`). The spec table also explicitly considered **Option C — filled `●` for free days** and rejected it:

> Option C (gefüllter Kreis für freie Tage) — verworfen: Doppelbedeutung von `●` (Ziel erreicht + Tag abgegolten)

Six months of real use proved this was the wrong tradeoff *for the tmux bar specifically*. Soenne's report (verbatim):

> "Die ungefüllten 'frei' Kreise sind nicht von den anderen 'ungefüllten' Kreisen für 'arbeit noch nicht begonnen' zu unterscheiden. Die Farbe hat nicht so einen krassen Kontrast. Und auch untereinander unterscheiden sich die freien Tage so doll."

Two collisions in the bar at status-font size:

1. **Free-day `○` vs missed/future-workday `○`** — both outlined; thin glyph means colour signal is too weak.
2. **Free-day `○`s among themselves** — Cyan/Green/Yellow on a 1-cell outlined glyph all read as "faintly tinted outline" in peripheral vision.

The TUI pace-strip is a different rendering surface (larger glyphs, different terminal context) — this spec leaves the TUI behaviour untouched. Apply the same reasoning there if/when in-TUI contrast is reported as inadequate.

## Decision

### Glyph carries shape-of-status, colour carries kind

```
●  this day is "accounted for" — workday target met OR scheduled day off
○  workday open / future / missed — nothing happened (yet)
```

The May-12 "double meaning of `●`" concern dissolves under this framing: filled = "the day has a known disposition", colour says *which* disposition. Outline = "no disposition yet". The shape distinction does the bulk of the visual work; colour disambiguates the filled siblings.

### Final mapping

| State                                  | Glyph | Colour       | Token       |
| -------------------------------------- | ----- | ------------ | ----------- |
| Workday target met                     | `●`   | Green        | `pal.Green`  |
| Today running, target not yet hit      | `●`   | Cyan         | `pal.Cyan`   |
| Workday open / future / missed         | `○`   | Dim          | `pal.Dim`    |
| Holiday (`KindHoliday`)                | `●`   | **Yellow**   | `pal.Yellow` |
| Vacation (`KindVacation`)              | `●`   | **Purple**   | `pal.Purple` |
| Sick (`KindSick`)                      | `●`   | **Orange**   | `pal.Orange` |
| Unknown kind                           | `●`   | Dim          | `pal.Dim`    |
| Banner `[Frei: …]` prefix today        | `●`   | per-kind     | as above    |

Six visually distinct dot states in the strip; the banner uses the same per-kind mapping.

### Side effects

- **Today-running dot** moves Yellow → Cyan to align with the `tui-usability` skill's §Color semantics (Cyan = Active/running/live). This also frees Yellow to be the unambiguous Holiday colour without colliding with the running-today dot.
- **Holiday** moves Cyan → Yellow. Cyan was a misuse per §Color semantics (Cyan = active/running, not a passive scheduled off-day).
- **Vacation** moves Green → Purple to avoid the `●` Green collision with target-met workdays. Purple = Identity per §Color semantics ("you chose this day off") fits the semantic exactly.
- **Sick** moves Yellow → Orange. The May-12 spec already noted the TUI rendered Sick in Orange (`Sem.Warning`) via `StatusPaletteFor`; the bar now matches that intent.

### Palette extension

`StatusPalette` (`internal/domain/status.go`) gains two slots aligned with Tokyonight Storm:

| Slot   | Hex       | Used by                                    |
| ------ | --------- | ------------------------------------------ |
| Purple | `#bb9af7` | Vacation pace-dot + banner                 |
| Orange | `#ff9e64` | Sick pace-dot + banner; banner over-streak |

`StatusComposer.palette()` resolves both via the standard `pick("tn_purple", def.Purple)` / `pick("tn_orange", def.Orange)` pattern. No changes to dotfiles `.tmux.conf` are required — the user-options stay optional, defaults always apply when unset.

## Tests updated

- `TestBuildPaceDots_HitGreenMissedDimRunningYellow` → renamed to `…RunningCyan`; running-today expectation switched.
- `TestBuildPaceDots_DayOffGlyphPerKind` → expected glyph `○` → `●`; per-kind colour expectations updated.
- `TestBuildStatusSegment_DayOffBannerGlyphPerKind` → same updates for banner.
- `TestKindStatusColor_PerKind` → per-kind colour expectations updated.
- One inline literal in `TestBuildStatusSegment_DayOffBannerHoliday` (`"○ Tag der Arbeit"` → `"● Tag der Arbeit"`).
- New `TestStatusComposer_PaletteOverrideForPurpleAndOrange` covers `tn_purple` / `tn_orange` overrides through `StatusComposer`.

## Notes on the original spec

The May-12 spec is left in place as historical context. The decision matrix in that doc remains valid for the TUI surface; this spec only overrides the tmux-bar rendering. If/when the TUI pace-strip shows the same in-bar contrast issue, apply the same mapping there in a follow-up.

## Verification

`go test ./internal/domain/... ./internal/usecase/...` ✓
`flow worktime status` on the live system shows filled `●` in distinct colours for day-offs, outline `○` Dim for open workdays. Visual sanity check from across the room: the four colour fills are unambiguously distinct at status-bar font size.
