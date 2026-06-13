package ports

import "time"

// ConnState represents the connection state of the flow client to the server.
// Shared between the httpapi adapter and the TUI frontend so the status bar
// can read server reachability without importing the httpapi package.
type ConnState int

const (
	// StateUnknown is the initial state before any connection attempt.
	StateUnknown ConnState = iota
	// StateOnline indicates the server is reachable and the client is authenticated.
	StateOnline
	// StateOffline indicates the server is unreachable.
	StateOffline
	// StateLoggedOut indicates the client has no valid token.
	StateLoggedOut
	// StateNotConfigured indicates FLOW_SERVER_URL is not set.
	StateNotConfigured
	// StateOutdated indicates the client version is below the server's minimum.
	StateOutdated
)

// StatusSnapshot is a point-in-time view of the server connection state.
// Read by the TUI status bar via Deps.Status without importing the httpapi adapter.
type StatusSnapshot struct {
	State         ConnState
	Host          string    // server host for the status bar
	LastFetched   time.Time // most recent successful read (for "Stand 14:32")
	ServerVersion string
}
