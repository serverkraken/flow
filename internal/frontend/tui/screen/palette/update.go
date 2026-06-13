package palette

// Palette update path — Update-Dispatch plus die spezifischen Key-
// Handler (handleNormalKey / handleFilterKey), Pin-/Section-/Dispatch-
// Commands und der async Loader. Split aus model.go (Skill §No-
// Monoliths): Update-Routing ist ein eigener Verantwortungs-Cluster
// neben Type-Definitionen/Construction (model.go) und Rendering
// (render.go). Same-package — keine Sichtbarkeitsänderung.

import (
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
)

func (m Model) loadCmd() tea.Cmd {
	r := m.reader
	return func() tea.Msg {
		snap, err := r.Load()
		return loadedMsg{snapshot: snap, err: err}
	}
}

// Update handles messages for the palette screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case loadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.snapshot != nil {
			m.all = msg.snapshot.Entries
			m.stats = msg.snapshot.Stats
			m.session = msg.snapshot.SessionName
		}
		m.applyFilter()
		return m, nil

	case dispatchedMsg:
		// External action handed off to tmux — stay in palette, toast.
		// On error, switch the toast to danger styling so the user
		// knows the action didn't actually run.
		var t toast.Model
		if msg.err != nil {
			t = toast.NewDanger("Aktion fehlgeschlagen: "+msg.err.Error(), m.pal)
		} else {
			t = toast.New("Ausgeführt: "+msg.label, 2*time.Second, m.pal)
		}
		m.toast = &t
		// Standalone-Mode: nach erfolgreichem Dispatch quittet der Popup.
		// Bei Fehler bleibt das Programm offen, damit der User die
		// Danger-Toast-Message sehen kann.
		if m.mode == ModeStandalone && msg.err == nil {
			return m, tea.Batch(t.Init(), tea.Quit)
		}
		return m, t.Init()

	case persistDoneMsg:
		if msg.err != nil {
			t := toast.NewWarning("persist fehlgeschlagen: "+msg.err.Error(), m.pal)
			m.toast = &t
			return m, tea.Batch(m.loadCmd(), t.Init())
		}
		return m, m.loadCmd()

	case toast.DismissedMsg:
		m.toast = nil
		return m, nil

	case tea.KeyPressMsg:
		if m.filter.Focused() {
			return m.handleFilterKey(msg)
		}
		return m.handleNormalKey(msg)
	}
	return m, nil
}

// handleNormalKey routes a key when the filter does not have focus.
// The function is a flat dispatch table over a fixed command set; a
// split would hide the structure behind helper indirection.
//
//nolint:gocyclo
func (m Model) handleNormalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := msg.String()
	switch s {
	case "/":
		m.filter.Focus()
		return m, textinput.Blink
	case "esc":
		// Standalone popup: esc on empty/normal mode closes the popup.
		// Embedded: no-op — the sidekick host owns the quit key.
		if m.mode == ModeStandalone {
			return m, tea.Quit
		}
		return m, nil
	case "j", "down":
		if m.cursor < len(m.visible)-1 {
			m.cursor++
			m.ensureCursorVisible()
		}
		return m, nil
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
			m.ensureCursorVisible()
		}
		return m, nil
	case "G":
		m.cursor = max(0, len(m.visible)-1)
		m.ensureCursorVisible()
		return m, nil
	case "g":
		m.cursor = 0
		m.ensureCursorVisible()
		return m, nil
	case "pgdown", "ctrl+d":
		m.cursor = min(len(m.visible)-1, m.cursor+m.maxVisible())
		m.ensureCursorVisible()
		return m, nil
	case "pgup", "ctrl+u":
		m.cursor = max(0, m.cursor-m.maxVisible())
		m.ensureCursorVisible()
		return m, nil
	case "enter":
		if len(m.visible) > 0 {
			return m, m.dispatch(m.visible[m.cursor])
		}
		return m, nil
	case "]":
		m.jumpSection(+1)
		return m, nil
	case "[":
		m.jumpSection(-1)
		return m, nil
	case ".":
		if len(m.visible) > 0 {
			return m, m.togglePin(m.visible[m.cursor])
		}
		return m, nil
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		n := int(s[0] - '0')
		if n-1 < len(m.visible) {
			m.cursor = n - 1
			m.ensureCursorVisible()
			return m, m.dispatch(m.visible[m.cursor])
		}
		return m, nil
	}

	// Standalone popup: q closes the window. Guard must come before the
	// type-to-filter fallthrough so 'q' does not get swallowed into the
	// filter in standalone mode. Embedded: 'q' falls through to filter.
	if s == "q" && m.mode == ModeStandalone {
		return m, tea.Quit
	}

	// Type-to-filter: any other single printable character auto-focuses
	// the filter and routes the keystroke into it. Saves the explicit
	// "/" before searching. Special keys (tab, ctrl-combos, etc.) have
	// multi-char names and fall through unhandled.
	if len(s) == 1 && s[0] >= ' ' && s[0] < 127 {
		m.filter.Focus()
		var cmd tea.Cmd
		prev := m.filter.Value()
		m.filter, cmd = m.filter.Update(msg)
		if m.filter.Value() != prev {
			m.applyFilter()
		}
		return m, tea.Batch(cmd, textinput.Blink)
	}
	return m, nil
}

func (m Model) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Esc with a non-empty filter clears the value AND blurs so j/k
		// (which are normal-mode navigation keys) reach the list. The
		// older two-stage "clear, keep focus, second esc quits" combined
		// with the type-to-filter auto-focus made it impossible to
		// navigate without re-pressing `/` — and any printable key then
		// re-trapped the user. Esc on an empty filter still quits.
		if m.filter.Value() != "" {
			m.filter.SetValue("")
			m.filter.Blur()
			m.applyFilter()
			return m, nil
		}
		// Empty filter + esc: standalone popup → quit; embedded → blur
		// and stay (the sidekick host owns program quit).
		m.filter.Blur()
		if m.mode == ModeStandalone {
			return m, tea.Quit
		}
		return m, nil
	case "enter":
		m.filter.Blur()
		if len(m.visible) > 0 {
			return m, m.dispatch(m.visible[m.cursor])
		}
		return m, nil
	}
	var cmd tea.Cmd
	prev := m.filter.Value()
	m.filter, cmd = m.filter.Update(msg)
	if m.filter.Value() != prev {
		m.applyFilter()
	}
	return m, cmd
}

// togglePin flips the pin bit asynchronously. The persist runs inside a
// tea.Cmd so a hung disk doesn't freeze the bubbletea event loop. On
// failure the cmd returns a persistDoneMsg whose Update branch surfaces
// a warning toast and reloads; on success it returns loadedMsg directly
// so the UI re-renders against authoritative state in one trip.
func (m Model) togglePin(target domain.PaletteEntry) tea.Cmd {
	w := m.writer
	r := m.reader
	return func() tea.Msg {
		if err := w.TogglePin(target); err != nil {
			return persistDoneMsg{err: err}
		}
		snap, err := r.Load()
		return loadedMsg{snapshot: snap, err: err}
	}
}

// jumpSection moves the cursor to the first entry of the next (dir=+1)
// or previous (dir=-1) section. When already mid-section, [ first jumps
// to the top of the current section, then to the previous one on a
// second press.
func (m *Model) jumpSection(dir int) {
	if len(m.visible) == 0 {
		return
	}
	sectionAt := func(i int) string { return m.stats.EffectiveSection(m.visible[i]) }
	cur := sectionAt(m.cursor)
	if dir > 0 {
		for i := m.cursor + 1; i < len(m.visible); i++ {
			if sectionAt(i) != cur {
				m.cursor = i
				m.ensureCursorVisible()
				return
			}
		}
		return
	}
	start := m.cursor
	for start > 0 && sectionAt(start-1) == cur {
		start--
	}
	if start < m.cursor {
		m.cursor = start
		m.ensureCursorVisible()
		return
	}
	if start == 0 {
		return
	}
	prev := sectionAt(start - 1)
	target := start - 1
	for target > 0 && sectionAt(target-1) == prev {
		target--
	}
	m.cursor = target
	m.ensureCursorVisible()
}

// dispatch records the pick via the writer (asynchronously inside the
// returned cmd so a hung disk doesn't freeze Update), then either:
//
//	(a) emits a SwitchScreenMsg if the action matches the goto.sh deep-link
//	    pattern — the sidekick root handles it as an in-process tab switch,
//	    no subshell, no flow restart;
//
//	(b) hands the action off to tmux via run-shell -b and returns a
//	    dispatchedMsg so the palette can toast confirmation while staying
//	    open. RunTmuxAction errors (tmux server down, malformed action) now
//	    surface as a danger toast — previously they were silently dropped
//	    and the user saw "ausgeführt" even when nothing ran. The earlier
//	    `sleep 0.15` prefix is gone: it was an undocumented workaround
//	    that introduced latency without solving any documented race.
//
// Pre-F-WAVE-1 every dispatch ended in tea.Quit, killing flow and forcing
// the sidekick pane to flicker on each action.
func (m Model) dispatch(e domain.PaletteEntry) tea.Cmd {
	w := m.writer
	tm := m.tmux
	entry := e

	if m.mode == ModeEmbedded {
		// SwitchScreenMsg ist nur im Sidekick-Embed sinnvoll; standalone
		// Popup hat keinen Parent, der das Msg verarbeitet.
		if matches := gotoScreenRe.FindStringSubmatch(e.Action); matches != nil {
			screen := matches[1]
			if domain.IsValidScreen(screen) {
				return func() tea.Msg {
					_ = w.Mark(entry) // persist err is non-fatal for screen-switch
					return SwitchScreenMsg{Screen: screen}
				}
			}
		}
	}

	action := e.Action
	label := e.Label
	return func() tea.Msg {
		_ = w.Mark(entry) // persist err is folded into dispatchedMsg.err below if RunTmuxAction also fails
		err := tm.RunTmuxAction(action)
		return dispatchedMsg{label: label, err: err}
	}
}
