package webui

import "strings"

// ShortName ist der Anzeigename eines Knotens: das letzte Pfadsegment eines
// Remote-Namens (Spec §5 Kurznamen-Regel; Kollisions-Dedup folgt in L2).
func ShortName(name string) string {
	name = strings.TrimRight(strings.TrimSpace(name), "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return name
}

// Initials liefert 1–2 Großbuchstaben für den Ersatz-Avatar: Anfangsbuchstaben
// der ersten beiden Wörter (Trenner: Space, "-", "_", "."), sonst die ersten
// beiden Runen. Leer → "?".
func Initials(name string) string {
	fields := strings.FieldsFunc(strings.TrimSpace(name), func(r rune) bool {
		return r == ' ' || r == '-' || r == '_' || r == '.'
	})
	switch {
	case len(fields) == 0:
		return "?"
	case len(fields) == 1:
		r := []rune(fields[0])
		if len(r) == 1 {
			return strings.ToUpper(string(r[0]))
		}
		return strings.ToUpper(string(r[0]) + string(r[1]))
	default:
		return strings.ToUpper(string([]rune(fields[0])[0]) + string([]rune(fields[len(fields)-1])[0]))
	}
}

// AvatarTone wählt deterministisch einen der sechs Lesesaal-Töne (Spec §7.2).
// Farbe pro Projekt lebt NUR hier — nirgendwo sonst.
func AvatarTone(name string) string {
	var h uint32
	for _, r := range name {
		h = h*31 + uint32(r)
	}
	return string([]byte{'a', 'v', '-', byte('a' + h%6)})
}
