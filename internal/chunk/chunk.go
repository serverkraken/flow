// Package chunk splits a document into overlapping character windows for
// embedding. It is intentionally simple (character windows, not paragraph-aware):
// embedding recall tolerates mid-sentence cuts, and a deterministic window with
// fixed overlap is easy to reason about and test. Paragraph-aware packing is a
// possible future refinement.
package chunk

import "strings"

const (
	// MaxChars is the window size (~500 tokens at ~4 chars/token).
	MaxChars = 2000
	// OverlapChars is carried between consecutive windows (~15%).
	OverlapChars = 300
)

// Split returns the embeddable chunk texts for a document. The (trimmed) title is
// prepended to every chunk so each carries the document's subject. An empty body
// yields a single title-only chunk; an empty title+body yields nil. Deterministic,
// no I/O.
func Split(title, body string) []string {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if body == "" {
		if title == "" {
			return nil
		}
		return []string{title}
	}
	r := []rune(body)
	step := MaxChars - OverlapChars
	var out []string
	for start := 0; start < len(r); start += step {
		end := start + MaxChars
		if end > len(r) {
			end = len(r)
		}
		w := strings.TrimSpace(string(r[start:end]))
		if w == "" {
			// all-whitespace window: emit nothing (no empty embed input, no
			// degenerate title-only duplicate).
			if end == len(r) {
				break
			}
			continue
		}
		if title != "" {
			w = title + "\n\n" + w
		}
		out = append(out, w)
		if end == len(r) {
			break
		}
	}
	return out
}
