package webui

import (
	"context"
	"html"
	"html/template"
	"sort"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
)

// TagebuchAssignment is one row in the "Zuordnungen in dieser Notiz" list.
type TagebuchAssignment struct {
	ID, TargetName, ParentLabel, Quote, BorderClass, RemoveHref string
}

// TagebuchNodeOption is one entry in the assign-target picker.
type TagebuchNodeOption struct{ ID, Label string }

// TagebuchTally is one row in "Diesen Monat markiert".
type TagebuchTally struct {
	Name, DotColor, CountLabel string
}

// TagebuchMarkierenVM is Screen 27 ("Stellen markieren und je Register zuordnen").
type TagebuchMarkierenVM struct {
	Doc          TagebuchDetail
	AssignAction string
	Assignments  []TagebuchAssignment
	NodeOptions  []TagebuchNodeOption
	Tally        []TagebuchTally
}

// markHighlights wraps the first literal occurrence of each highlight's
// quote in the rendered HTML with a <mark>. Best-effort: Markdown rendering
// can split a quote across inline formatting, in which case the quote simply
// renders unmarked rather than corrupting the HTML — the assignment itself
// (stored server-side) is unaffected either way.
func markHighlights(body template.HTML, highlights []domain.NodeHighlight, colorOf func(nodeID string) string) template.HTML {
	out := string(body)
	for _, h := range highlights {
		needle := html.EscapeString(h.Quote)
		i := strings.Index(out, needle)
		if i < 0 {
			continue
		}
		mark := `<mark class="` + markerBgWash(colorOf(h.NodeID)) + `">` + needle + `</mark>`
		out = out[:i] + mark + out[i+len(needle):]
	}
	return template.HTML(out)
}

// markerBgWash maps a node colour to its wash-background highlight class,
// spelled out per case (not string-concatenated) so the Tailwind scanner
// finds every literal — mirrors cockpitAccent.
func markerBgWash(color string) string {
	switch color {
	case "teal":
		return "bg-teal/[.14]"
	case "purple":
		return "bg-purple/[.14]"
	case "green":
		return "bg-green/[.14]"
	case "blue":
		return "bg-blue/[.14]"
	case "red":
		return "bg-red/[.14]"
	case "violet":
		return "bg-violet/[.14]"
	case "steel":
		return "bg-steel/[.14]"
	case "amber":
		return "bg-amber/[.14]"
	default:
		return "bg-blue/[.14]"
	}
}

// nodeBorderClass maps a node colour to its left-accent border class,
// spelled out per case (not string-concatenated) so the Tailwind scanner
// finds every literal — mirrors cockpitAccent.
func nodeBorderClass(color string) string {
	switch color {
	case "teal":
		return "border-teal"
	case "purple":
		return "border-purple"
	case "green":
		return "border-green"
	case "blue":
		return "border-blue"
	case "red":
		return "border-red"
	case "violet":
		return "border-violet"
	case "steel":
		return "border-steel"
	case "amber":
		return "border-amber"
	default:
		return "border-blue"
	}
}

// BuildTagebuchMarkierenVM assembles Screen 27. nodesByID must contain every
// node referenced by highlights or targets (assign-target candidates plus
// their parents, for ParentLabel). targets is the assignable node list
// (Vorhaben/Engagement, already filtered+sorted by the caller).
func BuildTagebuchMarkierenVM(ctx context.Context, doc domain.Document, highlights []domain.NodeHighlight, nodesByID map[string]domain.Node, targets []domain.Node, monthHighlights []domain.NodeHighlight) TagebuchMarkierenVM {
	vm := TagebuchMarkierenVM{
		Doc:          *tagebuchDetailOf(ctx, doc),
		AssignAction: "/tagebuch/" + doc.ID + "/highlights",
	}
	vm.Doc.BodyHTML = markHighlights(vm.Doc.BodyHTML, highlights, func(id string) string { return nodesByID[id].Color })

	vm.Assignments = make([]TagebuchAssignment, len(highlights))
	for i, h := range highlights {
		n := nodesByID[h.NodeID]
		parent := ""
		if n.ParentID != nil {
			parent = nodesByID[*n.ParentID].Name
		}
		vm.Assignments[i] = TagebuchAssignment{
			ID: h.ID, TargetName: n.Name, ParentLabel: parent, Quote: h.Quote,
			BorderClass: nodeBorderClass(n.Color),
			RemoveHref:  "/tagebuch/" + doc.ID + "/highlights/" + h.ID + "/delete",
		}
	}

	vm.NodeOptions = make([]TagebuchNodeOption, len(targets))
	for i, n := range targets {
		label := n.Name
		if n.Kind == domain.KindVorhaben && n.ParentID != nil {
			if parent, ok := nodesByID[*n.ParentID]; ok {
				label = n.Name + " · " + parent.Name
			}
		}
		vm.NodeOptions[i] = TagebuchNodeOption{ID: n.ID, Label: label}
	}

	counts := map[string]int{}
	order := []string{}
	for _, h := range monthHighlights {
		if _, ok := counts[h.NodeID]; !ok {
			order = append(order, h.NodeID)
		}
		counts[h.NodeID]++
	}
	sort.Slice(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })
	vm.Tally = make([]TagebuchTally, len(order))
	for i, id := range order {
		n := nodesByID[id]
		vm.Tally[i] = TagebuchTally{Name: n.Name, DotColor: ColorHex(n.Color), CountLabel: itoa(counts[id])}
	}
	return vm
}
