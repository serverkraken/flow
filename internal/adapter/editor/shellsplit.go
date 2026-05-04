package editor

import "errors"

// splitShell performs minimal POSIX-shell-style tokenisation: tokens
// separated by whitespace; single quotes preserve everything inside;
// double quotes preserve everything except embedded `"` (which is
// escaped via `\"`); a trailing backslash escapes the next byte. Used
// by parseViewer so $FLOW_NOTE_VIEWER like `bat --paging=always` still
// works while shell metachars (`;`, `|`, `$()`) become literal token
// content rather than re-interpreted by /bin/sh -c at exec time.
//
// A near-identical splitter lives in
// internal/kompendium/adapter/nvimeditor/split.go for $EDITOR. Kept
// duplicated rather than shared because each adapter owns its own
// hexagonal slice; depguard rejects sibling-adapter imports.
func splitShell(input string) ([]string, error) {
	var (
		cur    []byte
		tokens []string
		state  = splitNormal
	)
	flush := func() {
		if len(cur) > 0 {
			tokens = append(tokens, string(cur))
			cur = cur[:0]
		}
	}
	for i := 0; i < len(input); {
		c := input[i]
		switch state {
		case splitSingle:
			if c == '\'' {
				state = splitNormal
			} else {
				cur = append(cur, c)
			}
			i++
		case splitDouble:
			switch {
			case c == '"':
				state = splitNormal
				i++
			case c == '\\' && i+1 < len(input):
				cur = append(cur, input[i+1])
				i += 2
			default:
				cur = append(cur, c)
				i++
			}
		default:
			switch c {
			case '\'':
				state = splitSingle
				i++
			case '"':
				state = splitDouble
				i++
			case '\\':
				if i+1 >= len(input) {
					return nil, errors.New("trailing backslash")
				}
				cur = append(cur, input[i+1])
				i += 2
			case ' ', '\t':
				flush()
				i++
			default:
				cur = append(cur, c)
				i++
			}
		}
	}
	if state != splitNormal {
		return nil, errors.New("unterminated quote")
	}
	flush()
	return tokens, nil
}

type splitState int

const (
	splitNormal splitState = iota
	splitSingle
	splitDouble
)
