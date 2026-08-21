package webui

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// Neue Karte (Screen 12): anlegen ohne Pfad-Tipperei. Der Pfad entsteht aus
// Register, Typ und Titel — hier auf dem Server, in neuekarte.js als Vorschau
// nach derselben Regel. Wer ihn doch ändern will, klappt „anpassen" auf.

// NeueKarteVM treibt das Formular im Dialog.
type NeueKarteVM struct {
	NodeID string         // vorausgewählt, weil man hier steht; "" = ohne Register
	Nodes  []EditorOption // alle Register des Besitzers, als Baum
	Types  []NeueKarteTyp
	Type   string // vorausgewählter Typ
	Date   string // YYYY-MM-DD — Tagesnotiz und datierte Pfade
	Err    string
}

// NeueKarteTyp ist ein Kartentyp mit Erklärung und Pfadmuster (für die
// Vorschau im Dialog: „{date}" und „{slug}" werden ersetzt).
type NeueKarteTyp struct {
	Value, Label, Desc, Pattern string
}

// neueKarteTypen ist die Reihenfolge im Dialog: das Menschliche zuerst, das
// Agenten-Eigene danach; der veraltete Agent-Typ fehlt.
var neueKarteTypen = []domain.DocumentType{
	domain.DocProject, domain.DocDaily, domain.DocPlan, domain.DocSpec,
	domain.DocMemory, domain.DocFree, domain.DocInstruction, domain.DocSkill, domain.DocActiveContext,
}

// NeueKarteTypes liefert die Typen mit Beschriftung, Erklärung und Muster.
func NeueKarteTypes(ctx context.Context) []NeueKarteTyp {
	out := make([]NeueKarteTyp, 0, len(neueKarteTypen))
	for _, t := range neueKarteTypen {
		out = append(out, NeueKarteTyp{
			Value:   string(t),
			Label:   components.T(ctx, "wissen.type."+string(t)),
			Desc:    components.T(ctx, "neuekarte.desc."+string(t)),
			Pattern: PathPattern(t),
		})
	}
	return out
}

// PathPattern ist das Pfadmuster je Typ (TOKENS.md Typen-Matrix).
func PathPattern(t domain.DocumentType) string {
	switch t {
	case domain.DocDaily:
		return "daily/{date}"
	case domain.DocProject:
		return "notes/{slug}"
	case domain.DocPlan:
		return "plans/{date}-{slug}"
	case domain.DocSpec:
		return "specs/{date}-{slug}-design"
	case domain.DocMemory:
		return "memories/{slug}"
	case domain.DocInstruction:
		return "instructions/{slug}"
	case domain.DocSkill:
		return "skills/{slug}"
	case domain.DocActiveContext:
		return "active-context"
	default:
		return "{slug}"
	}
}

// DerivedPath füllt das Muster: Datum aus now, Slug aus dem Titel. Ein
// leerer Titel ergibt „karte", damit der Pfad nie leer ist.
func DerivedPath(t domain.DocumentType, title string, now time.Time) string {
	slug := usecase.Slugify(title)
	if slug == "" {
		slug = "karte"
	}
	p := PathPattern(t)
	p = replaceAll(p, "{date}", now.Format("2006-01-02"))
	p = replaceAll(p, "{slug}", slug)
	return p
}

func replaceAll(s, old, new string) string {
	for {
		i := indexOf(s, old)
		if i < 0 {
			return s
		}
		s = s[:i] + new + s[i+len(old):]
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// parseDate liest das Formular-Datum zurück; ein leerer oder kaputter Wert
// ergibt den Nullzeitpunkt — die Vorschau zeigt dann das Muster mit
// 0001-01-01, nie einen Absturz.
func parseDate(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}
