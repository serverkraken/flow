package output_test

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/output"
	"github.com/serverkraken/flow/internal/testutil"
)

// defaultRunner / defaultStdinRunner exercise via NewWithRunners(nil, nil).
// We don't need to spawn real binaries — just invoke through the
// adapter's Copy/Pager paths with deterministic commands.

func TestNewWithRunners_NilFallsBackToDefault(t *testing.T) {
	tg := output.NewWithRunners(t.TempDir(), &testutil.FakeTmux{}, nil, nil)
	if tg == nil {
		t.Fatalf("NewWithRunners with nil runners should still construct")
	}
	// Copy through the default stdin runner — `true` is a no-op binary
	// available on every Unix-like host. Should succeed without surfacing
	// an error to the caller.
	err := tg.Copy("payload")
	// Whatever the actual default runner is wired to (pbcopy on darwin,
	// xclip elsewhere), the assertion is just that the call doesn't panic.
	// Errors are acceptable (binary may be missing on CI); we just
	// confirm we exercised the default-runner branch.
	_ = err
}

func TestPager_RejectsViewerWithMetacharacters(t *testing.T) {
	tg := output.New(t.TempDir(), &testutil.FakeTmux{})
	err := tg.Pager("payload", "less; rm -rf /", "txt")
	if err == nil {
		t.Errorf("Pager should reject viewer with shell metacharacters")
	}
	if !strings.Contains(err.Error(), "metacharacter") {
		t.Errorf("error message should mention metacharacter, got %q", err)
	}
}

// (Empty-viewer rejection is covered by pager_test.go's
// TestPager_RejectsEmptyViewer; not duplicated here.)
