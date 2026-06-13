// Package shared — small helpers callable from the .templ files in this
// package. Kept minimal; per the per-handler-Deps convention these
// templates never touch I/O or business logic.
package shared

import "strconv"

// liveBannerStartedAttr formats a Unix timestamp as a string for the
// `data-started` attribute on `.live-elapsed`. Extracted so the templ
// expression stays a single function call (templ attr values can't
// embed arbitrary Go arithmetic). Used only by the SSE tick handler
// in worktime/today.templ to recompute elapsed client-side.
func liveBannerStartedAttr(unix int64) string {
	return strconv.FormatInt(unix, 10)
}
