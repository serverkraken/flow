package components_test

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

func TestButtonVariantsAndAttrs(t *testing.T) {
	out := render(t, components.Button(components.BtnDanger, "Löschen", "■",
		templ.Attributes{"hx-post": "/x/delete", "type": "submit"}))
	for _, w := range []string{"Löschen", "bg-red", `hx-post="/x/delete"`, `type="submit"`, "■"} {
		if !strings.Contains(out, w) {
			t.Errorf("Button missing %q: %s", w, out)
		}
	}
	prim := render(t, components.Button(components.BtnPrimary, "Speichern", "", nil))
	if strings.Contains(prim, "bg-red") {
		t.Errorf("primary button should not use danger color")
	}
}

func TestIconButtonRequiresAriaLabel(t *testing.T) {
	out := render(t, components.IconButton("✚", "Neu", templ.Attributes{"hx-get": "/new"}))
	for _, w := range []string{`aria-label="Neu"`, "✚", `hx-get="/new"`} {
		if !strings.Contains(out, w) {
			t.Errorf("IconButton missing %q", w)
		}
	}
}

func TestCardWrapsBody(t *testing.T) {
	out := render(t, components.Card("lg:col-span-2", templ.Raw(`<p id="cb">x</p>`)))
	if !strings.Contains(out, `id="cb"`) || !strings.Contains(out, "lg:col-span-2") || !strings.Contains(out, "glass") {
		t.Errorf("Card missing class or body: %s", out)
	}
}

func TestBadgeDocKinds(t *testing.T) {
	if out := render(t, components.Badge(components.KindProject)); !strings.Contains(out, "Projekt") || !strings.Contains(out, "text-green") {
		t.Errorf("project badge wrong: %s", out)
	}
	if out := render(t, components.Badge(components.KindDaily)); !strings.Contains(out, "Daily") {
		t.Errorf("daily badge wrong: %s", out)
	}
}

func TestChipAndTag(t *testing.T) {
	if out := render(t, components.Chip("flow", "teal")); !strings.Contains(out, "flow") || !strings.Contains(out, "text-teal") {
		t.Errorf("Chip wrong: %s", out)
	}
	if out := render(t, components.Tag("rebuild")); !strings.Contains(out, "rebuild") {
		t.Errorf("Tag wrong: %s", out)
	}
}

func TestStatTile(t *testing.T) {
	out := render(t, components.StatTile("nav.week", "32h 10m", "green"))
	for _, w := range []string{"Woche", "32h 10m", "text-green"} {
		if !strings.Contains(out, w) {
			t.Errorf("StatTile missing %q: %s", w, out)
		}
	}
}

func TestEmptyState(t *testing.T) {
	out := render(t, components.EmptyState("·", "empty.default", "empty.default"))
	if !strings.Contains(out, "Nichts vorhanden") {
		t.Errorf("EmptyState should render i18n title: %s", out)
	}
}

func TestButtonPrimary_KristallCTA(t *testing.T) {
	out := render(t, components.Button(components.BtnPrimary, "Timer starten", "▶", nil))
	for _, want := range []string{"from-green", "to-cyan", "text-oncolor", "cta-glow"} {
		if !strings.Contains(out, want) {
			t.Errorf("primary CTA missing %q: %s", want, out)
		}
	}
}

func TestStatTileAccent_RendersAccentBarAndSub(t *testing.T) {
	out := render(t, components.StatTileAccent("stats.week", "18h 20m", "+2h 05m", "purple"))
	for _, want := range []string{"rtile-ac", "--ac:var(--purple)", "18h 20m", "+2h 05m", "glass"} {
		if !strings.Contains(out, want) {
			t.Errorf("accent tile missing %q: %s", want, out)
		}
	}
}

func TestTabStrip_PillsAndCount(t *testing.T) {
	tabs := []components.Tab{{Key: "a", Href: "/a", LabelKey: "nav.projects", Count: 12}, {Key: "b", Href: "/b", LabelKey: "nav.home"}}
	out := render(t, components.TabStrip(tabs, "a"))
	if !strings.Contains(out, "pill-tabs") || !strings.Contains(out, `aria-current="page"`) {
		t.Errorf("pill container/active missing: %s", out)
	}
	if !strings.Contains(out, ">12<") {
		t.Errorf("count chip missing: %s", out)
	}
}

func TestSessionDialog_AddMode_NoNodes(t *testing.T) {
	vm := components.SessionDialogVM{
		DialogID: "session-dialog",
		Mode:     "add",
		Action:   "/api/v1/nodes/123/sessions",
		Target:   "#cockpit-main",
		Date:     "2026-07-02",
		From:     "14:00",
		To:       "15:00",
		Tag:      "build",
		Note:     "Feature work",
		Nodes:    []domain.Node{},
		NodeID:   "",
	}
	out := render(t, components.SessionDialog(vm))

	wants := []string{
		`<dialog id="session-dialog"`,
		`aria-modal="true"`,
		"Zeit nachbuchen",
		`hx-post="/api/v1/nodes/123/sessions"`,
		`hx-target="#cockpit-main"`,
		`hx-swap="innerHTML"`,
		`name="date"`,
		`value="2026-07-02"`,
		`name="from"`,
		`value="14:00"`,
		`name="to"`,
		`value="15:00"`,
		`name="tag"`,
		`value="build"`,
		`name="note"`,
		"Feature work",
	}
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("SessionDialog add-mode missing %q", want)
		}
	}

	// Verify NO select when Nodes empty
	if strings.Contains(out, `<select name="node"`) {
		t.Errorf("SessionDialog should not render select when Nodes empty")
	}
}

func TestSessionDialog_EditMode_WithNodes(t *testing.T) {
	vm := components.SessionDialogVM{
		DialogID: "edit-dialog",
		Mode:     "edit",
		Action:   "/api/v1/nodes/123/sessions/456/edit",
		Target:   "#cockpit-main",
		Date:     "2026-07-01",
		From:     "09:00",
		To:       "12:00",
		Tag:      "review",
		Note:     "Code review",
		Nodes: []domain.Node{
			{ID: "n1", Name: "Project A"},
			{ID: "n2", Name: "Project B"},
		},
		NodeID: "n1",
	}
	out := render(t, components.SessionDialog(vm))

	wants := []string{
		`<dialog id="edit-dialog"`,
		"Sitzung bearbeiten",
		`hx-post="/api/v1/nodes/123/sessions/456/edit"`,
		`<select name="node"`,
		`value="n1" selected`,
		">Project A<",
		`value="n2"`,
		">Project B<",
	}
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("SessionDialog edit-mode missing %q: %s", want, out)
		}
	}
}

func TestSessionDialog_AddMode_WithNodes_EmptyOptionSelectedByDefault(t *testing.T) {
	vm := components.SessionDialogVM{
		DialogID: "session-dialog",
		Mode:     "add",
		Action:   "/api/v1/nodes/123/sessions",
		Target:   "#cockpit-main",
		Date:     "2026-07-02",
		Nodes: []domain.Node{
			{ID: "n1", Name: "Project A"},
			{ID: "n2", Name: "Project B"},
		},
		NodeID: "",
	}
	out := render(t, components.SessionDialog(vm))

	if !strings.Contains(out, `<select name="node"`) {
		t.Fatalf("expected select when Nodes populated: %s", out)
	}

	selIdx := strings.Index(out, `<select name="node"`)
	firstOptIdx := strings.Index(out[selIdx:], "<option")
	if firstOptIdx == -1 {
		t.Fatalf("no option rendered: %s", out)
	}
	firstOpt := out[selIdx+firstOptIdx:]
	end := strings.Index(firstOpt, "</option>")
	firstOpt = firstOpt[:end+len("</option>")]

	if !strings.Contains(firstOpt, `value=""`) {
		t.Errorf("first option should be the empty/unassigned one, got: %s", firstOpt)
	}
	if !strings.Contains(firstOpt, "selected") {
		t.Errorf("empty option should be selected when NodeID is empty, got: %s", firstOpt)
	}
	if !strings.Contains(out, ">Project A<") || !strings.Contains(out, ">Project B<") {
		t.Errorf("real nodes should still be present: %s", out)
	}
}

func TestSessionDialog_EditMode_WithNodes_NodeSelectedNotEmptyOption(t *testing.T) {
	vm := components.SessionDialogVM{
		DialogID: "edit-dialog",
		Mode:     "edit",
		Action:   "/api/v1/nodes/123/sessions/456/edit",
		Target:   "#cockpit-main",
		Nodes: []domain.Node{
			{ID: "n1", Name: "Project A"},
			{ID: "n2", Name: "Project B"},
		},
		NodeID: "n1",
	}
	out := render(t, components.SessionDialog(vm))

	selIdx := strings.Index(out, `<select name="node"`)
	firstOptIdx := strings.Index(out[selIdx:], "<option")
	firstOpt := out[selIdx+firstOptIdx:]
	end := strings.Index(firstOpt, "</option>")
	firstOpt = firstOpt[:end+len("</option>")]

	if !strings.Contains(firstOpt, `value=""`) {
		t.Errorf("first option should still be the empty/unassigned one, got: %s", firstOpt)
	}
	if strings.Contains(firstOpt, "selected") {
		t.Errorf("empty option must NOT be selected when NodeID is set, got: %s", firstOpt)
	}
	if !strings.Contains(out, `value="n1" selected`) {
		t.Errorf("node n1 should be selected: %s", out)
	}
}

func TestSessionDialog_EditMode_RendersHiddenSessionID(t *testing.T) {
	vm := components.SessionDialogVM{
		DialogID: "d", Mode: "edit", Action: "/ui/worktime/edit", Target: "#content",
		SessionID: "s-42", Date: "2026-07-03", From: "09:00", To: "10:00",
	}
	out := render(t, components.SessionDialog(vm))
	if !strings.Contains(out, `name="sessionId"`) || !strings.Contains(out, `value="s-42"`) {
		t.Errorf("edit dialog missing hidden sessionId: %s", out)
	}
}

func TestSessionDialog_AddMode_NoSessionID(t *testing.T) {
	vm := components.SessionDialogVM{DialogID: "d", Mode: "add", Action: "/x", Target: "#c"}
	out := render(t, components.SessionDialog(vm))
	if strings.Contains(out, `name="sessionId"`) {
		t.Errorf("add dialog must not render sessionId: %s", out)
	}
}

func TestAvatar_RendersToneAndInitials(t *testing.T) {
	var sb strings.Builder
	if err := components.Avatar("BA", "av-c", "av-36").Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	for _, want := range []string{"av-c", "av-36", ">BA<", `aria-hidden="true"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("Avatar output missing %q:\n%s", want, out)
		}
	}
}

func TestSharedBanners_OnGlass(t *testing.T) {
	cases := []string{
		render(t, components.BurndownBanner(components.BurndownVM{})),
		render(t, components.WeekTotalBanner(components.WeekTotalVM{})),
		render(t, components.SelectionActionBar(components.SelectionBarVM{})),
		render(t, components.KennzahlenPanel(components.KennzahlenVM{})),
	}
	for i, out := range cases {
		if !strings.Contains(out, "glass") {
			t.Errorf("shared banner %d not on glass: %s", i, out)
		}
	}
}
