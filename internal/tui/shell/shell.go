package shell

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/breadcrumb"
	"github.com/serverkraken/flow/internal/tui/ui/grammar"
	"github.com/serverkraken/flow/internal/tui/ui/header"
	"github.com/serverkraken/flow/internal/tui/ui/help"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
	"github.com/serverkraken/flow/internal/tui/ui/overlay"
	"github.com/serverkraken/flow/internal/tui/ui/tabstrip"
	"github.com/serverkraken/flow/internal/tui/ui/titlebox"
)

// Shell is the root tea.Model for `flow ui`.
type Shell struct {
	tabs      []*NavStack
	activeTab int

	palette     Palette
	navEntries  []PaletteEntry
	paletteOpen bool
	helpOpen    bool

	width, height int
	user          string
	pal           theme.Palette

	client *apiclient.Client
	events <-chan apiclient.ClientEvent
}

type shellEventsReadyMsg struct{ ch <-chan apiclient.ClientEvent }

// EventMsg carries one server SSE event, broadcast by the Shell to every
// tab's top route so routes can refresh. Exported so route packages outside
// `shell` can type-switch on it.
type (
	EventMsg    struct{ Ev apiclient.ClientEvent }
	shellErrMsg struct{ err error }
)

// tabSwitchMsg requests a tab change (emitted by palette entries).
type tabSwitchMsg int

// New creates a Shell with a single Home tab. client may be nil (tests).
// pal is the visual palette (theme.Load()).
func New(client *apiclient.Client, user string, pal theme.Palette) Shell {
	s := Shell{user: user, pal: pal, client: client}
	return s.WithTabs([]Route{NewHomeRoute(client, pal, user)})
}

// WithTabs (re)builds the tab set; each Route becomes a stack root and gets a
// palette entry. Used in New, tests, and future M3c wiring.
func (s Shell) WithTabs(routes []Route) Shell {
	s.tabs = make([]*NavStack, len(routes))
	entries := make([]PaletteEntry, len(routes))
	for i, r := range routes {
		s.tabs[i] = NewNavStack(r)
		idx := i
		entries[i] = PaletteEntry{Label: r.Title(), Action: func() tea.Msg { return tabSwitchMsg(idx) }}
	}
	s.navEntries = entries
	if s.activeTab >= len(s.tabs) {
		s.activeTab = 0
	}
	return s
}

// WithActiveTab sets the initially-visible tab (clamped to range). Used by the
// `flow ui [tab]` deep-link to start on Worktime/Docs instead of Home.
func (s Shell) WithActiveTab(i int) Shell {
	if i < 0 || i >= len(s.tabs) {
		i = 0
	}
	s.activeTab = i
	return s
}

// Accessors (used by tests + cmd).
func (s Shell) Width() int        { return s.width }
func (s Shell) Height() int       { return s.height }
func (s Shell) ActiveTab() int    { return s.activeTab }
func (s Shell) PaletteOpen() bool { return s.paletteOpen }
func (s Shell) ActiveDepth() int  { return s.tabs[s.activeTab].Len() }

// Palette returns the current palette model (used by tests to inspect merged
// entries after the palette is opened).
func (s Shell) Palette() Palette { return s.palette }

// buildPalette gathers the active route's contextual actions (if it implements
// PaletteProvider) ahead of the static tab-navigation entries, fresh on each
// open so the action set reflects current route state.
func (s Shell) buildPalette() Palette {
	var entries []PaletteEntry
	if pp, ok := s.tabs[s.activeTab].Top().(PaletteProvider); ok {
		entries = append(entries, pp.PaletteEntries()...)
	}
	entries = append(entries, s.navEntries...)
	return NewPalette(entries)
}

// Init loads the initial (active) tab's route and subscribes to SSE if a
// client is present. Without initing the active tab route, a root tab never
// loads its data or starts its tick loop until an SSE event happens to arrive.
func (s Shell) Init() tea.Cmd {
	cmds := []tea.Cmd{s.initActiveTab()}
	if s.client != nil {
		cl := s.client
		cmds = append(cmds, func() tea.Msg {
			ch, err := cl.Events(context.Background())
			if err != nil {
				return shellErrMsg{err}
			}
			return shellEventsReadyMsg{ch}
		})
	}
	return tea.Batch(cmds...)
}

// initActiveTab returns the active tab's top route's Init cmd so the tab loads
// (and starts any tick loop) when it first becomes visible.
func (s Shell) initActiveTab() tea.Cmd { return s.tabs[s.activeTab].Top().Init() }

// switchTo activates tab i (if valid and different) and returns its route's
// Init cmd so the newly-visible tab (re)loads.
func (s *Shell) switchTo(i int) tea.Cmd {
	if i < 0 || i >= len(s.tabs) || i == s.activeTab {
		return nil
	}
	s.activeTab = i
	return s.initActiveTab()
}

// Update is the central dispatcher.
func (s Shell) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		s.width, s.height = msg.Width, msg.Height
		var cmds []tea.Cmd
		for _, ns := range s.tabs {
			if c := ns.UpdateTop(msg); c != nil {
				cmds = append(cmds, c)
			}
		}
		return s, tea.Batch(cmds...)

	case shellEventsReadyMsg:
		s.events = msg.ch
		return s, waitForShellEvent(msg.ch)
	case EventMsg:
		var cmds []tea.Cmd
		for _, ns := range s.tabs { // broadcast to all tabs
			if c := ns.UpdateTop(msg); c != nil {
				cmds = append(cmds, c)
			}
		}
		cmds = append(cmds, waitForShellEvent(s.events))
		return s, tea.Batch(cmds...)
	case shellErrMsg:
		return s, nil // swallow for M3b; M3c can toast

	case PushRouteMsg:
		s.tabs[s.activeTab].Push(msg.Route)
		return s, msg.Route.Init()
	case PopRouteMsg:
		s.tabs[s.activeTab].Pop()
		// Re-Init the revealed route so a root that paused background work while
		// drilled-over (e.g. Today's live clock) resumes — matching the Init
		// contract of push/switch/tab-switch.
		return s, s.tabs[s.activeTab].Top().Init()

	case SwitchRouteMsg:
		ns := s.tabs[s.activeTab]
		if ns.Len() == 1 {
			ns.Push(msg.Route)
		} else {
			ns.ReplaceTop(msg.Route)
		}
		return s, msg.Route.Init()

	case PaletteSelectedMsg:
		s.paletteOpen = false
		return s.Update(msg.Entry.Action())
	case PaletteDismissedMsg:
		s.paletteOpen = false
		return s, nil
	case tabSwitchMsg:
		cmd := s.switchTo(int(msg))
		return s, cmd

	case SwitchTabMsg:
		for i, ns := range s.tabs {
			if ns.Root().Title() == msg.Title {
				return s, s.switchTo(i)
			}
		}
		return s, nil

	case tea.KeyPressMsg:
		return s.handleKey(msg)
	}

	// default: forward to active route
	return s, s.tabs[s.activeTab].UpdateTop(msg)
}

func (s Shell) handleKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if s.paletteOpen {
		var cmd tea.Cmd
		s.palette, cmd = s.palette.Update(k)
		return s, cmd
	}
	// A text-entry route owns most keys — but NOT the back keys, so q/Esc always
	// reach the back chain below (a doc viewer's CapturesInput()==true yet its
	// q/Esc must still walk back; ResolveBack then forwards or pops as needed).
	if ic, ok := s.tabs[s.activeTab].Top().(InputCapturer); ok && ic.CapturesInput() && !s.helpOpen && !grammar.Back.Matches(k) {
		return s, s.tabs[s.activeTab].UpdateTop(k)
	}
	switch {
	case k.Code == 'c' && k.Mod == tea.ModCtrl:
		return s, tea.Quit
	case grammar.Back.Matches(k):
		top := s.tabs[s.activeTab].Top()
		switch ResolveBack(top, s.tabs[s.activeTab].Len(), s.helpOpen || s.paletteOpen) {
		case BackOverlay:
			s.helpOpen = false
			s.paletteOpen = false
			return s, nil
		case BackForward:
			return s, s.tabs[s.activeTab].UpdateTop(k)
		case BackRoute:
			if b, ok := top.(Backer); ok {
				nr, cmd, _ := b.Back()
				s.tabs[s.activeTab].ReplaceTop(nr)
				return s, cmd
			}
			return s, nil
		case BackPop:
			s.tabs[s.activeTab].Pop()
			// Re-Init the revealed route, mirroring the PopRouteMsg path: a parent
			// drilled-over (e.g. Woche under a daydetail Nachbuchen) must refresh
			// when revealed, not show stale data until manually reloaded.
			return s, s.tabs[s.activeTab].Top().Init()
		case BackQuit:
			return s, tea.Quit
		}
		return s, nil
	case k.Text == ":":
		s.paletteOpen = true
		s.palette = s.buildPalette()
		return s, nil
	case k.Text == "?":
		s.helpOpen = !s.helpOpen
		return s, nil
	case k.Code == tea.KeyTab && k.Mod.Contains(tea.ModShift):
		cmd := s.switchTo((s.activeTab - 1 + len(s.tabs)) % len(s.tabs))
		return s, cmd
	case k.Code == tea.KeyTab:
		cmd := s.switchTo((s.activeTab + 1) % len(s.tabs))
		return s, cmd
	case len(k.Text) == 1 && k.Text[0] >= '1' && k.Text[0] <= '9':
		cmd := s.switchTo(int(k.Text[0] - '1'))
		return s, cmd
	default:
		return s, s.tabs[s.activeTab].UpdateTop(k)
	}
}

// View renders header + tabstrip + breadcrumb + body + footer.
func (s Shell) View() tea.View {
	// FullScreener: if the active top route requests full-screen and no
	// overlay is open, skip all chrome and render the route over the full
	// terminal area. Help/palette overlays take precedence: the guard below
	// skips fullscreen when either is open, falling through to the switch.
	if !s.helpOpen && !s.paletteOpen {
		top := s.tabs[s.activeTab].Top()
		if fs, ok := top.(FullScreener); ok && fs.FullScreen() {
			v := tea.NewView(top.View(Frame{Width: max(s.width, 1), Height: max(s.height, 1), Pal: s.pal}))
			v.AltScreen = true
			return v
		}
	}

	// Chrome strings are only needed for the non-fullscreen path, so build them
	// after the FullScreener early-return rather than discarding them per frame.
	titles := make([]string, len(s.tabs))
	for i, ns := range s.tabs {
		titles[i] = ns.Top().Title()
	}
	head := header.Render("flow", s.user, max(s.width, 1), s.pal)
	tabs := tabstrip.Render(titles, s.activeTab, max(s.width, 1), s.pal)
	crumbs := breadcrumb.Render(s.tabs[s.activeTab].Crumbs(), s.pal)
	if bh, ok := s.tabs[s.activeTab].Top().(BreadcrumbHider); ok && bh.HideBreadcrumb() {
		crumbs = ""
	}

	chrome := 2 // header + tabstrip
	if crumbs != "" {
		chrome++
	}
	chrome++ // footer
	contentH := s.height - chrome
	if contentH < 0 {
		contentH = 0
	}

	var body, footer string
	switch {
	case s.helpOpen:
		body = overlay.Render(s.renderHelp(), s.width, contentH)
		footer = keyhint.Render([]keyhint.Hint{{Key: "esc", Desc: "schließen"}}, s.pal)
	case s.paletteOpen:
		modalW := min(theme.DefaultBox, max(s.width-4, 10))
		modal := titlebox.Render("Palette", s.palette.View(modalW-2, s.pal), modalW, s.pal)
		body = overlay.Render(modal, s.width, contentH)
		footer = keyhint.Render([]keyhint.Hint{{Key: "enter", Desc: "wählen"}, {Key: "esc", Desc: "schließen"}}, s.pal)
	default:
		top := s.tabs[s.activeTab].Top()
		body = top.View(Frame{Width: s.width, Height: contentH, Pal: s.pal})
		footer = keyhint.Render(top.KeyHints(), s.pal)
	}

	parts := []string{head, tabs}
	if crumbs != "" {
		parts = append(parts, crumbs)
	}
	// Normalize the body to exactly contentH rows so the footer is always pinned
	// to the bottom of the terminal — routes that underfill their height budget
	// (Home/Woche/Stats/Frei/Export ignore Frame.Height; Today underfills when
	// sessions are few) must not leave the footer floating mid-screen, and a
	// route that overfills must not push it off the bottom.
	parts = append(parts, fitVertical(body, contentH), footer)
	v := tea.NewView(strings.Join(parts, "\n"))
	v.AltScreen = true
	return v
}

// fitVertical normalizes body to exactly h terminal rows — padding with blank
// rows below (content stays top-aligned) or truncating extra rows. This keeps
// the shell footer pinned to the bottom regardless of how tall the active route
// renders. Self-windowing routes (Today, Docs) already return <= h rows, so they
// are only padded, never clipped.
func fitVertical(body string, h int) string {
	if h <= 0 {
		return ""
	}
	lines := strings.Split(body, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (s Shell) renderHelp() string {
	top := s.tabs[s.activeTab].Top()
	keys := make([][2]string, 0, len(top.KeyHints())+5)
	for _, h := range top.KeyHints() {
		keys = append(keys, [2]string{h.Key, h.Desc})
	}
	sections := []help.Section{
		{Title: "Aktueller Screen", Keys: keys},
		{Title: "Global", Keys: [][2]string{
			{grammar.MoveUp.KeyLabel, grammar.MoveUp.Desc},
			{grammar.Back.KeyLabel + " / Esc", "zurück / beenden"},
			{"Tab / 1–9", "Tab wechseln"},
			{grammar.Search.KeyLabel, grammar.Search.Desc},
			{grammar.Help.KeyLabel, grammar.Help.Desc},
			{":", "Palette"},
		}},
	}
	return help.Render("Tastatur", sections, theme.KeyHintWidth, theme.DefaultBox, s.pal)
}

func waitForShellEvent(ch <-chan apiclient.ClientEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return EventMsg{Ev: ev}
	}
}
