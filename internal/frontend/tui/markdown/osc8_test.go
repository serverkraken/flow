package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/cellbuf"
)

const (
	osc8Open  = "\x1b]8;;"
	osc8Sep   = "\x07"
	osc8Close = "\x1b]8;;\x07"
)

func TestWrapURLs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "no url passthrough", in: "just some prose", want: "just some prose"},
		{
			name: "single url wrapped",
			in:   "see https://example.com here",
			want: "see " + osc8Open + "https://example.com" + osc8Sep + "https://example.com" + osc8Close + " here",
		},
		{
			name: "trailing dot kept outside the link",
			in:   "ref: https://example.com/path.",
			want: "ref: " + osc8Open + "https://example.com/path" + osc8Sep + "https://example.com/path" + osc8Close + ".",
		},
		{
			name: "trailing comma and closing paren stay outside",
			in:   "(see https://example.com/a),",
			want: "(see " + osc8Open + "https://example.com/a" + osc8Sep + "https://example.com/a" + osc8Close + "),",
		},
		{
			name: "two urls each wrapped",
			in:   "https://a.com and https://b.com/x",
			want: osc8Open + "https://a.com" + osc8Sep + "https://a.com" + osc8Close +
				" and " + osc8Open + "https://b.com/x" + osc8Sep + "https://b.com/x" + osc8Close,
		},
		{
			name: "url between SGR escapes",
			in:   "\x1b[34m\x1b[4mhttps://example.com\x1b[0m",
			want: "\x1b[34m\x1b[4m" + osc8Open + "https://example.com" + osc8Sep + "https://example.com" + osc8Close + "\x1b[0m",
		},
		{
			name: "http (non-tls) also matched",
			in:   "http://internal.local/x",
			want: osc8Open + "http://internal.local/x" + osc8Sep + "http://internal.local/x" + osc8Close,
		},
		{
			name: "no double wrap when input already has OSC 8",
			in:   "pre " + osc8Open + "https://x" + osc8Sep + "https://x" + osc8Close + " post",
			want: "pre " + osc8Open + "https://x" + osc8Sep + "https://x" + osc8Close + " post",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := WrapURLs(tc.in); got != tc.want {
				t.Errorf("WrapURLs mismatch\n got:  %q\n want: %q", got, tc.want)
			}
		})
	}
}

// TestWrapURLs_SurvivesCellbufHardWrap is the scenario that motivated
// this whole change: a URL longer than the wrap width gets hard-broken
// by lipgloss/cellbuf, but each hard-wrap must close and re-open the
// OSC 8 hyperlink so terminals keep every line fragment clickable. The
// injected input carries 2 OSC 8 markers (open + close); after cellbuf
// splits the URL across multiple lines we expect strictly more,
// because cellbuf emits ResetHyperlink + SetHyperlink around each
// in-link newline. The URL itself must also survive byte-for-byte.
func TestWrapURLs_SurvivesCellbufHardWrap(t *testing.T) {
	t.Parallel()
	const longURL = "https://example.com/very/long/path/that/will/not/fit/in/a/narrow/preview/pane/so/it/wraps"
	wrapped := WrapURLs("see " + longURL + " here")
	out := cellbuf.Wrap(wrapped, 30, "")

	if lines := strings.Count(out, "\n") + 1; lines < 2 {
		t.Fatalf("expected hard-wrap into multiple lines at width 30, got %d:\n%q", lines, out)
	}
	const marker = "\x1b]8;;"
	if got := strings.Count(out, marker); got <= strings.Count(wrapped, marker) {
		t.Errorf("cellbuf did not re-open OSC 8 across wraps: got %d markers in output, want > %d (input)\n%q",
			got, strings.Count(wrapped, marker), out)
	}
	if flat := strings.ReplaceAll(out, "\n", ""); !strings.Contains(flat, longURL) {
		t.Errorf("URL bytes not preserved across cellbuf wrap:\n%q", out)
	}
}
