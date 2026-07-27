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

// DisplayNames liefert für jeden Namen seinen Kurznamen (ShortName). Kollidieren
// zwei Kurznamen im übergebenen (sichtbaren) Kontext, bekommt jeder betroffene
// Name genau ein Elternsegment davor ("gitlab / group"). Spec §5.5 — eine Quelle
// für Projekte, Cockpit, Palette, Pills.
func DisplayNames(names []string) map[string]string {
	short := make(map[string]string, len(names))
	count := map[string]int{}
	for _, n := range names {
		s := ShortName(n)
		short[n] = s
		count[s]++
	}
	out := make(map[string]string, len(names))
	for _, n := range names {
		s := short[n]
		if count[s] > 1 {
			out[n] = parentSlashLeaf(n)
		} else {
			out[n] = s
		}
	}
	return out
}

// parentSlashLeaf gibt "<vorletztes Segment> / <letztes Segment>" zurück
// (nur ein Segment mehr), Fallback = ShortName wenn kein Elternsegment existiert.
func parentSlashLeaf(name string) string {
	name = strings.TrimRight(strings.TrimSpace(name), "/")
	i := strings.LastIndex(name, "/")
	if i < 0 {
		return name
	}
	leaf := name[i+1:]
	rest := name[:i]
	j := strings.LastIndex(rest, "/")
	parent := rest
	if j >= 0 {
		parent = rest[j+1:]
	}
	return parent + " / " + leaf
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
