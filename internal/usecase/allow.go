package usecase

import "github.com/serverkraken/flow/internal/ports"

// AllowList builds the access gate: a user is allowed when their Username or
// Subject is in subs, OR any of their Groups is in groups. Either set may be
// empty; both empty denies everyone.
func AllowList(subs, groups map[string]bool) func(ports.Identity) bool {
	return func(id ports.Identity) bool {
		if subs[id.Username] || subs[id.Subject] {
			return true
		}
		for _, g := range id.Groups {
			if groups[g] {
				return true
			}
		}
		return false
	}
}
