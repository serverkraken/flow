package testutil

import (
	"fmt"

	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.Output = (*FakeOutput)(nil)

// FakeOutput records every ports.Output call so tests can assert
// behaviour without spawning real binaries or touching real files.
//
// Behaviour: each method appends its arguments to a slice and returns
// the matching *Err field (nil unless the test pre-set it). SaveFile
// returns a synthesised path "<SaveDir><basename>.<ext>" (no
// timestamp) so tests can string-match the expected basename without
// dealing with time-of-day jitter.
type FakeOutput struct {
	Copies []string
	Pagers []FakePagerCall
	Saves  []FakeSaveCall

	CopyErr  error
	PagerErr error
	SaveErr  error

	// SaveDir prefixes the synthesised SaveFile return path. Trailing
	// slash optional. Defaults to "/tmp/fake/" when empty.
	SaveDir string
}

// FakePagerCall captures one Pager invocation.
type FakePagerCall struct {
	Content string
	Viewer  string
	Ext     string
}

// FakeSaveCall captures one SaveFile invocation. Content is copied so
// the test owns a stable snapshot even when the caller mutates the
// original buffer afterwards.
type FakeSaveCall struct {
	Basename string
	Ext      string
	Content  []byte
}

// Copy records content and returns CopyErr.
func (f *FakeOutput) Copy(content string) error {
	f.Copies = append(f.Copies, content)
	return f.CopyErr
}

// Pager records the call and returns PagerErr.
func (f *FakeOutput) Pager(content, viewer, ext string) error {
	f.Pagers = append(f.Pagers, FakePagerCall{Content: content, Viewer: viewer, Ext: ext})
	return f.PagerErr
}

// SaveFile records the call and returns SaveErr or a synthesised path.
func (f *FakeOutput) SaveFile(basename, ext string, content []byte) (string, error) {
	cp := append([]byte(nil), content...)
	f.Saves = append(f.Saves, FakeSaveCall{Basename: basename, Ext: ext, Content: cp})
	if f.SaveErr != nil {
		return "", f.SaveErr
	}
	dir := f.SaveDir
	if dir == "" {
		dir = "/tmp/fake/"
	}
	if dir[len(dir)-1] != '/' {
		dir += "/"
	}
	return fmt.Sprintf("%s%s.%s", dir, basename, ext), nil
}
