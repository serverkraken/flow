package webui

// Aus dem Entwurfs-Branch portiert: die Helfer der Kasten-Spalte (Screen 01).
// Sie hier neu zu schreiben hieße, dieselbe Sache ein zweites Mal zu bauen.

import (
	"regexp"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
)

// Vorschau-Regexe der Auszugszeile: aus dem Rohtext einer Karte wird eine
// saubere Zeile — Wikilinks, HTML, Auszeichnung und Links fallen weg.
var (
	wikiAliasPreviewRE  = regexp.MustCompile(`\[\[([^|\]]+)\|([^\]]+)\]\]`)
	wikiSimplePreviewRE = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	htmlPreviewRE       = regexp.MustCompile(`<[^>]*>`)
	markdownPreviewRE   = regexp.MustCompile(`[*_~` + "`" + `]+([^*_~` + "`" + `]+)[*_~` + "`" + `]+`)
	markdownLinkRE      = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
)

func docRowTypeColor(t domain.DocumentType) string {
	switch t {
	case domain.DocDaily:
		return "green" // Tagebuch
	case domain.DocPlan:
		return "purple" // Plan
	case domain.DocSpec:
		return "teal" // Spec (Festlegungen)
	case domain.DocActiveContext:
		return "accent" // Kontext
	case domain.DocProject, domain.DocAgent, domain.DocMemory, domain.DocInstruction, domain.DocSkill:
		return "blue" // Notiz, Instruktion, Bibliothek
	case domain.DocFree:
		return "purple"
	default:
		return "accent"
	}
}

func stripPreviewFrontmatter(body string) string {
	if !strings.HasPrefix(body, "---\n") {
		return body
	}
	if end := strings.Index(body[4:], "\n---\n"); end >= 0 {
		return body[end+9:]
	}
	return body
}

func simplifyPreviewMarkdown(line string) string {
	line = strings.TrimLeft(line, "#>-*+ \t")
	line = wikiAliasPreviewRE.ReplaceAllString(line, "$2")
	line = wikiSimplePreviewRE.ReplaceAllString(line, "$1")
	line = markdownLinkRE.ReplaceAllString(line, "$1")
	line = htmlPreviewRE.ReplaceAllString(line, "")
	line = markdownPreviewRE.ReplaceAllString(line, "$1")
	return strings.Join(strings.Fields(line), " ")
}

// paletteTypeTextClass gibt die Textfarbe eines Kartentyps. Ausgeschrieben,
// nicht zusammengesetzt — der Tailwind-Scanner findet nur Literale.
func paletteTypeTextClass(color string) string {
	switch color {
	case "purple":
		return "text-purple"
	case "teal":
		return "text-teal"
	case "blue":
		return "text-blue"
	case "red":
		return "text-red"
	case "green":
		return "text-green"
	case "live":
		return "text-live"
	default:
		return "text-accent"
	}
}

// EbeneAccentColor bildet die Ebene eines Registers auf ihren Ton ab
// (TOKENS.md Ebenenfarben): Engagement ocker, Vorhaben violett, Repo stahl.
// Der Ton färbt den 3px-Streifen über der Arbeitsfläche.
func EbeneAccentColor(kind domain.NodeKind) string {
	switch kind {
	case domain.KindEngagement:
		return "amber"
	case domain.KindRepo, domain.KindBranch:
		return "steel"
	case domain.KindVorhaben:
		return "violet"
	default:
		return ""
	}
}
