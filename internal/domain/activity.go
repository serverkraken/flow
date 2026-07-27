package domain

import "time"

// ActivityEntry is one persisted, owner-scoped activity-log row: who (Actor)
// did what (Kind = a raw EventType) to which target, with a readable Label
// snapshot taken at action time.
type ActivityEntry struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"-"`
	ActorKind string    `json:"actorKind"` // "human" | "agent"
	ActorRef  string    `json:"actorRef"`  // display name or agent client name
	Kind      string    `json:"kind"`      // raw EventType, e.g. "document.updated"
	TargetRef *string   `json:"targetRef,omitempty"`
	Label     *string   `json:"label,omitempty"`
	NodeRef   *string   `json:"nodeRef,omitempty"`
	At        time.Time `json:"at"`
}
