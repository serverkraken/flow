package output_test

import (
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/output"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
)

// Compile-time assertion that *output.Targets satisfies ports.Output.
// Catches signature drift between the port and the adapter early.
var _ ports.Output = (*output.Targets)(nil)

func TestCopy_RoutesContentThroughPbcopyStdin(t *testing.T) {
	var sawName string
	var sawArgs []string
	var sawStdin string
	runIn := func(name string, args []string, stdin string) ([]byte, error) {
		sawName = name
		sawArgs = args
		sawStdin = stdin
		return nil, nil
	}
	tg := output.NewWithRunners(t.TempDir(), &testutil.FakeTmux{}, nil, runIn)
	if err := tg.Copy("Hallo Welt"); err != nil {
		t.Fatalf("Copy err: %v", err)
	}
	if sawName != "pbcopy" {
		t.Errorf("Copy called %q, want pbcopy", sawName)
	}
	if len(sawArgs) != 0 {
		t.Errorf("Copy passed args %v to pbcopy, want none", sawArgs)
	}
	if sawStdin != "Hallo Welt" {
		t.Errorf("Copy stdin = %q, want %q", sawStdin, "Hallo Welt")
	}
}

func TestCopy_PropagatesPbcopyError(t *testing.T) {
	want := errors.New("exec: pbcopy: not found")
	runIn := func(string, []string, string) ([]byte, error) { return nil, want }
	tg := output.NewWithRunners(t.TempDir(), &testutil.FakeTmux{}, nil, runIn)
	err := tg.Copy("x")
	if err == nil {
		t.Fatal("Copy should propagate pbcopy failure")
	}
	if !errors.Is(err, want) {
		t.Errorf("Copy err = %v, want wrap of %v", err, want)
	}
}
