package httpserver_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// formSegments scans rendered HTML for <form>/</form> pairs at parser level:
// it returns the body segment of the form whose opening tag contains marker,
// and the maximum form-nesting depth seen. Browsers DROP a nested <form>
// start tag, so its stray </form> closes the OUTER form early and every
// control below it (logo input, submit button) silently falls out of the
// form — the exact bug class this guards against.
func formSegments(t *testing.T, body, marker string) (segment string, maxDepth int) {
	t.Helper()
	depth, start := 0, -1
	for i := 0; i < len(body); {
		open := strings.Index(body[i:], "<form")
		clos := strings.Index(body[i:], "</form")
		switch {
		case open >= 0 && (clos < 0 || open < clos):
			depth++
			if depth > maxDepth {
				maxDepth = depth
			}
			end := strings.Index(body[i+open:], ">")
			if start < 0 && strings.Contains(body[i+open:i+open+end], marker) {
				start = i + open + end + 1
			}
			i += open + end + 1
		case clos >= 0:
			if depth == 1 && start >= 0 && segment == "" {
				segment = body[start : i+clos]
			}
			depth--
			i += clos + 1
		default:
			return segment, maxDepth
		}
	}
	return segment, maxDepth
}

// TestWebNodeEditPage_NoNestedForms pins the browser-facing integrity of the
// node edit page: no <form> may nest inside another (the nested reparent form
// used to orphan everything below it, so "Speichern" did nothing), and the
// logo file input plus a submit control must live INSIDE the main edit form.
func TestWebNodeEditPage_NoNestedForms(t *testing.T) {
	ts, c, ns := newWebNodesServer(t)
	seedTreeNode(t, ns, "n-eng", "Acme", domain.KindEngagement, nil)

	code, body := getN(t, ts, c, "/nodes/n-eng/edit")
	if code != 200 {
		t.Fatalf("edit GET = %d", code)
	}
	seg, maxDepth := formSegments(t, body, `action="/nodes/n-eng"`)
	if maxDepth > 1 {
		t.Errorf("edit page nests <form> elements (max depth %d) — browsers drop the inner tag and close the outer form early", maxDepth)
	}
	if seg == "" {
		t.Fatal("edit form (action=/nodes/n-eng) not found")
	}
	if !strings.Contains(seg, `name="logo"`) {
		t.Error("logo file input must sit inside the edit form")
	}
	if !strings.Contains(seg, `type="submit"`) {
		t.Error("submit button must sit inside the edit form")
	}
	// The reparent controls stay wired to the external move form.
	if !strings.Contains(body, `action="/nodes/n-eng/move"`) {
		t.Error("move form must still exist on the edit page")
	}
}
