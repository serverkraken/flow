package worktime_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/tui/screen/worktime"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func TestBuildRegistry_hasAllSiblingKeys(t *testing.T) {
	reg := worktime.BuildRegistry(nil, theme.Default)
	for _, k := range []string{"w", "t", "d", "e"} {
		if reg[k] == nil {
			t.Fatalf("registry missing key %q", k)
		}
		if reg[k]() == nil {
			t.Fatalf("factory %q returned nil route", k)
		}
	}
}
