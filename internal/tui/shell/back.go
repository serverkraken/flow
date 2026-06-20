package shell

// BackAction is what a back key (q/Esc) should do, decided by ResolveBack.
type BackAction int

const (
	BackForward BackAction = iota // route owns the key (literal text entry)
	BackOverlay                   // close the open help/palette overlay
	BackRoute                     // route handles it internally (Backer)
	BackPop                       // pop the nav-stack one level
	BackQuit                      // quit the program
)

// ResolveBack decides how a back key resolves for the active route, given the
// nav-stack depth and whether an overlay is open. Shell and RouteHost both call
// this; the host passes stackDepth=1 so it can only ever reach BackQuit.
func ResolveBack(top Route, stackDepth int, overlayOpen bool) BackAction {
	if overlayOpen {
		return BackOverlay
	}
	if tc, ok := top.(TextCapturer); ok && tc.CapturesText() {
		return BackForward
	}
	if b, ok := top.(Backer); ok {
		if _, _, handled := b.Back(); handled {
			return BackRoute
		}
	}
	if stackDepth > 1 {
		return BackPop
	}
	return BackQuit
}
