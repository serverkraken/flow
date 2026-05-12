package testutil

import "github.com/serverkraken/flow/internal/ports"

var _ ports.MarkdownRenderer = (*FakeMarkdownRenderer)(nil)

// FakeMarkdownRenderer wraps the input untouched (or with a "[w=N] "
// prefix when Width-marking is helpful for the test). Adequate for use
// cases that just need to know the renderer was called.
type FakeMarkdownRenderer struct {
	// Prefix is prepended to the content if non-empty.
	Prefix string
	Err    error
	// LastWidth records the most recent width passed in, for assertions.
	LastWidth int
}

func (f *FakeMarkdownRenderer) Render(content string, width int) (string, error) {
	if f.Err != nil {
		return "", f.Err
	}
	f.LastWidth = width
	return f.Prefix + content, nil
}
