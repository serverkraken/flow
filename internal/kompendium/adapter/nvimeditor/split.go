package nvimeditor

import (
	"errors"
	"fmt"
	"os"
)

// resolveEditor picks the editor binary and parses any embedded flags.
// $VISUAL takes precedence over $EDITOR per POSIX convention; both fall
// back to "nvim" when unset.
func resolveEditor() (string, []string, error) {
	raw := os.Getenv("VISUAL")
	if raw == "" {
		raw = os.Getenv("EDITOR")
	}
	if raw == "" {
		return defaultEditor, nil, nil
	}
	parts, err := splitCommand(raw)
	if err != nil {
		return "", nil, fmt.Errorf("parse editor %q: %w", raw, err)
	}
	if len(parts) == 0 {
		return defaultEditor, nil, nil
	}
	return parts[0], parts[1:], nil
}

// splitCommand performs minimal POSIX-shell-style splitting: tokens are
// separated by whitespace; single quotes preserve everything inside
// (including double quotes), double quotes preserve everything except
// embedded double quotes (which must be escaped with backslash); a
// trailing backslash escapes the next byte. It is enough for the common
// EDITOR shapes (`code -w`, `nvim -O`, `'/path with space/editor' -w`)
// without pulling in a full shell parser.
func splitCommand(input string) ([]string, error) {
	var sp splitter
	for i := 0; i < len(input); {
		var consumed int
		switch sp.state {
		case splitSingle:
			consumed = sp.stepSingle(input[i])
		case splitDouble:
			consumed = sp.stepDouble(input, i)
		default:
			consumed = sp.stepNormal(input, i)
		}
		i += consumed
	}
	if sp.state != splitNormal {
		return nil, errors.New("unterminated quote")
	}
	sp.flush()
	return sp.tokens, nil
}

// splitter holds the running state of splitCommand: the current token
// being assembled, the quote-mode state, and the tokens emitted so far.
type splitter struct {
	cur    []byte
	state  splitState
	tokens []string
}

func (s *splitter) flush() {
	if len(s.cur) > 0 || s.state != splitNormal {
		s.tokens = append(s.tokens, string(s.cur))
		s.cur = s.cur[:0]
	}
}

func (s *splitter) stepNormal(input string, i int) int {
	c := input[i]
	switch {
	case c == '\'':
		s.state = splitSingle
	case c == '"':
		s.state = splitDouble
	case c == '\\' && i+1 < len(input):
		s.cur = append(s.cur, input[i+1])
		return 2
	case c == ' ' || c == '\t':
		s.flush()
	default:
		s.cur = append(s.cur, c)
	}
	return 1
}

func (s *splitter) stepSingle(c byte) int {
	if c == '\'' {
		s.state = splitNormal
		return 1
	}
	s.cur = append(s.cur, c)
	return 1
}

func (s *splitter) stepDouble(input string, i int) int {
	c := input[i]
	switch {
	case c == '"':
		s.state = splitNormal
	case c == '\\' && i+1 < len(input):
		s.cur = append(s.cur, input[i+1])
		return 2
	default:
		s.cur = append(s.cur, c)
	}
	return 1
}

type splitState int

const (
	splitNormal splitState = iota
	splitSingle
	splitDouble
)
