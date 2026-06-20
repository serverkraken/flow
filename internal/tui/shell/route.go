// Package shell is the flow sidekick-shell: a top tabstrip, a :-command
// palette, and a per-tab nav-stack router. Each screen implements Route and
// is hosted by the Shell tea.Model (or, chrome-less, by RouteHost).
package shell

import (
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/theme"
	"github.com/serverkraken/flow/internal/tui/ui/keyhint"
)

// Frame is the usable content area handed to a Route's View, after the shell
// chrome (header, tabstrip, breadcrumb, footer) has been subtracted. It also
// carries the active palette so routes never reach for a global.
type Frame struct {
	Width  int
	Height int
	Pal    theme.Palette
}

// Route is the contract every hosted screen, drill-down, and modal implements.
// Update returns the (possibly swapped) Route so the nav-stack can replace it
// without type assertions; to navigate, a Route returns a command emitting
// PushRouteMsg/PopRouteMsg which the Shell applies to the active stack.
type Route interface {
	Title() string
	Init() tea.Cmd
	Update(msg tea.Msg) (Route, tea.Cmd)
	View(f Frame) string
	KeyHints() []keyhint.Hint
}

// PushRouteMsg asks the Shell to push Route onto the active tab's nav-stack
// (a drill-down). Emit it as a tea.Cmd from a Route's Update.
type PushRouteMsg struct{ Route Route }

// PopRouteMsg asks the Shell to pop the active tab's nav-stack (a back).
type PopRouteMsg struct{}

// SwitchRouteMsg performs a lateral move: if the active tab's nav-stack is at
// its root (depth 1) it pushes Route (entering the sibling group); otherwise it
// replaces the top Route, so switching between siblings never deepens the
// stack. Emit it as a tea.Cmd from a Route's Update (see wtnav.Registry.Nav).
type SwitchRouteMsg struct{ Route Route }

// SwitchTabMsg asks the Shell to activate the tab whose nav-stack ROOT route
// title equals Title (a no-op if no tab matches). Emit it from a Route to drill
// laterally into another tab (e.g. the Home dashboard jumping to Worktime).
type SwitchTabMsg struct{ Title string }

// InputCapturer lets a route signal it is in text-entry mode. While the active
// tab's top route reports CapturesInput()==true, the Shell forwards every key
// to it instead of consuming digits/Tab/Esc/q/:/? as global shortcuts. It is an
// optional interface — routes that don't implement it keep the global shortcuts.
type InputCapturer interface{ CapturesInput() bool }

// FullScreener lets the active tab's top route take over the whole terminal,
// suppressing the shell's header/tabstrip/breadcrumb/footer. Used by the Docs
// viewer for an immersive read (matches main's old ModeView takeover). Combine
// with InputCapturer so the shell also forwards every key to the route.
type FullScreener interface{ FullScreen() bool }

// TextCapturer lets a route signal it is in a *literal text-entry* field, where
// even the back keys (q/Esc) belong to the route (q is typed, the route's own
// Esc cancels the field). Narrower than InputCapturer, which also covers
// non-text key forwarding (e.g. the doc viewer's Tab/scroll).
type TextCapturer interface{ CapturesText() bool }

// Backer lets a route resolve one level of "back" within its own internal state
// (e.g. document view → list, clear an active filter) before the frame pops the
// nav-stack or quits. ok=false means "nothing internal — frame decides".
//
// Contract: Back() MUST NOT mutate its receiver. It is called twice per back
// key — once by ResolveBack to probe `handled`, then again by the Shell to
// apply — so it has to be referentially transparent: return the new (possibly
// swapped) Route carrying the resolved state rather than mutating in place. A
// receiver-mutating implementation double-applies the back step (e.g. pops a
// drill-stack twice), skipping a level.
type Backer interface{ Back() (Route, tea.Cmd, bool) }
