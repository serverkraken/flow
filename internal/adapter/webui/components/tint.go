package components

// Tint normalizes a Karteikasten tone name into the value a row's data-tint
// attribute carries. The stylesheet keys its Tint-Hover rules on that value:
// hovering a row paints the tone's wash plus the 3px Auswahlkante in the full
// tone, and data-tint-on makes the same paint permanent for a selected row.
// An unknown or empty tone falls back to the ocher accent, matching every
// surface's default Auswahlkante.
//
// Keeping the mapping here (rather than per surface) is what makes hover one
// rule across ~20 screens instead of a hand-rolled hover class per list.
func Tint(tone string) string {
	switch tone {
	case "purple", "teal", "cyan", "blue", "red", "green", "live", "violet", "steel", "amber":
		return tone
	default:
		return "accent"
	}
}
