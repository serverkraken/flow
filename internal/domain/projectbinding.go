package domain

import "time"

type BindingKind string

const (
	BindingRemote BindingKind = "remote"
	BindingPath   BindingKind = "path"
)

type ProjectBinding struct {
	ID, OwnerID, ProjectID string
	Kind                   BindingKind
	RemoteSlug             string // kind=remote
	MachineID, MachineLabel, Path string // kind=path (Slice 2)
	CreatedAt, UpdatedAt   time.Time
}

// ResolveBinding returns the project binding for the given context. A remote
// match (by remoteSlug) wins. The path tier (machineID/cwd longest-prefix) is
// added in Slice 2.
func ResolveBinding(bs []ProjectBinding, remoteSlug, machineID, cwd string) (ProjectBinding, bool) {
	if remoteSlug != "" {
		for _, b := range bs {
			if b.Kind == BindingRemote && b.RemoteSlug == remoteSlug {
				return b, true
			}
		}
	}
	// Slice 2: else longest-prefix path match for machineID over cwd.
	return ProjectBinding{}, false
}
