// Package textscan centralises bufio.Scanner construction with a buffer
// large enough to tolerate the worst realistic input across flow's TSV /
// INI / palette readers. The default 64 KiB cap surfaces as bufio.ErrTooLong
// on a single very long line and brings the whole load down — a single
// corrupted byte sequence (no `\n` for a long stretch) shouldn't take the
// worktime UI offline.
package textscan

import (
	"bufio"
	"io"
)

// MaxLine is the per-line cap. 1 MiB is well above any sane palette /
// session / dayoff line and well below the memory we'd want to spend on
// a clearly corrupt file.
const MaxLine = 1 << 20

// New returns a bufio.Scanner configured with the package's larger
// buffer. Callers handle scanner errors as before; this just shifts the
// failure threshold up.
func New(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), MaxLine)
	return sc
}
