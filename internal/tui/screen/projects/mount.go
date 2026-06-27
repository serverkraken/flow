package projects

import (
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

// var _ DetailAPI guards that *apiclient.Client keeps satisfying the detail
// route's surface (the other two surfaces are asserted in api.go / form.go).
// A drift in the client method set fails the build here.
var _ DetailAPI = (*apiclient.Client)(nil)

// Mount builds the "Projekte" tab root with list↔detail↔form navigation wired.
// formAPI is a separate parameter because *apiclient.Client no longer satisfies
// FormAPI after the rich CreateNode change (C4); use engagementCreateClient from
// cmd/flow as the form API in production.
func Mount(client *apiclient.Client, formAPI FormAPI, pal theme.Palette, user string) shell.Route {
	return MountWithAPI(client, client, formAPI, pal, user)
}

// MountWithAPI is the DI seam: tests inject fakes, production passes the same
// client three times. It returns the list route with the detail and form
// factories wired so enter→detail, n→create-form, and detail's e→edit-form all
// push real routes.
func MountWithAPI(list ProjectsAPI, detail DetailAPI, form FormAPI, pal theme.Palette, user string) shell.Route {
	root := NewRoute(list, pal, user)
	formFactory := func(editing *domain.Node) shell.Route {
		return NewFormRoute(form, pal, editing)
	}
	root.SetFormFactory(formFactory)
	root.SetDetailFactory(func(p domain.Node) shell.Route {
		d := NewDetailRoute(detail, pal, p)
		d.SetFormFactory(formFactory)
		return d
	})
	return root
}
