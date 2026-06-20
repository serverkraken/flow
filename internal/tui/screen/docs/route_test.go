package docs_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/screen/docs"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func TestDocsRoute_titleAndRenders(t *testing.T) {
	// client nil + nil editor/opener: DocsModel renders its list chrome without
	// touching the network until Init's cmd runs (which we don't drain here).
	var r shell.Route = docs.NewRoute(nil, nil, nil, theme.Default, "alice")
	if r.Title() != "Docs" {
		t.Fatalf("title = %q, want Docs", r.Title())
	}
	body := r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	if !strings.Contains(body, "kompendium") { // DocsModel renders a "kompendium" list header
		t.Fatalf("docs body should contain the kompendium list header:\n%s", body)
	}
	if len(r.KeyHints()) == 0 {
		t.Fatal("docs route should expose key hints")
	}
}

func TestDocsRoute_updateReturnsRoute(t *testing.T) {
	var r shell.Route = docs.NewRoute(nil, nil, nil, theme.Default, "alice")
	r2, _ := r.Update(tea.KeyPressMsg{Text: "j"})
	if r2 == nil {
		t.Fatal("Update must return a Route")
	}
}

// TestDocsRoute_implementsFullScreenerListFalse asserts the docs route satisfies
// shell.FullScreener and reports false while in the document list (the true case
// needs an unexported docViewMsg, covered in the package tui test + done-gate).
func TestDocsRoute_implementsFullScreenerListFalse(t *testing.T) {
	r := docs.NewRoute(nil, nil, nil, theme.Default, "alice")
	fs, ok := interface{}(r).(shell.FullScreener)
	if !ok {
		t.Fatal("docs.Route must implement shell.FullScreener")
	}
	if fs.FullScreen() {
		t.Fatal("list mode: FullScreen() must be false")
	}
}

// TestDocsRoute_capturesInputInSubmode guards the bug where the shell ate the
// New-Document form's Tab/Esc keys: the adapter must implement
// shell.InputCapturer and report capture once the docs screen leaves list mode.
func TestDocsRoute_capturesInputInSubmode(t *testing.T) {
	r := docs.NewRoute(nil, nil, nil, theme.Default, "alice")
	ic, ok := interface{}(r).(shell.InputCapturer)
	if !ok {
		t.Fatal("docs.Route must implement shell.InputCapturer")
	}
	if ic.CapturesInput() {
		t.Fatal("list mode: route must not capture (host nav must work)")
	}
	// 'n' opens the create form; the adapter must now report capture so the
	// shell forwards Tab/Esc to the form instead of switching tabs.
	r.Update(tea.KeyPressMsg{Text: "n"})
	if !ic.CapturesInput() {
		t.Fatal("create mode: route must report CapturesInput()==true")
	}
}

// chainDocSrv serves three docs wired by wikilinks so a test can build a docs
// viewStack of depth >=2 through the real public Update path:
//
//	d1 (Path "docs/a", body "[[docs/b]]") -> d2 (Path "docs/b", body "[[docs/c]]") -> d3 (Path "docs/c")
//
// It is the minimal subset of the package-tui newFakeDocSrv (list + get-by-id +
// backlinks) needed here; replicated because that helper is test-only to tui.
func chainDocSrv(t *testing.T) (*apiclient.Client, func()) {
	t.Helper()
	// Each body carries a unique sentinel so the test can tell WHICH doc the
	// viewer is rendering (a title alone is ambiguous: doc A's body links to
	// doc B, so B's title shows as a link label even while reading A).
	docs := []domain.Document{
		{ID: "d1", Type: domain.DocFree, Path: "docs/a", Title: "Alpha", Body: "BODY_OF_ALPHA\n\n[[docs/b]]"},
		{ID: "d2", Type: domain.DocFree, Path: "docs/b", Title: "Bravo", Body: "BODY_OF_BRAVO\n\n[[docs/c]]"},
		{ID: "d3", Type: domain.DocFree, Path: "docs/c", Title: "Charlie", Body: "BODY_OF_CHARLIE"},
	}
	stored := map[string]domain.Document{}
	for _, d := range docs {
		stored[d.ID] = d
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/documents", func(w http.ResponseWriter, _ *http.Request) {
		list := make([]domain.Document, 0, len(stored))
		for _, d := range docs { // stable order
			list = append(list, stored[d.ID])
		}
		_ = json.NewEncoder(w).Encode(list)
	})
	mux.HandleFunc("GET /api/v1/documents/{id}", func(w http.ResponseWriter, r *http.Request) {
		d, ok := stored[r.PathValue("id")]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(d)
	})
	// Backlinks endpoint: empty list keeps the doc-view path happy.
	mux.HandleFunc("GET /api/v1/documents/{id}/backlinks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.BacklinkRef{})
	})
	srv := httptest.NewServer(mux)
	return apiclient.New(srv.URL, "tok"), srv.Close
}

// drainRoute runs the cmd the adapter returned to completion, feeding every
// resulting msg back into the route (acting as the bubbletea runtime). It sizes
// the overlay via View after each step so the modeView render path is exercised.
// Stops once a step yields no further cmd or no msg. It deliberately ignores
// tea.BatchMsg fan-out beyond the first useful payload — the chain docs here
// produce a single docViewMsg per navigation, which is all the test needs.
func drainRoute(t *testing.T, r shell.Route, cmd tea.Cmd) shell.Route {
	t.Helper()
	for i := 0; cmd != nil && i < 20; i++ {
		msg := cmd()
		if msg == nil {
			break
		}
		if b, ok := msg.(tea.BatchMsg); ok {
			// Run each batched cmd; keep the route, chain only the last non-nil cmd.
			var next tea.Cmd
			for _, c := range b {
				if c == nil {
					continue
				}
				m := c()
				if m == nil {
					continue
				}
				var nc tea.Cmd
				r, nc = r.Update(m)
				if nc != nil {
					next = nc
				}
			}
			cmd = next
		} else {
			r, cmd = r.Update(msg)
		}
		r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})
	}
	return r
}

// driveToViewStackDepth2 builds a docs adapter route in modeView at viewStack
// depth 2 by: load list -> open d1 -> follow [[docs/b]] (push d1) -> follow
// [[docs/c]] (push d2). The returned route is reading d3 (Charlie) with d1,d2
// on the back-stack.
func driveToViewStackDepth2(t *testing.T) shell.Route {
	t.Helper()
	c, stop := chainDocSrv(t)
	t.Cleanup(stop)
	var r shell.Route = docs.NewRoute(c, nil, nil, theme.Default, "alice")

	// Init loads the list.
	r = drainRoute(t, r, r.Init())
	r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})

	// Enter opens the selected (first) doc -> Alpha (d1).
	var cmd tea.Cmd
	r, cmd = r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	r = drainRoute(t, r, cmd)
	if !strings.Contains(r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default}), "BODY_OF_ALPHA") {
		t.Fatalf("setup: expected to be reading Alpha (d1) body")
	}

	// Tab focuses the [[docs/b]] wikilink, Enter follows it -> Bravo (d2), push d1.
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	r, cmd = r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	r = drainRoute(t, r, cmd)
	if !strings.Contains(r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default}), "BODY_OF_BRAVO") {
		t.Fatalf("setup: expected to follow [[docs/b]] to Bravo (d2) body")
	}

	// Tab + Enter again -> Charlie (d3), push d2. viewStack now = [d1, d2].
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	r, cmd = r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	r = drainRoute(t, r, cmd)
	if !strings.Contains(r.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default}), "BODY_OF_CHARLIE") {
		t.Fatalf("setup: expected to follow [[docs/c]] to Charlie (d3) body")
	}
	return r
}

// TestDocsRoute_BackIsIdempotentOnReceiver locks the double-pop bug: ResolveBack
// calls Back() once to probe `handled` and the shell calls it again to apply.
// If *Route.Back() mutates its receiver, the probe permanently pops the
// viewStack, so the apply pops a SECOND level and the previous doc is skipped.
// Calling Back() twice on the SAME receiver must therefore yield the SAME
// resolved state — i.e. the first call must NOT mutate r.
func TestDocsRoute_BackIsIdempotentOnReceiver(t *testing.T) {
	r := driveToViewStackDepth2(t) // reading Charlie (d3); stack [d1, d2]

	b, ok := r.(shell.Backer)
	if !ok {
		t.Fatal("docs.Route must implement shell.Backer")
	}

	// Mirror the shell's exact back-chain: ResolveBack calls Back() ONCE as a
	// probe (discarding route+cmd, reading only `handled`), then the shell calls
	// Back() AGAIN to apply. The probe's cmd is never drained between the two
	// calls, so the only state change a probe could cause is via the receiver.
	// A pure Back() must therefore yield the SAME resolved route on both calls.
	_, _, ok1 := b.Back() // probe (result discarded, exactly like ResolveBack)
	if !ok1 {
		t.Fatal("first Back() (probe) should report handled==true")
	}
	applied, cmd, ok2 := b.Back() // apply
	if !ok2 {
		t.Fatal("second Back() (apply) should also report handled==true — the probe must not mutate the receiver")
	}
	applied = drainRoute(t, applied, cmd)
	got := applied.View(shell.Frame{Width: 80, Height: 24, Pal: theme.Default})

	// One q-press = exactly one level: Charlie -> Bravo (d2). If the probe popped
	// the receiver, the apply pops a SECOND level and lands on Alpha (d1). Assert
	// on the body sentinel (a title is ambiguous: Alpha's body links to Bravo, so
	// "Bravo" appears as a label even while reading Alpha).
	if !strings.Contains(got, "BODY_OF_BRAVO") {
		t.Fatalf("probe+apply must pop exactly one level to Bravo (d2); receiver-mutating Back() skips to a wrong doc:\n%s", got)
	}
	if strings.Contains(got, "BODY_OF_ALPHA") {
		t.Fatalf("probe+apply landed on Alpha (d1) — the probe mutated the receiver and a level was skipped:\n%s", got)
	}
}
