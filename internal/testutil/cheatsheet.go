package testutil

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
