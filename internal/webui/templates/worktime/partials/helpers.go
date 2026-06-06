// helpers.go — small string helpers usable from the templ files in
// this package. Kept minimal; per the per-handler-Deps convention these
// templates never touch I/O or business logic.
package partials

import "strconv"

// intToStr renders int64 as decimal for inline interpolation in
// hx-headers JSON or hidden version inputs.
func intToStr(v int64) string { return strconv.FormatInt(v, 10) }
