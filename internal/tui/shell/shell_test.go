package shell_test

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

func newShell() shell.Shell {
	return shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{
		shell.NewHomeRoute(nil, theme.Default, "alice"),
		stubRoute{title: "Worktime", push: stubRoute{title: "Detail"}},
	})
}

func TestShell_windowSize(t *testing.T) {
	next, _ := newShell().Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	s := next.(shell.Shell)
	if s.Width() != 120 || s.Height() != 40 {
		t.Fatalf("size = %dx%d", s.Width(), s.Height())
	}
}

func TestShell_tabSwitchByDigitAndTab(t *testing.T) {
	next, _ := newShell().Update(tea.KeyPressMsg{Text: "2"})
	if next.(shell.Shell).ActiveTab() != 1 {
		t.Fatal("digit 2 -> tab 1")
	}
	next2, _ := newShell().Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if next2.(shell.Shell).ActiveTab() != 1 {
		t.Fatal("tab -> next")
	}
}

func TestShell_paletteOpenClose(t *testing.T) {
	next, _ := newShell().Update(tea.KeyPressMsg{Text: ":"})
	s := next.(shell.Shell)
	if !s.PaletteOpen() {
		t.Fatal("':' opens palette")
	}
	// Esc inside palette emits PaletteDismissedMsg; feed it back.
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	next2, _ := s.Update(cmd())
	if next2.(shell.Shell).PaletteOpen() {
		t.Fatal("dismiss closes palette")
	}
}

func TestShell_drillDownAndBack(t *testing.T) {
	// Switch to tab 1 (stub that pushes "Detail" on Enter).
	s, _ := newShell().Update(tea.KeyPressMsg{Text: "2"})
	sh := s.(shell.Shell)
	// Enter -> route emits PushRouteMsg; feed the produced msg back.
	_, cmd := sh.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should produce a push command")
	}
	pushed, _ := sh.Update(cmd())
	sh = pushed.(shell.Shell)
	if sh.ActiveDepth() != 2 {
		t.Fatalf("after push depth = %d want 2", sh.ActiveDepth())
	}
	// Esc pops back.
	back, _ := sh.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if back.(shell.Shell).ActiveDepth() != 1 {
		t.Fatal("esc should pop back to depth 1")
	}
}

// initCountRoute records how often its Init() is invoked (through a shared
// pointer, so value copies inside the NavStack still count).
type initCountRoute struct {
	stubRoute
	calls *int
}

func (r initCountRoute) Init() tea.Cmd { *r.calls++; return nil }

// Popping back to a revealed route must re-Init it, just like push/switch/tab
// switch do — so a root route that paused background work (e.g. Today's live
// clock) while drilled-over resumes when it becomes the active top again.
func TestShell_pop_reinitsRevealedRoute(t *testing.T) {
	var rootInit, childInit int
	root := initCountRoute{stubRoute{title: "Worktime"}, &rootInit}
	child := initCountRoute{stubRoute{title: "Woche"}, &childInit}

	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{root})
	next, _ := s.Update(shell.PushRouteMsg{Route: child})
	sh := next.(shell.Shell)
	if sh.ActiveDepth() != 2 {
		t.Fatalf("after push depth = %d, want 2", sh.ActiveDepth())
	}
	before := rootInit
	next2, _ := sh.Update(shell.PopRouteMsg{})
	sh2 := next2.(shell.Shell)
	if sh2.ActiveDepth() != 1 {
		t.Fatalf("after pop depth = %d, want 1", sh2.ActiveDepth())
	}
	if rootInit != before+1 {
		t.Fatalf("pop should re-Init the revealed route (rootInit=%d, want %d)", rootInit, before+1)
	}
}

// Popping back via the Esc/q back-chain (ResolveBack -> BackPop) must re-Init the
// revealed route too, exactly like the programmatic PopRouteMsg path — otherwise a
// drilled-in child (e.g. daydetail after a Nachbuchen) returns to a stale parent
// (e.g. Woche showing pre-edit per-day totals) until it is manually reloaded.
func TestShell_backKeyPop_reinitsRevealedRoute(t *testing.T) {
	var rootInit, childInit int
	root := initCountRoute{stubRoute{title: "Worktime"}, &rootInit}
	child := initCountRoute{stubRoute{title: "Woche"}, &childInit}

	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{root})
	next, _ := s.Update(shell.PushRouteMsg{Route: child})
	sh := next.(shell.Shell)
	if sh.ActiveDepth() != 2 {
		t.Fatalf("after push depth = %d, want 2", sh.ActiveDepth())
	}
	before := rootInit
	// Esc on a plain (non-Backer, non-TextCapturer) child at depth 2 resolves to
	// BackPop. The revealed root must be re-Init'd.
	back, _ := sh.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	sh2 := back.(shell.Shell)
	if sh2.ActiveDepth() != 1 {
		t.Fatalf("after esc-pop depth = %d, want 1", sh2.ActiveDepth())
	}
	if rootInit != before+1 {
		t.Fatalf("esc/BackPop should re-Init the revealed route (rootInit=%d, want %d)", rootInit, before+1)
	}
}

func TestShell_initsActiveTabRouteAtStartup(t *testing.T) {
	var home, work int
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{
		initCountRoute{stubRoute{title: "Home"}, &home},
		initCountRoute{stubRoute{title: "Worktime"}, &work},
	})
	_ = s.Init()
	if home != 1 {
		t.Fatalf("active tab Init calls = %d, want 1", home)
	}
}

func TestShell_initsTabRouteOnSwitch(t *testing.T) {
	var home, work int
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{
		initCountRoute{stubRoute{title: "Home"}, &home},
		initCountRoute{stubRoute{title: "Worktime"}, &work},
	})
	_ = s.Init()
	if _, _ = s.Update(tea.KeyPressMsg{Text: "2"}); work != 1 {
		t.Fatalf("switched-to tab Init calls = %d, want 1", work)
	}
}

func TestShell_quit(t *testing.T) {
	_, cmd := newShell().Update(tea.KeyPressMsg{Text: "q"})
	if cmd == nil {
		t.Fatal("q should quit")
	}
}

func TestShell_viewNoPanic(t *testing.T) {
	s, _ := newShell().Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("View panicked: %v", r)
		}
	}()
	_ = s.(shell.Shell).View()
}

// captureRoute is a stubRoute that captures input and records keys it receives.
type captureRoute struct {
	stubRoute
	capturing bool
	gotKeys   *[]string
}

func (r captureRoute) CapturesInput() bool { return r.capturing }
func (r captureRoute) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		*r.gotKeys = append(*r.gotKeys, k.Text)
	}
	return r, nil
}

func TestShell_capturingRouteReceivesDigitInsteadOfTabSwitch(t *testing.T) {
	var keys []string
	cap := captureRoute{stubRoute{title: "Form"}, true, &keys}
	other := stubRoute{title: "Other"}
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{cap, other})

	// Active tab 0 captures: "2" must reach the route, NOT switch to tab 1.
	next, _ := s.Update(tea.KeyPressMsg{Text: "2"})
	sh := next.(shell.Shell)
	if sh.ActiveTab() != 0 {
		t.Fatalf("capturing route: digit must not switch tab (activeTab=%d)", sh.ActiveTab())
	}
	if len(keys) != 1 || keys[0] != "2" {
		t.Fatalf("capturing route should receive '2', got %v", keys)
	}
}

func TestShell_nonCapturingRouteStillSwitchesTabOnDigit(t *testing.T) {
	var keys []string
	cap := captureRoute{stubRoute{title: "Form"}, false, &keys}
	other := stubRoute{title: "Other"}
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{cap, other})

	next, _ := s.Update(tea.KeyPressMsg{Text: "2"})
	if next.(shell.Shell).ActiveTab() != 1 {
		t.Fatal("non-capturing route: digit '2' should switch to tab 1")
	}
}

func TestShell_switchTabByTitle(t *testing.T) {
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{
		stubRoute{title: "Home"},
		stubRoute{title: "Worktime"},
		stubRoute{title: "Docs"},
	})
	next, _ := s.Update(shell.SwitchTabMsg{Title: "Docs"})
	if next.(shell.Shell).ActiveTab() != 2 {
		t.Fatalf("SwitchTabMsg{Docs} should activate tab 2, got %d", next.(shell.Shell).ActiveTab())
	}
	// Unknown title is a no-op (stays put).
	again, _ := next.(shell.Shell).Update(shell.SwitchTabMsg{Title: "Nope"})
	if again.(shell.Shell).ActiveTab() != 2 {
		t.Fatalf("unknown SwitchTabMsg should be a no-op, got %d", again.(shell.Shell).ActiveTab())
	}
}

func TestShell_withActiveTabStartsThere(t *testing.T) {
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{
		stubRoute{title: "Home"},
		stubRoute{title: "Worktime"},
		stubRoute{title: "Docs"},
	}).WithActiveTab(1)
	if s.ActiveTab() != 1 {
		t.Fatalf("WithActiveTab(1) => ActiveTab %d, want 1", s.ActiveTab())
	}
	// Out of range clamps to 0.
	if shell.New(nil, "a", theme.Default).WithActiveTab(9).ActiveTab() != 0 {
		t.Fatal("WithActiveTab(out-of-range) should clamp to 0")
	}
}

// fullScreenRoute embeds stubRoute but overrides View to return a distinct
// body string ("BODY") so the test can tell the body apart from the tabstrip
// title ("Docs") in the rendered output.
type fullScreenRoute struct{ stubRoute }

func (r fullScreenRoute) FullScreen() bool                          { return true }
func (r fullScreenRoute) View(_ shell.Frame) string                 { return "BODY" }
func (r fullScreenRoute) Update(msg tea.Msg) (shell.Route, tea.Cmd) { return r, nil }

func TestShell_fullScreenSuppressesChrome(t *testing.T) {
	normal := shell.New(nil, "alice", theme.Default).
		WithTabs([]shell.Route{stubRoute{title: "Docs"}})
	full := shell.New(nil, "alice", theme.Default).
		WithTabs([]shell.Route{fullScreenRoute{stubRoute{title: "Docs"}}})
	// Give both a size.
	n, _ := normal.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	f, _ := full.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	nv := n.(shell.Shell).View().Content
	fv := f.(shell.Shell).View().Content
	// The tab strip title appears in normal chrome but not in fullscreen.
	if !strings.Contains(nv, "Docs") {
		t.Fatalf("normal view should show the tabstrip:\n%s", nv)
	}
	if strings.Contains(fv, "Docs") {
		t.Fatalf("fullscreen view must suppress the tabstrip chrome:\n%s", fv)
	}
	// The body content must be present in fullscreen.
	if !strings.Contains(fv, "BODY") {
		t.Fatalf("fullscreen view must contain body content:\n%s", fv)
	}
}

// backStubRoute is a Route that reports it resolved one internal "back" level by
// swapping itself for replacement when Back() is called.
type backStubRoute struct {
	stubRoute
	replacement shell.Route
}

func (r backStubRoute) Back() (shell.Route, tea.Cmd, bool) { return r.replacement, nil, true }

// TestShell_BackKeyResolves: a Backer route at the root handles Esc internally
// (ReplaceTop with its replacement, no Quit), while q at a clean root quits.
func TestShell_BackKeyResolves(t *testing.T) {
	replaced := stubRoute{title: "List"}
	root := backStubRoute{stubRoute{title: "View"}, replaced}
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{root})

	// Esc on a Backer root: internal back, no quit, top swapped to "List".
	next, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	sh := next.(shell.Shell)
	if cmd != nil {
		if _, isQuit := cmd().(tea.QuitMsg); isQuit {
			t.Fatal("Backer route must not quit on Esc")
		}
	}
	if sh.ActiveDepth() != 1 {
		t.Fatalf("Backer root: depth = %d, want 1 (replace not pop)", sh.ActiveDepth())
	}
	v := sh.View().Content
	if !strings.Contains(v, "List") {
		t.Fatalf("Back should have replaced the top route with 'List':\n%s", v)
	}

	// A clean (non-Backer) root quits on q.
	plain := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{stubRoute{title: "Home"}})
	_, qcmd := plain.Update(tea.KeyPressMsg{Text: "q"})
	if qcmd == nil {
		t.Fatal("q at a clean root should quit")
	}
}

// textCaptureRoute captures literal text: every key (including q/Esc) belongs to
// it. It records the keys it receives so the test can assert forwarding.
type textCaptureRoute struct {
	stubRoute
	gotKeys *[]string
}

func (r textCaptureRoute) CapturesInput() bool { return true }
func (r textCaptureRoute) CapturesText() bool  { return true }
func (r textCaptureRoute) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		*r.gotKeys = append(*r.gotKeys, k.Text)
	}
	return r, nil
}

// In a literal text field, q/Esc must be forwarded to the route (BackForward),
// not consumed by the shell as a back/quit.
func TestShell_BackKeyForwardedToTextField(t *testing.T) {
	var keys []string
	root := textCaptureRoute{stubRoute{title: "Form"}, &keys}
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{root})

	next, cmd := s.Update(tea.KeyPressMsg{Text: "q"})
	if cmd != nil {
		if _, isQuit := cmd().(tea.QuitMsg); isQuit {
			t.Fatal("q in a text field must not quit — it is forwarded to the route")
		}
	}
	if next.(shell.Shell).ActiveDepth() != 1 {
		t.Fatal("text field: q must not pop")
	}
	if len(keys) != 1 || keys[0] != "q" {
		t.Fatalf("text field should receive 'q', got %v", keys)
	}
}

func TestShell_switchRoute_pushesAtRootThenReplaces(t *testing.T) {
	var rootInit, aInit, bInit int
	root := initCountRoute{stubRoute{title: "Worktime"}, &rootInit}
	routeA := initCountRoute{stubRoute{title: "Woche"}, &aInit}
	routeB := initCountRoute{stubRoute{title: "Stats"}, &bInit}

	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{root})

	// At root (depth 1): SwitchRouteMsg pushes -> depth 2, crumb tip "Woche".
	next, _ := s.Update(shell.SwitchRouteMsg{Route: routeA})
	sh := next.(shell.Shell)
	if sh.ActiveDepth() != 2 {
		t.Fatalf("after switch at root depth = %d, want 2", sh.ActiveDepth())
	}
	if aInit != 1 { // the Shell must Init the pushed route (stub Init returns nil)
		t.Fatalf("switch at root should Init the new route (aInit=%d)", aInit)
	}

	// In a sibling (depth 2): SwitchRouteMsg replaces top -> stays depth 2.
	next2, _ := sh.Update(shell.SwitchRouteMsg{Route: routeB})
	sh2 := next2.(shell.Shell)
	if sh2.ActiveDepth() != 2 {
		t.Fatalf("after lateral switch depth = %d, want 2 (replace, not push)", sh2.ActiveDepth())
	}
	if bInit != 1 {
		t.Fatalf("lateral switch should Init the replacement (bInit=%d)", bInit)
	}
}

// --- docs back double-pop regression (end-to-end through the Shell) ---------

// chainDocSrv serves three wikilink-chained docs (Alpha->Bravo->Charlie) so the
// test can drive the REAL docs route into a viewStack of depth 2 via the public
// Update path. Each body carries a unique sentinel so the test can tell which
// doc the viewer is rendering (a title is ambiguous: a body links to the next
// doc, so that title appears as a label even while reading the current doc).
func chainDocSrv(t *testing.T) (*apiclient.Client, func()) {
	t.Helper()
	all := []domain.Document{
		{ID: "d1", Type: domain.DocFree, Path: "docs/a", Title: "Alpha", Body: "BODY_OF_ALPHA\n\n[[docs/b]]"},
		{ID: "d2", Type: domain.DocFree, Path: "docs/b", Title: "Bravo", Body: "BODY_OF_BRAVO\n\n[[docs/c]]"},
		{ID: "d3", Type: domain.DocFree, Path: "docs/c", Title: "Charlie", Body: "BODY_OF_CHARLIE"},
	}
	stored := map[string]domain.Document{}
	for _, d := range all {
		stored[d.ID] = d
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/documents", func(w http.ResponseWriter, _ *http.Request) {
		list := make([]domain.Document, 0, len(all))
		for _, d := range all {
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
	mux.HandleFunc("GET /api/v1/documents/{id}/backlinks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.BacklinkRef{})
	})
	srv := httptest.NewServer(mux)
	return apiclient.New(srv.URL, "tok"), srv.Close
}

// drainRoute runs cmd to completion against the route, acting as the bubbletea
// runtime (feeding every produced msg back). It Views after each step to size
// the modeView overlay. Batches are fanned out; only the last non-nil child cmd
// is chained — enough for the single-docViewMsg-per-navigation chain here.
func drainRoute(t *testing.T, r shell.Route, cmd tea.Cmd) shell.Route {
	t.Helper()
	for i := 0; cmd != nil && i < 20; i++ {
		msg := cmd()
		if msg == nil {
			break
		}
		if b, ok := msg.(tea.BatchMsg); ok {
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

// docsRouteAtDepth2 returns the real docs route reading Charlie (d3) with d1,d2
// on its internal wikilink viewStack.
func docsRouteAtDepth2(t *testing.T) shell.Route {
	t.Helper()
	c, stop := chainDocSrv(t)
	t.Cleanup(stop)
	var r shell.Route = docs.NewRoute(c, nil, nil, theme.Default, "alice")
	frame := shell.Frame{Width: 80, Height: 24, Pal: theme.Default}

	r = drainRoute(t, r, r.Init()) // load list
	r.View(frame)

	var cmd tea.Cmd
	r, cmd = r.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // open Alpha (d1)
	r = drainRoute(t, r, cmd)
	if !strings.Contains(r.View(frame), "BODY_OF_ALPHA") {
		t.Fatal("setup: expected to be reading Alpha (d1)")
	}
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})     // focus [[docs/b]]
	r, cmd = r.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // follow -> Bravo (d2)
	r = drainRoute(t, r, cmd)
	if !strings.Contains(r.View(frame), "BODY_OF_BRAVO") {
		t.Fatal("setup: expected to follow to Bravo (d2)")
	}
	r, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyTab})     // focus [[docs/c]]
	r, cmd = r.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // follow -> Charlie (d3)
	r = drainRoute(t, r, cmd)
	if !strings.Contains(r.View(frame), "BODY_OF_CHARLIE") {
		t.Fatal("setup: expected to follow to Charlie (d3)")
	}
	return r
}

// TestShell_docsBackPopsExactlyOneLevel is the end-to-end guard for the
// double-pop bug: pressing q once while reading Charlie (viewStack [d1,d2]) must
// pop EXACTLY ONE level back to Bravo (d2), staying in modeView. ResolveBack
// calls the route's Back() once to probe and the shell calls it again to apply;
// if the adapter's Back() mutates its receiver the probe pops a level too, so q
// skips Bravo and lands on Alpha (d1).
func TestShell_docsBackPopsExactlyOneLevel(t *testing.T) {
	r := docsRouteAtDepth2(t)
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{r})
	next, _ := s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	sh := next.(shell.Shell)

	// One q-press through the shell back-chain. The BackRoute branch returns a
	// loadDocNoPush cmd (-> docViewMsg) which the runtime would deliver; drain it
	// through the Shell so the revealed doc actually renders.
	var model tea.Model = sh
	var cmd tea.Cmd
	model, cmd = model.Update(tea.KeyPressMsg{Text: "q"})
	model = drainShell(t, model, cmd)
	v := model.(shell.Shell).View().Content

	if !strings.Contains(v, "BODY_OF_BRAVO") {
		t.Fatalf("q must pop exactly one level to Bravo (d2):\n%s", v)
	}
	if strings.Contains(v, "BODY_OF_ALPHA") {
		t.Fatalf("q skipped a level to Alpha (d1) — the docs adapter's Back() mutated on the ResolveBack probe:\n%s", v)
	}
}

// hiderRoute is a depth-2 child route that suppresses the breadcrumb.
type hiderRoute struct{ stubRoute }

func (h hiderRoute) HideBreadcrumb() bool { return true }
func (h hiderRoute) Update(msg tea.Msg) (shell.Route, tea.Cmd) {
	_, cmd := h.stubRoute.Update(msg)
	return h, cmd
}

func TestShell_BreadcrumbHiddenWhenRouteOptsOut(t *testing.T) {
	// Build a Shell with a root; push a hiderRoute so the nav-stack is at depth 2
	// and Crumbs() yields two entries (root title + child title).
	root := stubRoute{title: "Worktime"}
	child := hiderRoute{stubRoute{title: "Woche"}}
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{root})
	next, _ := s.Update(shell.PushRouteMsg{Route: child})
	sh := next.(shell.Shell)
	if sh.ActiveDepth() != 2 {
		t.Fatalf("setup: expected depth 2, got %d", sh.ActiveDepth())
	}
	// Give a real window size so View renders chrome.
	sized, _ := sh.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	out := sized.(shell.Shell).View().Content
	if strings.Contains(out, "›") {
		t.Fatalf("breadcrumb separator must be absent when top hides it:\n%s", out)
	}
}

func TestShell_BreadcrumbShownForPlainRoute(t *testing.T) {
	// Same setup but a plain stub (no HideBreadcrumb) — breadcrumb must appear.
	root := stubRoute{title: "Worktime"}
	child := stubRoute{title: "Woche"}
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{root})
	next, _ := s.Update(shell.PushRouteMsg{Route: child})
	sh := next.(shell.Shell)
	if sh.ActiveDepth() != 2 {
		t.Fatalf("setup: expected depth 2, got %d", sh.ActiveDepth())
	}
	sized, _ := sh.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	out := sized.(shell.Shell).View().Content
	if !strings.Contains(out, "›") {
		t.Fatalf("breadcrumb separator expected for a non-hider at depth 2:\n%s", out)
	}
}

// drainShell runs cmd to completion against the Shell, feeding produced msgs
// back (the runtime's job). Batches are fanned out; the last non-nil child cmd
// is chained.
func drainShell(t *testing.T, m tea.Model, cmd tea.Cmd) tea.Model {
	t.Helper()
	for i := 0; cmd != nil && i < 20; i++ {
		msg := cmd()
		if msg == nil {
			break
		}
		if b, ok := msg.(tea.BatchMsg); ok {
			var next tea.Cmd
			for _, c := range b {
				if c == nil {
					continue
				}
				cm := c()
				if cm == nil {
					continue
				}
				var nc tea.Cmd
				m, nc = m.Update(cm)
				if nc != nil {
					next = nc
				}
			}
			cmd = next
		} else {
			m, cmd = m.Update(msg)
		}
		m.(shell.Shell).View()
	}
	return m
}

// TestShell_helpOverlay exercises renderHelp (0% coverage) by pressing '?'
// to open the help overlay and then calling View().
func TestShell_helpOverlay(t *testing.T) {
	s := newShell()
	// Give the shell a window size so overlay rendering has dimensions.
	next, _ := s.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	s = next.(shell.Shell)

	// Press '?' to open the help panel.
	next2, _ := s.Update(tea.KeyPressMsg{Text: "?"})
	s = next2.(shell.Shell)

	// View must call renderHelp without panicking.
	out := s.View().Content
	if !strings.Contains(out, "Tastatur") && !strings.Contains(out, "Global") {
		// renderHelp produces a help overlay with "Tastatur" or "Global" section.
		t.Error("help overlay should contain 'Tastatur' or 'Global' section header")
	}

	// Press '?' again to close it.
	next3, _ := s.Update(tea.KeyPressMsg{Text: "?"})
	s = next3.(shell.Shell)
	_ = s.View() // should not panic with helpOpen=false
}

type paletteStub struct {
	stubRoute
	actions []shell.PaletteEntry
}

func (p paletteStub) PaletteEntries() []shell.PaletteEntry { return p.actions }

func TestShell_paletteMergesProviderEntriesFirst(t *testing.T) {
	action := shell.PaletteEntry{Label: "Startzeit anpassen", Action: func() tea.Msg { return nil }}
	provider := paletteStub{stubRoute{title: "Worktime"}, []shell.PaletteEntry{action}}
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{provider, stubRoute{title: "Docs"}})

	next, _ := s.Update(tea.KeyPressMsg{Text: ":"})
	f := next.(shell.Shell).Palette().Filtered()
	if len(f) != 3 {
		t.Fatalf("want 3 entries (1 action + 2 tabs), got %d: %v", len(f), f)
	}
	if f[0].Label != "Startzeit anpassen" {
		t.Fatalf("action should be first, got %q", f[0].Label)
	}
}

func TestShell_paletteWithoutProvider(t *testing.T) {
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{
		stubRoute{title: "Home"}, stubRoute{title: "Docs"},
	})
	next, _ := s.Update(tea.KeyPressMsg{Text: ":"})
	if f := next.(shell.Shell).Palette().Filtered(); len(f) != 2 {
		t.Fatalf("want 2 nav entries, got %d", len(f))
	}
}

type dynPaletteStub struct {
	stubRoute
	mk func() []shell.PaletteEntry
}

func (d dynPaletteStub) PaletteEntries() []shell.PaletteEntry { return d.mk() }

func TestShell_paletteRebuiltOnEachOpen(t *testing.T) {
	on := false
	pp := dynPaletteStub{stubRoute{title: "Worktime"}, func() []shell.PaletteEntry {
		if !on {
			return nil
		}
		return []shell.PaletteEntry{{Label: "Aktion", Action: func() tea.Msg { return nil }}}
	}}
	s := shell.New(nil, "alice", theme.Default).WithTabs([]shell.Route{pp})

	// First open while idle: only the single nav entry.
	n1, _ := s.Update(tea.KeyPressMsg{Text: ":"})
	if got := len(n1.(shell.Shell).Palette().Filtered()); got != 1 {
		t.Fatalf("idle: want 1 nav entry, got %d", got)
	}
	// Dismiss, flip the flag, reopen: the action now appears -> 2 entries.
	sh := n1.(shell.Shell)
	_, cmd := sh.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	closed, _ := sh.Update(cmd())
	on = true
	n2, _ := closed.(shell.Shell).Update(tea.KeyPressMsg{Text: ":"})
	if got := len(n2.(shell.Shell).Palette().Filtered()); got != 2 {
		t.Fatalf("after toggle: want 2 entries, got %d", got)
	}
}
