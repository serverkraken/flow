package nodetree

import (
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// Mount builds the "Knoten" tab root with tree↔detail↔form navigation wired.
// The production composition root passes the shared *apiclient.Client, which
// satisfies all three narrow surfaces.
func Mount(client *apiclient.Client, pal theme.Palette, user string) shell.Route {
	return MountWithAPI(client, client, client, pal, user)
}

// MountWithAPI is the DI seam: tests inject fakes; production passes one client
// three times.
func MountWithAPI(tree TreeAPI, detail DetailAPI, form FormAPI, pal theme.Palette, user string) shell.Route {
	root := NewRoute(tree, pal, user)
	formFactory := func(editing *domain.Node) shell.Route {
		return NewFormRoute(form, pal, editing)
	}
	root.SetFormFactory(formFactory)
	root.SetDetailFactory(func(n domain.Node) shell.Route {
		d := NewDetailRoute(detail, pal, n)
		d.SetFormFactory(formFactory)
		return d
	})
	return root
}
