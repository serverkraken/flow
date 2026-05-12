// Package shellsafe holds the predicates that gate strings interpolated
// into bash -c command lines. Two callers historically duplicated the
// list — the palette's tmux run-shell action filter and the pager's
// viewer-command filter — so the canonical "what would escape bash -c"
// set lives here.
//
// The set is intentionally exclusive (reject these), not inclusive
// (allow only these). Legitimate uses of bash include slashes, dots,
// hyphens, `-flags`, `@option-names`, and so on; an allowlist would
// either drift away from real usage or end up just enumerating the
// printable ASCII table. The forbidden set targets the specific
// characters that let a value chain a follow-up command, redirect
// I/O, or substitute output / variables.
package shellsafe

import "strings"

// chainingMetacharacters lists single-character forbiddances. Note that
// $ alone is not in the list: a literal $ in (for example) a tmux user
// option name (`@my-$var`) is harmless. The dangerous combinations are
// $( and ${ — caught separately so the predicate stays precise.
const chainingMetacharacters = ";|&`\n\r<>"

// ChainingOK reports whether s is free of characters that would chain
// extra commands or substitute output / variables inside a bash -c
// context. Suitable for strings that the caller surrounds with its own
// quoting (so embedded ' and " are part of a legitimate quoted-arg
// pattern).
func ChainingOK(s string) bool {
	if strings.ContainsAny(s, chainingMetacharacters) {
		return false
	}
	if strings.Contains(s, "$(") || strings.Contains(s, "${") {
		return false
	}
	return true
}

// Quote wraps s in single quotes and escapes embedded single quotes via
// the POSIX 'backslash-single-quote-single-quote-single-quote' sequence
// so the result reads as a single literal argument under /bin/sh -c.
// Callers that compose a bash command-line from untrusted-but-validated
// parts (paths, editor argv tokens, viewer arguments) use this to keep
// the parts atomic. Used to live duplicated in adapter/output and
// adapter/editor — kept here as the single canonical implementation.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// UnquotedOK extends ChainingOK by also rejecting ' and ". Use this for
// strings the caller interpolates verbatim (no surrounding quotes), e.g.
// a viewer command "less -S" placed at the head of a bash -c line. A
// stray quote in such a value would terminate or shift the surrounding
// bash quoting and let the rest of the value execute outside the
// intended context.
func UnquotedOK(s string) bool {
	if !ChainingOK(s) {
		return false
	}
	if strings.ContainsAny(s, `'"`) {
		return false
	}
	return true
}
