package projects_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/tui/screen/projects"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func TestMountWiresDrillDown(t *testing.T) {
	// Mount with fakes; enter on the list must push a DetailRoute — proving the
	// detail factory is wired (not nil) by the composition seam.
	root := projects.MountWithAPI(&fakeAPI{ps: seed()}, &fakeDetailAPI{p: seed()[0]}, &fakeFormAPI{}, theme.Default, "msoent")
	r := root.(*projects.Route)
	drainInit(r)

	_, cmd := r.Update(keyEnter())
	if cmd == nil {
		t.Fatal("enter should emit a command")
	}
	push, ok := cmd().(shell.PushRouteMsg)
	if !ok {
		t.Fatalf("enter should push, got %T", cmd())
	}
	if _, ok := push.Route.(*projects.DetailRoute); !ok {
		t.Fatalf("enter should push a *DetailRoute, got %T", push.Route)
	}
}
