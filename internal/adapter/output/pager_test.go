package output_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/output"
	"github.com/serverkraken/flow/internal/testutil"
)

func TestPager_DispatchesTmuxSplitWithBashAndViewer(t *testing.T) {
	tmux := &testutil.FakeTmux{}
	tg := output.New(t.TempDir(), tmux)
	if err := tg.Pager("# Hallo\n", "glow", "md"); err != nil {
		t.Fatalf("Pager err: %v", err)
	}
	if len(tmux.Splits) != 1 {
		t.Fatalf("expected 1 split, got %d (%v)", len(tmux.Splits), tmux.Splits)
	}
	split := tmux.Splits[0]
	if !strings.HasPrefix(split, "bash -c ") {
		t.Errorf("split must invoke `bash -c …`; got %q", split)
	}
	if !strings.Contains(split, "glow ") {
		t.Errorf("split must include the viewer (`glow`); got %q", split)
	}
	if !strings.Contains(split, "; rm ") {
		t.Errorf("split must clean up the temp file via `; rm …`; got %q", split)
	}
	if !strings.Contains(split, ".md") {
		t.Errorf("temp file must carry the .md extension; got %q", split)
	}
}

func TestPager_DefaultsExtensionToTxtWhenEmpty(t *testing.T) {
	tmux := &testutil.FakeTmux{}
	tg := output.New(t.TempDir(), tmux)
	if err := tg.Pager("plain text\n", "less -S", ""); err != nil {
		t.Fatalf("Pager err: %v", err)
	}
	if !strings.Contains(tmux.Splits[0], ".txt") {
		t.Errorf("missing .txt fallback; got %q", tmux.Splits[0])
	}
}

func TestPager_RejectsEmptyViewer(t *testing.T) {
	tg := output.New(t.TempDir(), &testutil.FakeTmux{})
	err := tg.Pager("x", "  ", "md")
	if err == nil {
		t.Fatal("Pager should reject blank viewer command")
	}
}

func TestPager_PropagatesTmuxSplitError(t *testing.T) {
	want := errors.New("split-window failed")
	tmux := &testutil.FakeTmux{SplitErr: want}
	tg := output.New(t.TempDir(), tmux)
	err := tg.Pager("x", "less", "txt")
	if err == nil {
		t.Fatal("Pager should propagate tmux split failure")
	}
	if !errors.Is(err, want) {
		t.Errorf("Pager err = %v, want wrap of %v", err, want)
	}
}

// TestPager_TempFileExistsBeforeSplit verifies that the dispatched
// command-line points at a real, on-disk file at the moment SplitWindowH
// is invoked. The viewer would otherwise launch against a path that
// got removed in the same goroutine.
func TestPager_TempFileExistsBeforeSplit(t *testing.T) {
	var capturedPath string
	tmux := &testutil.FakeTmux{}
	tg := output.New(t.TempDir(), tmux)
	if err := tg.Pager("hi", "less", "txt"); err != nil {
		t.Fatalf("Pager err: %v", err)
	}
	// Extract the path between the first single-quoted token after the
	// viewer name. shellQuote wraps it in plain '<path>' (no embedded
	// quotes for simple temp paths under TMPDIR).
	split := tmux.Splits[0]
	const tag = "less '"
	i := strings.Index(split, tag)
	if i < 0 {
		t.Fatalf("could not find quoted path in %q", split)
	}
	rest := split[i+len(tag):]
	j := strings.Index(rest, "'")
	if j < 0 {
		t.Fatalf("unterminated quoted path in %q", split)
	}
	capturedPath = rest[:j]
	// The file is removed by the bash -c clause, but in-process before
	// rm runs (we never executed the bash line), it must still exist.
	if _, err := os.Stat(capturedPath); err != nil {
		t.Errorf("temp file %s must exist after Pager returns: %v", capturedPath, err)
	}
	// Clean up the temp file ourselves since the bash -c never ran.
	_ = os.Remove(capturedPath)
}
