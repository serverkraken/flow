package textscan_test

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/textscan"
)

// TestNew_ScansLineWellBeyondDefaultBufioCap guards review finding T3:
// the package raises bufio's 64 KiB per-line cap to 1 MiB so a single
// long line in a TSV / INI / palette file doesn't tear the worktime UI
// offline. Without this test, a future "I'll just call bufio.NewScanner
// directly" simplification would silently regress the protection.
func TestNew_ScansLineWellBeyondDefaultBufioCap(t *testing.T) {
	cases := []struct {
		name string
		size int
	}{
		{"65 KiB — past bufio default", 65 * 1024},
		{"512 KiB — well past default", 512 * 1024},
		{"900 KiB — close to MaxLine", 900 * 1024},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			line := strings.Repeat("x", tc.size) + "\n"
			sc := textscan.New(strings.NewReader(line))
			if !sc.Scan() {
				t.Fatalf("Scan returned false; err=%v", sc.Err())
			}
			if got := len(sc.Bytes()); got != tc.size {
				t.Errorf("length: got %d, want %d", got, tc.size)
			}
		})
	}
}

// TestNew_RejectsBeyondMaxLine confirms the upper ceiling still
// engages — a 2 MiB line must surface bufio.ErrTooLong rather than
// silently allocating without bound.
func TestNew_RejectsBeyondMaxLine(t *testing.T) {
	huge := bytes.Repeat([]byte("x"), 2*textscan.MaxLine)
	sc := textscan.New(bytes.NewReader(huge))
	if sc.Scan() {
		t.Fatal("Scan must refuse a line > MaxLine")
	}
	if !errors.Is(sc.Err(), bufio.ErrTooLong) {
		t.Errorf("got %v, want ErrTooLong", sc.Err())
	}
}

func TestNew_HandlesMultipleShortLines(t *testing.T) {
	sc := textscan.New(strings.NewReader("a\nbb\nccc\n"))
	var got []string
	for sc.Scan() {
		got = append(got, sc.Text())
	}
	if sc.Err() != nil {
		t.Fatalf("Err: %v", sc.Err())
	}
	want := []string{"a", "bb", "ccc"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
}
