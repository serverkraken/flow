package sse

import (
	"context"
	"strings"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/domain"
)

// activityFor maps a live event to an activity entry. Returns ok=false for
// events that must not be logged (settings.changed; activity.logged itself).
func activityFor(ctx context.Context, ev domain.Event) (domain.ActivityEntry, bool) {
	switch ev.Type {
	case domain.EventSettingsChanged, domain.EventActivityLogged:
		return domain.ActivityEntry{}, false
	}
	a := actor.FromContext(ctx)
	e := domain.ActivityEntry{
		OwnerID:   ev.UserID,
		ActorKind: string(a.Kind),
		ActorRef:  a.Ref,
		Kind:      string(ev.Type),
	}
	if ev.Data != nil {
		if v, ok := ev.Data["id"].(string); ok && v != "" {
			e.TargetRef = &v
		}
		// title (documents) or name (nodes) → readable label snapshot.
		if v, ok := ev.Data["title"].(string); ok && v != "" {
			e.Label = &v
		} else if v, ok := ev.Data["name"].(string); ok && v != "" {
			e.Label = &v
		}
		if v, ok := ev.Data["node"].(string); ok && v != "" {
			e.NodeRef = &v
		}
	}
	// German document verbs such as "bearbeitete" require a concrete object,
	// and a document activity without a target cannot link anywhere. Keep the
	// live SSE event, but do not persist a malformed activity row.
	if strings.HasPrefix(e.Kind, "document.") && (e.TargetRef == nil || e.Label == nil) {
		return domain.ActivityEntry{}, false
	}
	return e, true
}
