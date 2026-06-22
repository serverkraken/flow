package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/domain"
)

const docURIPrefix = "flow://doc/"

func docURI(id string) string { return docURIPrefix + id }

// inScope reports whether a document belongs to the resolved project — only such
// documents are exposed as resources.
func (h *handlers) inScope(d domain.Document) bool {
	return h.matched && d.ProjectID != nil && *d.ProjectID == h.proj.ID
}

// resourceFor builds the resource descriptor for a document.
func resourceFor(d domain.Document) *mcp.Resource {
	desc := fmt.Sprintf("%s · %s", d.Path, d.Type)
	if len(d.Tags) > 0 {
		desc += " · " + strings.Join(d.Tags, ", ")
	}
	return &mcp.Resource{
		URI:         docURI(d.ID),
		Name:        d.Title,
		Description: desc,
		MIMEType:    "text/markdown",
	}
}

// registerResources lists the resolved project's documents and registers a
// resource per document. No-op when unauthenticated or no project is bound.
func (h *handlers) registerResources(ctx context.Context) error {
	if !h.authed || !h.matched {
		return nil
	}
	docs, err := h.client.ListDocumentsScoped(ctx, &h.proj.ID)
	if err != nil {
		return err
	}
	for _, d := range docs {
		h.addResource(d)
	}
	return nil
}

// addResource registers (or refreshes) a document's resource. The read handler
// fetches the body fresh via GetDocument so content never goes stale.
func (h *handlers) addResource(d domain.Document) {
	if h.srv == nil || !h.inScope(d) {
		return
	}
	id := d.ID
	h.srv.AddResource(resourceFor(d), func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		doc, err := h.client.GetDocument(ctx, id)
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI: docURI(id), MIMEType: "text/markdown", Text: doc.Body,
		}}}, nil
	})
}

// removeResource unregisters a document's resource (safe if it was never added).
func (h *handlers) removeResource(id string) {
	if h.srv == nil {
		return
	}
	h.srv.RemoveResources(docURI(id))
}
