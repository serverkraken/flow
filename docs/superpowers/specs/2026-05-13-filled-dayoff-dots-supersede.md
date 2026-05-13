# Spec — Filled Pace-Dots, Distinct Kind Hues, Unified TUI ↔ Tmux (supersedes 2026-05-12)

**Status:** Implemented (2026-05-13).
**Supersedes:** `2026-05-12-unified-dayoff-glyphs-design.md` (in-tree, kept as historical context).
**Touches:** `internal/domain/status.go`, `internal/usecase/status_composer.go`, `internal/frontend/tui/theme/status_adapter.go`, `internal/frontend/tui/screen/worktime/{week.go, dayoffs.go, history_heatmap.go}`, related `_test.go`. No changes required in `dotfiles` (`@tn_*` user-options stay optional defaults).

## Why this revisits the May-12 decision

The May-12 spec unified the day-off glyph to `○` (outlined circle) across the tmux pace-dot row and the TUI pace-strip, with kind carried by colour (`Sem.Info` / `Sem.Success` / `Sem.Warning`). The decision matrix in that doc explicitly considered **Option C — filled `●` for free days** and rejected it:

> Option C (gefüllter Kreis für freie Tage) — verworfen: Doppelbedeutung von `●` (Ziel erreicht + Tag abgegolten)

Six months of real use proved that was the wrong tradeoff *for the tmux bar specifically*. Soenne's report (verbatim):

> "Die ungefüllten 'frei' Kreise sind nicht von den anderen 'ungefüllten' Kreisen für 'arbeit noch nicht begonnen' zu unterscheiden. Die Farbe hat nicht so einen krassen Kontrast. Und auch untereinander unterscheiden sich die freien Tage so doll."

A first iteration of this spec (initial commit `e7da013`) flipped the glyph to `●` and rewired the colours to **Holiday=Yellow, Vacation=Purple, Sick=Orange**. A subsequent design review surfaced two remaining issues:

1. **Yellow + Orange are too perceptually close** — only ~21° apart on the hue wheel, both warm-amber. At status-bar font size the two warm fills read as "the same warm dot, just slightly different". The exact discriminability problem the patch was meant to fix.
2. **Cross-surface drift** — the TUI continued to use the May-12 mapping (Cyan/Green/Yellow via Sem.Info/Success/Warning) while the tmux bar now used Yellow/Purple/Orange. Same concept, two different colours depending on which surface the user looks at.

This revised spec adopts the **clean Option C** from the design review: separate hue families for the kind triad, and the same hue values shared across both surfaces.

## Decision

### Glyph carries shape-of-status, colour carries kind

```
●  this day is "accounted for" — workday target met OR scheduled day off
○  workday open / future / missed — nothing happened (yet)
```

The May-12 "double meaning of `●`" concern dissolves under this framing: filled = "the day has a known disposition", colour says *which* disposition. Outline = "no disposition yet". The shape distinction does the bulk of the visual work; colour disambiguates the filled siblings.

### Final mapping — three separated hue families

| State                                  | Glyph | Colour       | Hex       | Hue family    |
| -------------------------------------- | ----- | ------------ | --------- | ------------- |
| Workday target met                     | `●`   | Green        | `#9ece6a` | yellow-green  |
| Today running, target not yet hit      | `●`   | Cyan         | `#7dcfff` | sky           |
| Workday open / future / missed         | `○`   | Dim          | `#565f89` | muted blue-grey |
| Holiday (`KindHoliday`)                | `●`   | **Blue**     | `#7aa2f7` | cool, sachlich |
| Vacation (`KindVacation`)              | `●`   | **Purple**   | `#bb9af7` | identity      |
| Sick (`KindSick`)                      | `●`   | **Orange**   | `#ff9e64` | warning       |
| Unknown kind                           | `●`   | Dim          | `#565f89` | (glyph-distinct from open `○`) |
| Banner `[Frei: …]` prefix today        | `●`   | per-kind     | as above  | as above      |

The day-off triad is now a **triangle on the hue wheel** — Blue (~210°), Purple (~260°), Orange (~17°) — with each pair separated by ≥45°, well above the perceptual confusion threshold at small glyph sizes.

### Cross-surface unification (TUI ↔ Tmux)

The same hex values are used in **both** surfaces:

| Surface | Implementation point | Reads from |
| ------- | -------------------- | ---------- |
| Tmux bar (pace dots, banner `[Frei: …]`) | `domain.KindStatusColor` | `StatusPalette.{Blue,Purple,Orange}` |
| TUI worktime week pace-strip | `wocheStyles.kinds` map | `theme.Palette.{Blue,Purple,Orange}` |
| TUI history heatmap (cells + legend) | `kindColor` + heatmap legend | `theme.Palette.{Blue,Purple,Orange}` |
| TUI history month grid | `kindColor` | `theme.Palette.{Blue,Purple,Orange}` |
| TUI Frei tab (summary, entry rows, picker) | `kindColor` | `theme.Palette.{Blue,Purple,Orange}` |

The TUI bridge `theme.StatusPaletteFor` now also exposes Blue/Purple/Orange so future TUI-side previews of the tmux bar render with the same hues.

### Frei-view rule: text colour matches kind colour

In the Frei tab, **every Kind-bound piece of text** wears the kind colour:

- **Top summary** (`renderKindSummary`) — kind label (`"Feiertag"` / `"Urlaub"` / `"Krank"`) in kind colour; count remains dim. Already followed pre-spec.
- **Entry rows** (`renderEntryRow`) — kind label *and* user label (`d.Label`, e.g. `"Tag der Arbeit"`) both in kind colour. Date stays FgMuted as a contextual prefix.
- **Add-dialog kind picker** (`renderKindPicker`) — non-selected chips: glyph *and* label in kind colour (previously label was FgMuted, glyph was kind colour). Selected chip retains the Accent selection treatment.

Rationale: the Kind colour is an **identity** signal, not an accent. The skill rule "one accent per row" is preserved because identity colours don't compete with the row's accent (selection / focus); they let the eye scan `[date] [kind] [label]` as one cohesive unit per kind.

### Side effects (carried forward from the first iteration)

- **Today-running dot** stays Yellow → Cyan from the first iteration. Matches `tui-usability` skill's §Color semantics (Cyan = Active/running/live) and the running-banner Cyan.
- **Sick** stays at Orange (Warning); previously rendered Yellow in the bar but Orange-equivalent in the TUI. The unification picks Orange.

### Palette extensions

`StatusPalette` (`internal/domain/status.go`) now carries three slots beyond the original five:

| Slot   | Hex       | Used by                                    |
| ------ | --------- | ------------------------------------------ |
| Blue   | `#7aa2f7` | Holiday pace-dot + banner                  |
| Purple | `#bb9af7` | Vacation pace-dot + banner                 |
| Orange | `#ff9e64` | Sick pace-dot + banner; banner over-streak |

`StatusComposer.palette()` resolves all three via the standard `pick("tn_<name>", def.<Name>)` pattern. `StatusPaletteFor` (TUI → tmux preview adapter) was extended to also project Blue/Purple/Orange. `theme.Palette` already had Blue/Purple/Orange as raw hues — no TUI-palette additions needed.

No changes to `dotfiles/tmux/.tmux.conf` are required: the user-options stay optional, defaults always apply when unset.

## Tests updated

- `internal/domain/status_test.go` — three test groups updated for the new per-kind colour expectations (Blue/Purple/Orange).
- `internal/usecase/status_composer_test.go` — `TestStatusComposer_PaletteOverrideForPurpleAndOrange` covers the override path; Blue uses the same `pick()` pattern.
- `internal/frontend/cli/testdata/status.golden` — regenerated via `go test -update`.
- `internal/frontend/tui/screen/worktime/{week_test.go, dayoffs_test.go, history_heatmap_test.go, history_month_test.go}` — direct hue references replace `Sem.Info/Success/Warning` expectations.
- New `TestRenderEntryRow_UserLabelInKindColor` in `dayoffs_test.go` pins the new Frei-view rule.

## Notes on the original spec

The May-12 spec is left in place as historical context. The kind-style+glyph helpers in `glyphs/` (e.g. `glyphs.Holiday/Vacation/Extra`) are no longer load-bearing for worktime; they remain available for any future markdown / legacy consumer.

## Verification

`go test ./internal/domain/... ./internal/usecase/... ./internal/frontend/cli/... ./internal/frontend/tui/...` ✓ (pre-existing `TestMenu_TabStripStaysVisibleWhenMenuOpen` failure persists on `main` and is unrelated)

`flow worktime status` on the live system shows filled `●` in Blue/Purple/Orange for the three day-off kinds. Visual sanity check from across the room: the three colour fills are unambiguously distinct at status-bar font size, and there is no perceptual collision between them or with the today-running Cyan dot.
