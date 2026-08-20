package components_test

// Slice 2 — die Hülle. Der Karteikasten kennt keine Topbar: links steht die
// 264px-Schiene, rechts die Arbeitsfläche, und über der Fläche liegt ein
// 3px-Streifen in der Farbe des Bereichs, in dem man sich befindet
// (Konzept 02: "Der Farbstreifen oben am Screen nennt immer die Ebene oder
// den Bereich"). Er beginnt erst rechts der Schiene — die bleibt neutral.

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func renderShell(t *testing.T, active string) string {
	t.Helper()
	var b strings.Builder
	body := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := w.Write([]byte("<p>INHALT</p>"))
		return err
	})
	if err := components.AppShell(active, nil, nil, body).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestAppShell_IsARailNotATopbar(t *testing.T) {
	html := renderShell(t, "docs")
	for _, want := range []string{"264px", `id="app-rail"`, "/ui/nav/tree?active=docs"} {
		if !strings.Contains(html, want) {
			t.Errorf("Hülle vermisst %q", want)
		}
	}
	if strings.Contains(html, "topbar-nav") {
		t.Errorf("die Topbar-Navigation muss verschwunden sein")
	}
	// Ohne diese Datei ist der Baum tot: htmx führt Skripte in Fragmenten
	// hier nicht aus, das Verhalten muss also mit der Hülle kommen.
	if !strings.Contains(html, "js/railnav.js") {
		t.Errorf("die Hülle muss railnav.js laden")
	}
	if !strings.Contains(html, "INHALT") {
		t.Errorf("der Seiteninhalt muss durchgereicht werden")
	}
}

func TestAppShell_EbenenstreifenNamesTheArea(t *testing.T) {
	for active, want := range map[string]string{
		"docs":     "bg-blue",
		"zeit":     "bg-live",
		"projekte": "bg-amber",
		"home":     "bg-accent",
	} {
		html := renderShell(t, active)
		if !strings.Contains(html, want) {
			t.Errorf("Bereich %q braucht den Ebenenstreifen %q", active, want)
		}
	}
}

// Die Schiene selbst bleibt neutral: der Streifen beginnt rechts von ihr.
func TestAppShell_StripeStartsRightOfTheRail(t *testing.T) {
	html := renderShell(t, "docs")
	rail := strings.Index(html, `id="app-rail"`)
	stripe := strings.Index(html, "bg-blue")
	if rail < 0 || stripe < 0 {
		t.Fatalf("Schiene oder Streifen fehlen")
	}
	if stripe < rail {
		t.Errorf("der Ebenenstreifen darf nicht über der Schiene liegen")
	}
}
