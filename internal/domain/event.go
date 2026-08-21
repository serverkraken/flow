package domain

// EventType identifies a live event pushed to clients over SSE.
type EventType string

const (
	EventSessionStarted  EventType = "session.started"
	EventSessionStopped  EventType = "session.stopped"
	EventSessionUpdated  EventType = "session.updated"
	EventSessionDeleted  EventType = "session.deleted"
	EventNodeCreated     EventType = "node.created"
	EventNodeDeleted     EventType = "node.deleted"
	EventNodeUpdated     EventType = "node.updated"
	EventNodeMoved       EventType = "node.moved"
	EventDayOffChanged   EventType = "dayoff.changed"
	EventSettingsChanged EventType = "settings.changed"

	EventDocumentCreated EventType = "document.created"
	EventDocumentUpdated EventType = "document.updated"
	EventDocumentDeleted EventType = "document.deleted"

	EventArtifactCreated EventType = "artifact.created"
	EventArtifactUpdated EventType = "artifact.updated"
	EventArtifactDeleted EventType = "artifact.deleted"

	// EventHighlightChanged fires when a marked passage is assigned to a
	// register or removed again (Screen 27).
	EventHighlightChanged EventType = "highlight.changed"

	EventActivityLogged EventType = "activity.logged"
)

// Event is a server-originated change notification. UserID is the routing
// key and is never serialized to the client.
type Event struct {
	Type   EventType      `json:"type"`
	UserID string         `json:"-"`
	Data   map[string]any `json:"data,omitempty"`
}
