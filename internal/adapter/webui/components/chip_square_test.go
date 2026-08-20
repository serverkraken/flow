package components_test

// R5 des Karteikastens: "Alles eckig. Kein Radius — nicht an Flächen,
// Knöpfen, Feldern, Dialogen. Rund ist nur der Live-Punkt."
// Die Chips kamen aus der Kristall-Zeit mit rounded-full herein.

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func TestChipsAreSquare(t *testing.T) {
	for name, out := range map[string]string{
		"Chip": render(t, components.Chip("Plan", "purple")),
		"Tag":  render(t, components.Tag("oauth")),
	} {
		if strings.Contains(out, "rounded") {
			t.Errorf("%s trägt einen Radius (R5: alles eckig): %s", name, out)
		}
	}
}
