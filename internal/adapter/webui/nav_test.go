package webui_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/webui"
)

func TestNavRendersAllLinksIncludingProjects(t *testing.T) {
	var buf bytes.Buffer
	if err := webui.Nav("projekte", "msoent").Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{`href="/"`, `href="/dayoffs"`, `href="/stats"`, `href="/export"`, `href="/wissen"`, `href="/projects"`, "projekte", "msoent", "/auth/logout"} {
		if !strings.Contains(out, want) {
			t.Errorf("nav missing %q\n%s", want, out)
		}
	}
}
