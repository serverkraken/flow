package shellsafe_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/shellsafe"
)

func TestQuote_NoSingleQuotes(t *testing.T) {
	t.Parallel()
	if got, want := shellsafe.Quote("/tmp/foo bar.txt"), `'/tmp/foo bar.txt'`; got != want {
		t.Errorf("Quote(plain) = %q, want %q", got, want)
	}
}

func TestQuote_WithSingleQuotes(t *testing.T) {
	t.Parallel()
	// POSIX 'don't' → 'don'\''t'
	if got, want := shellsafe.Quote("don't"), `'don'\''t'`; got != want {
		t.Errorf("Quote(don't) = %q, want %q", got, want)
	}
}

func TestQuote_Empty(t *testing.T) {
	t.Parallel()
	if got, want := shellsafe.Quote(""), `''`; got != want {
		t.Errorf("Quote(\"\") = %q, want %q", got, want)
	}
}
