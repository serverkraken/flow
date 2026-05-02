package testutil

// FakeNoteLauncher records every Open/View invocation as the note ID,
// prefixed with "open:" or "view:" so a single Calls slice tells the test
// what happened in order.
type FakeNoteLauncher struct {
	Calls []string
	Err   error
}

func (f *FakeNoteLauncher) Open(id string) error {
	f.Calls = append(f.Calls, "open:"+id)
	return f.Err
}

func (f *FakeNoteLauncher) View(id string) error {
	f.Calls = append(f.Calls, "view:"+id)
	return f.Err
}
