package webui

import (
	"context"
	"strings"
	"testing"
)

func TestCallout_Note(t *testing.T) {
	out := string(mustHTML(RenderDocument(context.Background(), "> [!NOTE]\n> hello\n", resolveNone, nil)))
	if !strings.Contains(out, `class="callout callout-note"`) {
		t.Fatalf("expected callout-note div, got: %s", out)
	}
	if !strings.Contains(out, "hello") {
		t.Fatalf("callout body lost: %s", out)
	}
}

func TestCallout_AllKinds(t *testing.T) {
	for _, k := range []string{"note", "tip", "warning", "important", "danger"} {
		src := "> [!" + strings.ToUpper(k) + "]\n> body\n"
		out := string(mustHTML(RenderDocument(context.Background(), src, resolveNone, nil)))
		if !strings.Contains(out, "callout-"+k) {
			t.Fatalf("kind %s not rendered: %s", k, out)
		}
	}
}

func TestCallout_PlainBlockquoteUnchanged(t *testing.T) {
	out := string(mustHTML(RenderDocument(context.Background(), "> just a quote\n", resolveNone, nil)))
	if strings.Contains(out, "callout") {
		t.Fatalf("plain blockquote should not be a callout: %s", out)
	}
	if !strings.Contains(out, "<blockquote") {
		t.Fatalf("expected blockquote, got: %s", out)
	}
}
