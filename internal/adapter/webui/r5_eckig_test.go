package webui

// R5 des Karteikastens: "Alles eckig. Kein Radius — nicht an Flächen,
// Knöpfen, Feldern, Dialogen. Rund ist nur der Live-Punkt; gerundet nur das
// Gerät im Mobil-Mockup."
//
// Der Test liest die echten Quellen. Er ist bewusst streng: eine einzige
// vergessene Ecke fällt hier auf, statt sich über zwanzig Screens zu ziehen.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var roundedClassRE = regexp.MustCompile(`rounded-[a-z0-9\[\]()#-]*`)

// liveDot erkennt die eine erlaubte Ausnahme: ein kleiner runder Punkt, der
// "läuft gerade" sagt.
func liveDot(line string) bool {
	return strings.Contains(line, "bg-live") &&
		(strings.Contains(line, "h-[7px]") || strings.Contains(line, "h-1.5"))
}

func TestR5_TemplatesAreSquare(t *testing.T) {
	var offenders []string
	roots := []string{"..", "../components"}
	for _, root := range roots {
		matches, _ := filepath.Glob(filepath.Join(root, "*.templ"))
		sub, _ := filepath.Glob(filepath.Join(root, "*", "*.templ"))
		for _, f := range append(matches, sub...) {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			for i, line := range strings.Split(string(b), "\n") {
				if !roundedClassRE.MatchString(line) || liveDot(line) {
					continue
				}
				offenders = append(offenders, filepath.Base(f)+":"+itoa(i+1)+" "+strings.TrimSpace(line)[:min(90, len(strings.TrimSpace(line)))])
			}
		}
	}
	if len(offenders) > 0 {
		t.Errorf("R5: %d Stellen tragen noch einen Radius:\n  %s", len(offenders), strings.Join(offenders[:min(12, len(offenders))], "\n  "))
	}
}

func TestR5_StylesheetIsSquare(t *testing.T) {
	b, err := os.ReadFile("../../../web/tailwind.css")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`border-radius: ([^;]+);`)
	var offenders []string
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		v := strings.TrimSpace(m[1])
		if v == "0" || v == "999px" {
			continue
		}
		offenders = append(offenders, v)
	}
	if len(offenders) > 0 {
		t.Errorf("R5: %d Radien im Stylesheet: %v", len(offenders), offenders[:min(10, len(offenders))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
