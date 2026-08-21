package domain

import "strings"

// NormalizeLineEndings rewrites CRLF and lone CR to LF.
//
// Why at the domain edge: browsers normalise a <textarea> to CRLF on form
// submit (HTML spec, application/x-www-form-urlencoded), and every other
// reader in this codebase — fencedBlock, WikilinkTargets, the renderers — is
// written against "\n". Stored verbatim, a web-saved card silently lost its
// frontmatter: fencedBlock looks for "---\n" and saw "---\r\n". Normalising
// here, before Create and Update persist, covers web, REST and MCP with one
// rule instead of three.
func NormalizeLineEndings(s string) string {
	if !strings.ContainsRune(s, '\r') {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}
