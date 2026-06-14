package domain

import "strings"

// IcalEscape escapes the four characters RFC 5545 §3.3.11 requires escaped
// in TEXT-typed values: backslash, semicolon, comma, newline. Carriage
// returns are dropped — the ICS line ending is CRLF and a literal \r in
// content would corrupt it.
func IcalEscape(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`;`, `\;`,
		`,`, `\,`,
		"\n", `\n`,
		"\r", "",
	)
	return r.Replace(s)
}
