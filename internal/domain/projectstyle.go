package domain

// ProjectColors is the single source of valid project color NAMES (the visual
// identity palette). Each surface maps a name to its own rendering (WebUI: a
// hex swatch; TUI: a theme color) — the NAME set lives here so the surfaces
// cannot drift on which colors exist. Names mirror the theme palette accents.
var ProjectColors = []string{
	"blue", "cyan", "green", "purple", "magenta", "yellow", "orange", "red", "teal",
}

// ProjectGlyphs is the whitelist of monospace identity glyphs a project may
// carry (a curated subset of the UI glyph set; not emoji).
var ProjectGlyphs = []string{"◆", "●", "▶", "★", "☼", "✚", "▲", "■"}

// ValidProjectColor reports whether c is unset ("") or a whitelisted name.
func ValidProjectColor(c string) bool { return c == "" || inList(ProjectColors, c) }

// ValidProjectGlyph reports whether g is unset ("") or a whitelisted glyph.
func ValidProjectGlyph(g string) bool { return g == "" || inList(ProjectGlyphs, g) }

func inList(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
