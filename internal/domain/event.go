package domain

// EventType identifies a live event pushed to clients over SSE.
type EventType string

const (
	EventSessionStarted  EventType = "session.started"
	EventSessionStopped  EventType = "session.stopped"
	EventSessionUpdated  EventType = "session.updated"
	EventProjectCreated  EventType = "project.created"
	EventDayOffChanged   EventType = "dayoff.changed"
	EventSettingsChanged EventType = "settings.changed"
)

// Event is a server-originated change notification. UserID is the routing
// key and is never serialized to the client.
type Event struct {
	Type   EventType      `json:"type"`
	UserID string         `json:"-"`
	Data   map[string]any `json:"data,omitempty"`
}
