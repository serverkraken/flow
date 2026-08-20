package webui

// Slice 1 des Karteikasten-Redesigns: web/tailwind.css trägt die Werte aus
// docs/superpowers/specs/assets/2026-08-karteikasten/TOKENS.md. Die Namen
// bleiben, die Werte wechseln — genau so steht es in der Übergabe ("es ändern
// sich die Werte, nicht die Semantik").
//
// Der Test liest die echte Datei, nicht eine Kopie: driftet ein Wert, faellt
// er hier auf, und zwar mit dem Namen aus dem Design daneben.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// squeeze macht die Ausrichtungs-Leerzeichen der Datei unschädlich: main
// richtet die Zahlenspalten optisch aus ("--live:   15 138  70"), das ist
// Formatierung und keine Aussage über den Wert.
var wsRun = regexp.MustCompile(`[ \t]+`)

func squeeze(s string) string { return wsRun.ReplaceAllString(s, " ") }

func TestKarteikastenTokens(t *testing.T) {
	css, err := os.ReadFile("../../../web/tailwind.css")
	if err != nil {
		t.Fatal(err)
	}
	src := squeeze(string(css))

	cases := []struct{ design, token, rgb string }{
		// Papier — fünf Flächen
		{"Tisch #E7E5DF", "--desk", "231 229 223"},
		{"Kasten #F1EDE2", "--panel", "241 237 226"},
		{"Kasten #F1EDE2", "--sunken", "241 237 226"},
		{"Lesesaal #F8F6F0", "--canvas", "248 246 240"},
		{"Lesesaal #F8F6F0", "--paper", "248 246 240"},
		{"Beleg #FDFCF7", "--sheet", "253 252 247"},
		{"Blatt #FFFFFF", "--surface", "255 255 255"},
		// Linien — fünf Stärken
		{"Rahmen #D9D4C6", "--hair2", "217 212 198"},
		{"Spalte #E0DACB", "--hairp", "224 218 203"},
		{"Abschnitt #E6E1D4", "--line2", "230 225 212"},
		{"Zeile #EDE9DC", "--line", "237 233 220"},
		{"Leerstelle #C9C3B2", "--hair3", "201 195 178"},
		// Tinte — fünf Stufen
		{"Titel #26241E", "--ink", "38 36 30"},
		{"Lesetext #33312A", "--body", "51 49 42"},
		{"Sekundär #5C5748", "--body2", "92 87 72"},
		{"Meta #8A8578", "--muted", "138 133 120"},
		{"Beschriftung #B5AFA0", "--faint", "181 175 160"},
		// Akzent und Kartentypen
		{"Akzent Ocker #B8720F", "--accent", "184 114 15"},
		{"Akzent-Wash #F8ECD4", "--accent-wash", "248 236 212"},
		{"Akzent tief #8A5A18", "--accent-deep", "138 90 24"},
		{"Plan #7A4FD0", "--purple", "122 79 208"},
		{"Spec #0B8A7B", "--teal", "11 138 123"},
		{"Notiz #3D5EDB", "--blue", "61 94 219"},
		{"Erinnerung #B4452F", "--red", "180 69 47"},
		{"Zeit live #0F8A46", "--live", "15 138 70"},
		{"Tagebuch #5A7A2E", "--green", "90 122 46"},
		// Ebenenfarben — die drei Register
		{"Engagement #8A5A18", "--amber", "138 90 24"},
		{"Vorhaben #8A4F7A", "--violet", "138 79 122"},
		{"Repo #4A6B8A", "--steel", "74 107 138"},
	}
	for _, c := range cases {
		if !strings.Contains(src, c.token+":") {
			t.Errorf("Token %s fehlt (%s)", c.token, c.design)
			continue
		}
		if !strings.Contains(src, c.rgb) {
			t.Errorf("%s (%s) erwartet RGB %q — nicht in tailwind.css", c.token, c.design, c.rgb)
		}
	}

	// Die Ebenenfarben müssen als Tailwind-Utilities herauskommen, sonst
	// kann die Schiene sie in Slice 2 nicht benutzen.
	for _, want := range []string{"--color-amber:", "--color-violet:", "--color-steel:", "--color-desk:", "--color-body2:"} {
		if !strings.Contains(src, want) {
			t.Errorf("@theme exportiert %s nicht", want)
		}
	}
}
