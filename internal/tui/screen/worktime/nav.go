package worktime

import (
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/dayoffs"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/export"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/statsrange"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/week"
	"github.com/serverkraken/flow/internal/tui/screen/worktime/wtnav"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// BuildRegistry wires the four Worktime sibling factories. The closures capture
// reg by reference so every sibling (and Today) navigates through the same
// registry — built before any closure runs, so the self-reference is safe.
// client may be nil in tests; the leaf routes only call it on Init/actions.
func BuildRegistry(client *apiclient.Client, pal theme.Palette) wtnav.Registry {
	var reg wtnav.Registry
	reg = wtnav.Registry{
		"w": func() shell.Route { return week.NewRoute(client, pal, reg) },
		"t": func() shell.Route { return statsrange.NewRoute(client, pal, reg) },
		"d": func() shell.Route { return dayoffs.NewRoute(client, pal, reg) },
		"e": func() shell.Route { return export.NewRoute(client, nil, pal, reg) },
	}
	return reg
}
