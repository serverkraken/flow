package httpserver

import "context"

// sessionEventData builds the Data payload for session.* events: always the
// session id, plus — when the session is booked to a node — the target's
// identity (node id, name, kind) so the activity log persists NodeRef+Label
// and live SSE consumers can render the target without a lookup. A missing or
// unreadable node degrades to id-only; emitting never fails the mutation.
func (s *Server) sessionEventData(ctx context.Context, ownerID, sessionID string, nodeID *string) map[string]any {
	data := map[string]any{"id": sessionID}
	if nodeID == nil {
		return data
	}
	n, err := s.GetNode.Execute(ctx, ownerID, *nodeID)
	if err != nil {
		return data
	}
	data["node"] = n.ID
	data["name"] = n.Name
	data["kind"] = string(n.Kind)
	return data
}
