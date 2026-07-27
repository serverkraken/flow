package domain

import (
	"strings"
	"time"
)

// pathSep is the separator for client paths. flow clients are macOS/Linux, so
// paths are "/"-separated; hardcoding it keeps the domain pure (no os import)
// and avoids comparing with the SERVER's separator (the path is the client's).
const pathSep = "/"

type BindingKind string

const (
	BindingRemote BindingKind = "remote"
	BindingPath   BindingKind = "path"
)

type ProjectBinding struct {
	ID, OwnerID, NodeID string
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
	// Path tier: longest segment-boundary prefix of cwd among this machine's path bindings.
	var best ProjectBinding
	bestLen := -1
	for _, b := range bs {
		if b.Kind != BindingPath || b.MachineID != machineID || b.Path == "" {
			continue
		}
		if cwd == b.Path || strings.HasPrefix(cwd, b.Path+pathSep) {
			if len(b.Path) > bestLen {
				best, bestLen = b, len(b.Path)
			}
		}
	}
	if bestLen >= 0 {
		return best, true
	}
	return ProjectBinding{}, false
}
