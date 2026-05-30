package lint_test

import (
	"os"
	"path/filepath"
	"testing"
)

// kompendiumHueExceptions are file paths (relative to
// internal/kompendium/frontend/tui) that may read raw Palette hues
// directly. Each entry lists the specific hues that are intentionally
// referenced and the reason — drift is caught by the symmetric strict
// check in TestKompendiumFrontendSemanticOnly.
//
// browse/styles_struct.go documents Teal as "Repo-Chip-Hue" — no
// Sem-Slot covers the chip's distinctive background today. Adding a
// Sem-Slot is a separate audit; until then this is the canonical
// allowlist entry.
//
// Note on the other raw-ish references in browse/styles_struct.go that
// are NOT in this map: pal.TagPalette (deliberate per-tag hash
// rotation), pal.Bg and pal.BgChip (surface tokens used as fg-on-accent
// backgrounds, contrast-tested separately) are not part of the rawHues
// set the check forbids — they're scaffold/palette tokens with no
// Sem-Slot competition, so no exception is required.
var kompendiumHueExceptions = map[string]map[string]struct{}{
	"browse/styles_struct.go": {"Teal": {}},
}

// TestKompendiumFrontendSemanticOnly extends the screen-tree hue check
// to the kompendium frontend (browse / view / writepicker). Without it
// a future Teal-style Hue-direct-Zugriff could land in this tree
// undetected — review finding M-Lint-Asymmetry.
//
// The same rawHues set is reused, so adopting a Sem-Slot for Red/Green/
// Yellow/Cyan/Blue/Purple/Magenta in the kompendium tree gets caught the
// same way as in the worktime / palette / projects screens.
func TestKompendiumFrontendSemanticOnly(t *testing.T) {
	t.Parallel()

	root := findKompendiumFrontendDir(t)
	files := walkScreenFiles(t, root)

	for relpath, fpath := range files {
		exempt := kompendiumHueExceptions[filepath.ToSlash(relpath)]
		hits := findRawHueAccess(t, fpath)
		for _, h := range hits {
			if _, ok := exempt[h.field]; ok {
				continue
			}
			t.Errorf("kompendium frontend %s:%d: raw palette hue %q is forbidden — use theme.Palette.Sem() instead",
				relpath, h.line, h.field)
		}
	}
}

// findKompendiumFrontendDir mirrors findScreensDir but targets
// internal/kompendium/frontend/tui — the parallel TUI tree that hosts
// browse / view / writepicker.
func findKompendiumFrontendDir(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	cur := wd
	for {
		candidate := filepath.Join(cur, "internal", "kompendium", "frontend", "tui")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	t.Fatalf("could not find internal/kompendium/frontend/tui above %q", wd)
	return ""
}
