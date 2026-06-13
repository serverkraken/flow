// Package partials holds the templ-generated worktime partials plus the
// tiny string helpers they call inline (e.g. intToStr for hx-headers
// JSON, hidden version inputs). The package never touches I/O or
// business logic — view-models are built upstream in the handlers
// package per the per-handler-Deps convention.
package partials

import "strconv"

// intToStr renders int64 as decimal for inline interpolation in
// hx-headers JSON or hidden version inputs.
func intToStr(v int64) string { return strconv.FormatInt(v, 10) }
