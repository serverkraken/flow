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

// Die Klassen-Strings, die Go-Helfer zusammensetzen (nodeFilterChip,
// StatusBadge, historie_vm …), sieht TestR5_TemplatesAreSquare nicht — genau
// dort überlebten Pillen und Tailwind-Standardpalette den ersten Pass. Dieser
// Test liest deshalb Templates UND Go-Quellen (ohne Generate und Tests) und
// prüft neben dem Radius die anderen Off-Token-Spuren: Schriftgrößen in rem
// statt auf der Typo-Leiter, Standardpalette mit Stufen (amber-100, slate-400),
// Blur, Gradienten und Ring-Schatten.
func TestTokens_NoOffTokenClassesInSources(t *testing.T) {
	rules := []struct {
		name string
		re   *regexp.Regexp
	}{
		{"Radius", roundedClassRE},
		{"rem-Schriftgröße", regexp.MustCompile(`text-\[\d*\.\d+rem\]|text-\[\d+rem\]`)},
		{"Tailwind-Standardpalette", regexp.MustCompile(`\b(bg|text|border|ring|from|to|via|fill|stroke)-(amber|slate|emerald|gray|zinc|neutral|stone|red|orange|yellow|lime|green|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-\d{2,3}\b`)},
		{"Blur", regexp.MustCompile(`backdrop-blur`)},
		{"Gradient", regexp.MustCompile(`bg-gradient-`)},
		{"Ring", regexp.MustCompile(`\bring-\d`)},
	}
	var offenders []string
	_ = filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && (d.Name() == "static" || d.Name() == "gen") {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		isTempl := strings.HasSuffix(name, ".templ")
		isGo := strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_templ.go") && !strings.HasSuffix(name, "_test.go")
		if !isTempl && !isGo {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			for _, r := range rules {
				if !r.re.MatchString(line) || (r.name == "Radius" && liveDot(line)) {
					continue
				}
				offenders = append(offenders, r.name+" · "+filepath.Base(path)+":"+itoa(i+1)+" "+strings.TrimSpace(line)[:min(90, len(strings.TrimSpace(line)))])
			}
		}
		return nil
	})
	if len(offenders) > 0 {
		t.Errorf("Token-Pass: %d Off-Token-Stellen:\n  %s", len(offenders), strings.Join(offenders[:min(20, len(offenders))], "\n  "))
	}
}
