package theme

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// WCAG 2.1 contrast thresholds.
const (
	WCAGNormalAA = 4.5 // body text — AA minimum
	WCAGLargeAA  = 3.0 // ≥18 pt or ≥14 pt bold; in TUI applies to
	// glyph-only signals (pace dots, progress bar cells).
)

// ContrastRatio returns the WCAG 2.1 contrast ratio of two #rrggbb
// hex strings. Range [1.0, 21.0]; higher means more contrast.
//
// Returns an error when either input fails hex parsing — keeps the
// caller's tests honest about typos in hardcoded palette literals.
func ContrastRatio(a, b string) (float64, error) {
	la, err := relativeLuminance(a)
	if err != nil {
		return 0, fmt.Errorf("contrast lhs %q: %w", a, err)
	}
	lb, err := relativeLuminance(b)
	if err != nil {
		return 0, fmt.Errorf("contrast rhs %q: %w", b, err)
	}
	hi, lo := la, lb
	if hi < lo {
		hi, lo = lo, hi
	}
	return (hi + 0.05) / (lo + 0.05), nil
}

// MustContrast wraps ContrastRatio for use in test fixtures and one-off
// scripts; panics on parse error.
func MustContrast(a, b string) float64 {
	r, err := ContrastRatio(a, b)
	if err != nil {
		panic(err)
	}
	return r
}

// relativeLuminance computes WCAG 2.1 relative luminance for a hex
// color string. Implements the sRGB → linear → weighted-sum pipeline
// straight from the W3C definition.
func relativeLuminance(hex string) (float64, error) {
	r, g, b, err := parseHex(hex)
	if err != nil {
		return 0, err
	}
	return 0.2126*linearize(r) + 0.7152*linearize(g) + 0.0722*linearize(b), nil
}

func linearize(c float64) float64 {
	if c <= 0.03928 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func parseHex(s string) (r, g, b float64, err error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return 0, 0, 0, fmt.Errorf("invalid hex %q (want #rrggbb)", s)
	}
	parse := func(part string) (float64, error) {
		v, err := strconv.ParseUint(part, 16, 8)
		if err != nil {
			return 0, fmt.Errorf("hex byte %q: %w", part, err)
		}
		return float64(v) / 255.0, nil
	}
	if r, err = parse(s[0:2]); err != nil {
		return
	}
	if g, err = parse(s[2:4]); err != nil {
		return
	}
	if b, err = parse(s[4:6]); err != nil {
		return
	}
	return
}
