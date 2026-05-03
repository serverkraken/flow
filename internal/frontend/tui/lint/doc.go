// Package lint hosts custom static checks that don't fit the
// golangci-lint plugin model — typically rules that depend on counts
// or per-file baselines instead of an unconditional pattern match.
//
// The screen-baseline test (screen_baseline_test.go) is the
// design-system audit's "depguard for inline NewStyle": forbidigo
// can't tell a layout-chain NewStyle from a Foreground-only one, so
// instead we count NewStyle calls per screen file and fail when a
// PR exceeds the documented baseline. New layout-chain NewStyle
// requires updating the baseline visibly; the diff is the review
// gate. Reductions are encouraged but don't fail (the test logs the
// new lower number so the dev can ratchet the baseline down).
package lint
