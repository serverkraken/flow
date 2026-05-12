package testutil

import "github.com/serverkraken/flow/internal/ports"

var _ ports.CheatsheetReader = (*FakeCheatsheetReader)(nil)

// FakeCheatsheetReader returns the canned content unless Err is set.
type FakeCheatsheetReader struct {
	Content string
	Err     error
}

func (f *FakeCheatsheetReader) Load() (string, error) {
	if f.Err != nil {
		return "", f.Err
	}
	return f.Content, nil
}
