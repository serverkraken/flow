package httpserver

import (
	"context"
	"log/slog"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
)

// emitEvent is the HTTP adapter's single event exit. Document activity verbs
// expect a readable target label, so id-only document mutations are enriched
// owner-scoped from the canonical store before the shared emitter persists the
// activity row. The live event is still emitted if enrichment fails; activityFor
// rejects incomplete document entries instead of rendering a broken sentence.
func (s *Server) emitEvent(ctx context.Context, ev domain.Event) {
	if strings.HasPrefix(string(ev.Type), "document.") {
		s.enrichDocumentEvent(ctx, &ev)
	}
	s.Emitter.Emit(ctx, ev)
}

func (s *Server) enrichDocumentEvent(ctx context.Context, ev *domain.Event) {
	if ev.Data == nil {
		return
	}
	id, _ := ev.Data["id"].(string)
	title, _ := ev.Data["title"].(string)
	if id == "" || strings.TrimSpace(title) != "" {
		return
	}
	if s.GetDocument.Docs == nil {
		slog.WarnContext(ctx, "document event: title enrichment unavailable", "id", id, "type", ev.Type)
		return
	}
	doc, err := s.GetDocument.Execute(ctx, ev.UserID, id)
	if err != nil {
		slog.WarnContext(ctx, "document event: title enrichment failed", "id", id, "type", ev.Type, "err", err)
		return
	}
	data := make(map[string]any, len(ev.Data)+1)
	for key, value := range ev.Data {
		data[key] = value
	}
	data["title"] = doc.Title
	ev.Data = data
}
